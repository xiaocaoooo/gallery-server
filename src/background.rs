use crate::state::AppState;
use crate::error::AppError;

pub fn spawn_background_tasks(state: AppState) {
    let hours = state.config.orphan_cleanup_interval_hours;
    if hours == 0 {
        tracing::info!("Orphan cleanup disabled");
        return;
    }

    let interval = std::time::Duration::from_secs(hours * 3600);

    tokio::spawn(async move {
        let mut ticker = tokio::time::interval(interval);
        // 首次延迟
        ticker.tick().await;

        loop {
            ticker.tick().await;
            if let Err(e) = cleanup_orphans(&state).await {
                tracing::error!("Background orphan cleanup failed: {}", e);
            }
        }
    });
}

async fn cleanup_orphans(state: &AppState) -> Result<(), AppError> {
    tracing::info!("Starting background orphan cleanup");

    let orphans = sqlx::query(
        "SELECT i.id, i.sha256_hex, i.ext FROM images i
         LEFT JOIN gallery_images gi ON i.id = gi.image_id
         WHERE gi.image_id IS NULL"
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
        tracing::info!("Removed orphan record from db: {}", id);
    }

    // 清理临时文件
    if state.config.tmp_dir.exists() {
        let mut entries = tokio::fs::read_dir(&state.config.tmp_dir).await?;
        while let Some(entry) = entries.next_entry().await? {
            let meta = entry.metadata().await?;
            if let Ok(modified) = meta.modified() {
                let age = std::time::SystemTime::now().duration_since(modified).unwrap_or_default();
                // 删除超过 24 小时的临时文件
                if age > std::time::Duration::from_secs(86400) {
                    let _ = tokio::fs::remove_file(entry.path()).await;
                }
            }
        }
    }

    tracing::info!("Background orphan cleanup completed");
    Ok(())
}
