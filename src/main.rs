mod background;
mod config;
mod db;
mod error;
mod hash;
mod models;
mod routes;
mod state;
mod storage;

use axum::{extract::State, http::StatusCode, response::IntoResponse, Json, Router};
use sqlx::postgres::{PgConnectOptions, PgPoolOptions};
use sqlx::PgPool;
use std::net::SocketAddr;
use tower::ServiceBuilder;
use tower_http::limit::RequestBodyLimitLayer;
use tower_http::trace::TraceLayer;

use crate::config::{Config, LogFormat};
use crate::state::AppState;

pub async fn create_pool(config: &Config) -> Result<PgPool, sqlx::Error> {
    let connect_options = config.database_url.parse::<PgConnectOptions>()?.ssl_mode(
        match config.database_ssl_mode.as_str() {
            "disable" => sqlx::postgres::PgSslMode::Disable,
            "require" => sqlx::postgres::PgSslMode::Require,
            _ => sqlx::postgres::PgSslMode::Prefer,
        },
    );

    PgPoolOptions::new()
        .max_connections(config.database_max_connections)
        .acquire_timeout(std::time::Duration::from_secs(10))
        .connect_with(connect_options)
        .await
}

pub async fn run_migrations(pool: &PgPool) -> Result<(), sqlx::migrate::MigrateError> {
    sqlx::migrate!("./migrations").run(pool).await
}

// GET /health
async fn health_check(State(state): State<AppState>) -> impl IntoResponse {
    let db_healthy = sqlx::query("SELECT 1").fetch_one(&state.db).await.is_ok();

    let status = if db_healthy { "healthy" } else { "unhealthy" };
    let code = if db_healthy {
        StatusCode::OK
    } else {
        StatusCode::SERVICE_UNAVAILABLE
    };

    (
        code,
        Json(serde_json::json!({
            "status": status,
            "database": db_healthy,
            "timestamp": chrono::Utc::now().to_rfc3339()
        })),
    )
}

fn init_tracing(config: &Config) {
    let subscriber = tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env());

    match config.log_format {
        LogFormat::Json => subscriber.json().init(),
        LogFormat::Pretty => subscriber.pretty().init(),
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 加载 .env 变量
    let _ = dotenvy::dotenv();

    let config = Config::from_env().expect("Invalid configuration");
    config.validate().expect("Validation failed");

    // 初始化日志
    init_tracing(&config);

    let pool = create_pool(&config)
        .await
        .expect("Database connection failed");

    if config.auto_migrate {
        run_migrations(&pool).await.expect("Migration failed");
    }

    let state = AppState::new(pool, config.clone());

    // 启动后台孤儿清理定时任务
    background::spawn_background_tasks(state.clone());

    let app = Router::new()
        .route("/health", axum::routing::get(health_check))
        .merge(routes::all_routes())
        .layer(
            ServiceBuilder::new()
                .layer(TraceLayer::new_for_http())
                .layer(RequestBodyLimitLayer::new(
                    state.config.request_body_limit_mb * 1024 * 1024,
                )),
        )
        .with_state(state);

    let addr: SocketAddr = config.bind_address.parse().expect("Invalid bind address");
    tracing::info!("Server listening on {}", addr);

    let listener = tokio::net::TcpListener::bind(&addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}
