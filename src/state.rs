use crate::config::Config;
use crate::storage::Storage;
use sqlx::PgPool;
use std::sync::Arc;
use tokio::sync::Mutex;

#[derive(Clone)]
pub struct AppState {
    pub db: PgPool,
    pub storage: Storage,
    pub upload_lock: Arc<Mutex<()>>,
    pub config: Config,
}

impl AppState {
    pub fn new(db: PgPool, config: Config) -> Self {
        let storage = Storage::new(&config.data_dir);
        Self {
            db,
            storage,
            upload_lock: Arc::new(Mutex::new(())),
            config,
        }
    }
}
