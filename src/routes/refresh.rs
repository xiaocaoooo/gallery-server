use crate::error::AppError;
use crate::hash::perceptual;
use crate::models::RefreshReq;
use crate::state::AppState;
use axum::{extract::State, http::StatusCode, response::IntoResponse, Json};

// POST /refresh
#[utoipa::path(post, path = "/refresh", request_body = RefreshReq, responses((status = 202, description = "Refresh tasks scheduled in background successfully")))]
pub async fn refresh_tasks(
    State(state): State<AppState>,
    Json(payload): Json<RefreshReq>,
) -> Result<impl IntoResponse, AppError> {
    let clear_orphans = payload.clear_orphans.unwrap_or(false);
    let refresh_hashes = payload.refresh_hashes.unwrap_or(false);
    let clear_temp = payload.clear_temp.unwrap_or(false);

    let state_clone = state.clone();

    tokio::spawn(async move {
        if clear_orphans {
            if let Err(e) = run_clear_orphans(&state_clone).await {
                tracing::error!("Manual orphan cleanup failed: {}", e);
            }
        }

        if clear_temp {
            if let Err(e) = run_clear_temp(&state_clone).await {
                tracing::error!("Manual temp cleanup failed: {}", e);
            }
        }

        if refresh_hashes {
            if let Err(e) = run_refresh_hashes(&state_clone).await {
                tracing::error!("Manual hash refresh failed: {}", e);
            }
        }
    });

    Ok((
        StatusCode::ACCEPTED,
        Json(serde_json::json!({ "status": "Refresh tasks scheduled in background" })),
    ))
}

async fn run_clear_orphans(state: &AppState) -> Result<(), AppError> {
    tracing::info!("Starting manual orphan cleanup");
    let orphans = sqlx::query(
        "SELECT id, sha256_hex, ext FROM images i
         LEFT JOIN gallery_images gi ON i.id = gi.image_id
         WHERE gi.image_id IS NULL",
    )
    .fetch_all(&state.db)
    .await?;

    use sqlx::Row;
    for o in orphans {
        let id: uuid::Uuid = o.try_get("id")?;
        let sha256_hex: String = o.try_get("sha256_hex")?;
        let ext: String = o.try_get("ext")?;

        let path = state.storage.final_path(&sha256_hex, &ext);
        if tokio::fs::remove_file(&path).await.is_ok() {
            tracing::info!("Removed orphan file: {:?}", path);
        }
        sqlx::query("DELETE FROM images WHERE id = $1")
            .bind(id)
            .execute(&state.db)
            .await?;
        tracing::info!("Removed orphan record: {}", id);
    }
    Ok(())
}

async fn run_clear_temp(state: &AppState) -> Result<(), AppError> {
    tracing::info!("Starting manual temp cleanup");
    if !state.config.tmp_dir.exists() {
        return Ok(());
    }
    let mut entries = tokio::fs::read_dir(&state.config.tmp_dir).await?;
    while let Some(entry) = entries.next_entry().await? {
        let _ = tokio::fs::remove_file(entry.path()).await;
    }
    Ok(())
}

async fn run_refresh_hashes(state: &AppState) -> Result<(), AppError> {
    tracing::info!("Starting manual hash refresh");
    let images = sqlx::query("SELECT id, sha256_hex, ext FROM images")
        .fetch_all(&state.db)
        .await?;

    use sqlx::Row;
    for img in images {
        let id: uuid::Uuid = img.try_get("id")?;
        let sha256_hex: String = img.try_get("sha256_hex")?;
        let ext: String = img.try_get("ext")?;

        let path = state.storage.final_path(&sha256_hex, &ext);
        if !path.exists() {
            continue;
        }

        let data = tokio::fs::read(&path).await?;
        let is_gif = ext == "gif";

        let perceptual_hash = tokio::task::spawn_blocking(move || {
            if is_gif {
                perceptual::compute_gif(&data)
            } else {
                let format = image::guess_format(&data)?;
                let dyn_img = image::load_from_memory_with_format(&data, format)?;
                Ok(perceptual::compute(&dyn_img))
            }
        })
        .await
        .map_err(|e| AppError::JoinError(e.to_string()))??;

        let frame_hashes_json = serde_json::to_value(&perceptual_hash.frame_hashes)
            .unwrap_or(serde_json::Value::Array(vec![]));

        sqlx::query(
            "UPDATE perceptual_hashes SET 
             ahash=$1, dhash_h=$2, dhash_v=$3, phash=$4,
             bucket1=$5, bucket2=$6, bucket3=$7, bucket4=$8,
             frame_hashes=$9
             WHERE image_id=$10",
        )
        .bind(perceptual_hash.ahash)
        .bind(perceptual_hash.dhash_h)
        .bind(perceptual_hash.dhash_v)
        .bind(perceptual_hash.phash)
        .bind(perceptual_hash.bucket1)
        .bind(perceptual_hash.bucket2)
        .bind(perceptual_hash.bucket3)
        .bind(perceptual_hash.bucket4)
        .bind(frame_hashes_json)
        .bind(id)
        .execute(&state.db)
        .await?;
    }
    tracing::info!("Manual hash refresh completed");
    Ok(())
}
