use crate::db::find_perceptual_duplicate;
use crate::error::AppError;
use crate::hash::perceptual;
use crate::hash::sha256::StreamSha256;
use crate::models::{Image, ImageDetail};
use crate::state::AppState;
use axum::{
    extract::{Multipart, Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use image::GenericImageView;
use serde::Deserialize;
use std::io::Write;
use uuid::Uuid;

#[derive(Debug, Deserialize)]
pub struct UploadParams {
    pub force: Option<bool>,
}

// POST /galleries/:gallery_id/images
#[utoipa::path(post, path = "/galleries/{gallery_id}/images", params(("gallery_id" = Uuid, Path, description = "Gallery UUID"), ("force" = Option<bool>, Query, description = "Force upload and bypass similarity check")), responses((status = 200, description = "Image already existed and was associated", body = ImageDetail), (status = 201, description = "New image uploaded and associated successfully", body = ImageDetail), (status = 409, description = "Similarity check collision or duplicate association")))]
pub async fn upload_image(
    State(state): State<AppState>,
    Path(gallery_id): Path<Uuid>,
    Query(params): Query<UploadParams>,
    mut multipart: Multipart,
) -> Result<impl IntoResponse, AppError> {
    // 确保 gallery 存在
    let exists =
        sqlx::query_scalar::<_, bool>("SELECT EXISTS(SELECT 1 FROM galleries WHERE id = $1)")
            .bind(gallery_id)
            .fetch_one(&state.db)
            .await?;

    if !exists {
        return Err(AppError::NotFound);
    }

    let mut temp_file = state.storage.temp_file()?;
    let mut stream_sha = StreamSha256::new();

    // 解析上传的文件流
    while let Some(mut field) = multipart.next_field().await? {
        if let Some(name) = field.name() {
            if name == "file" {
                while let Some(chunk) = field.chunk().await? {
                    temp_file.write_all(&chunk)?;
                    stream_sha.update(&chunk);
                }
                break;
            }
        }
    }

    let (sha256_hash, sha256_hex, size_bytes) = stream_sha.finalize();
    if size_bytes == 0 {
        return Err(AppError::BadRequest("Uploaded file is empty".into()));
    }

    // 将临时文件路径读取到内存，进行解码和哈希
    let temp_path = temp_file.path().to_path_buf();
    let data = tokio::fs::read(&temp_path).await?;

    // CPU 密集型任务：在 block 线程池中计算哈希和图片信息
    let (is_gif, width, height, ext, perceptual_hash) = tokio::task::spawn_blocking(move || {
        let format = image::guess_format(&data)?;
        let ext = match format {
            image::ImageFormat::Jpeg => "jpg",
            image::ImageFormat::Png => "png",
            image::ImageFormat::Gif => "gif",
            image::ImageFormat::WebP => "webp",
            image::ImageFormat::Bmp => "bmp",
            _ => "bin",
        };

        let is_gif = format == image::ImageFormat::Gif;
        let perceptual_hash = if is_gif {
            perceptual::compute_gif(&data)?
        } else {
            let img = image::load_from_memory_with_format(&data, format)?;
            perceptual::compute(&img)
        };

        // 重新 load 获取尺寸（避免大图拷贝）
        let img_info = image::load_from_memory_with_format(&data, format)?;
        let (w, h) = img_info.dimensions();

        Ok::<_, AppError>((is_gif, w as i32, h as i32, ext.to_string(), perceptual_hash))
    })
    .await
    .map_err(|e| AppError::JoinError(e.to_string()))??;

    // 获取全局上传锁
    let _lock = state.upload_lock.lock().await;

    // 开始数据库事务
    let mut tx = state.db.begin().await?;

    // Check if image already exists by SHA256
    let existing_image = sqlx::query_as::<_, Image>("SELECT * FROM images WHERE sha256_hash = $1")
        .bind(&sha256_hash)
        .fetch_optional(&mut *tx)
        .await?;

    if let Some(img) = existing_image {
        // [复用分支]
        let link_exists = sqlx::query_scalar::<_, bool>(
            "SELECT EXISTS(SELECT 1 FROM gallery_images WHERE gallery_id = $1 AND image_id = $2)",
        )
        .bind(gallery_id)
        .bind(img.id)
        .fetch_one(&mut *tx)
        .await?;

        if link_exists {
            tx.rollback().await?;
            return Err(AppError::AlreadyInGallery);
        }

        sqlx::query("INSERT INTO gallery_images (gallery_id, image_id) VALUES ($1, $2)")
            .bind(gallery_id)
            .bind(img.id)
            .execute(&mut *tx)
            .await?;

        let aliases =
            sqlx::query_scalar::<_, String>("SELECT alias FROM image_aliases WHERE image_id = $1")
                .bind(img.id)
                .fetch_all(&mut *tx)
                .await?;

        tx.commit().await?;

        // 成功，删除临时文件
        let _ = tokio::fs::remove_file(&temp_path).await;
        return Ok((
            StatusCode::OK,
            Json(ImageDetail {
                id: img.id,
                sha256_hex: img.sha256_hex,
                name: img.name,
                ext: img.ext,
                width: img.width,
                height: img.height,
                size_bytes: img.size_bytes,
                metadata: img.metadata,
                created_at: img.created_at,
                updated_at: img.updated_at,
                aliases,
            }),
        ));
    }

    // [新图分支]
    // 检查感知哈希碰撞
    let force = params.force.unwrap_or(false);
    if !force {
        let duplicate = find_perceptual_duplicate(
            &mut tx,
            &perceptual_hash,
            state.config.perceptual.ahash_threshold,
            state.config.perceptual.dhash_threshold,
            state.config.perceptual.phash_threshold,
        )
        .await?;

        if let Some(dup) = duplicate {
            tx.rollback().await?;
            return Err(AppError::PerceptualDuplicate {
                image_id: dup.image_id,
            });
        }
    }

    // 原子移动临时文件到最终路径
    let _final_dest = state.storage.persist(temp_file, &sha256_hex, &ext)?;

    // 写入 images 表
    let img = sqlx::query_as::<_, Image>(
        "INSERT INTO images (sha256_hash, ext, width, height, size_bytes)
         VALUES ($1, $2, $3, $4, $5) RETURNING *",
    )
    .bind(&sha256_hash)
    .bind(&ext)
    .bind(width)
    .bind(height)
    .bind(size_bytes as i64)
    .fetch_one(&mut *tx)
    .await?;

    // 写入 perceptual_hashes 表
    let frame_hashes_json = serde_json::to_value(&perceptual_hash.frame_hashes)
        .unwrap_or(serde_json::Value::Array(vec![]));

    sqlx::query(
        "INSERT INTO perceptual_hashes (
            image_id, is_gif, frame_count, ahash, dhash_h, dhash_v, phash,
            bucket1, bucket2, bucket3, bucket4, frame_hashes
         ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
    )
    .bind(img.id)
    .bind(is_gif)
    .bind(if is_gif {
        perceptual_hash.frame_hashes.len() as i32
    } else {
        1
    })
    .bind(perceptual_hash.ahash)
    .bind(perceptual_hash.dhash_h)
    .bind(perceptual_hash.dhash_v)
    .bind(perceptual_hash.phash)
    .bind(perceptual_hash.bucket1)
    .bind(perceptual_hash.bucket2)
    .bind(perceptual_hash.bucket3)
    .bind(perceptual_hash.bucket4)
    .bind(frame_hashes_json)
    .execute(&mut *tx)
    .await?;

    // 写入 gallery_images 表
    sqlx::query("INSERT INTO gallery_images (gallery_id, image_id) VALUES ($1, $2)")
        .bind(gallery_id)
        .bind(img.id)
        .execute(&mut *tx)
        .await?;

    tx.commit().await?;

    Ok((
        StatusCode::CREATED,
        Json(ImageDetail {
            id: img.id,
            sha256_hex: img.sha256_hex,
            name: img.name,
            ext: img.ext,
            width: img.width,
            height: img.height,
            size_bytes: img.size_bytes,
            metadata: img.metadata,
            created_at: img.created_at,
            updated_at: img.updated_at,
            aliases: vec![],
        }),
    ))
}

// GET /galleries/:gallery_id/images
#[utoipa::path(get, path = "/galleries/{gallery_id}/images", params(("gallery_id" = Uuid, Path, description = "Gallery UUID")), responses((status = 200, description = "Images list retrieved successfully", body = [ImageDetail])))]
pub async fn list_gallery_images(
    State(state): State<AppState>,
    Path(gallery_id): Path<Uuid>,
) -> Result<impl IntoResponse, AppError> {
    // 确保 gallery 存在
    let exists =
        sqlx::query_scalar::<_, bool>("SELECT EXISTS(SELECT 1 FROM galleries WHERE id = $1)")
            .bind(gallery_id)
            .fetch_one(&state.db)
            .await?;

    if !exists {
        return Err(AppError::NotFound);
    }

    let images = sqlx::query_as::<_, Image>(
        "SELECT i.* FROM images i
         JOIN gallery_images gi ON i.id = gi.image_id
         WHERE gi.gallery_id = $1
         ORDER BY gi.created_at DESC",
    )
    .bind(gallery_id)
    .fetch_all(&state.db)
    .await?;

    let mut list = Vec::new();
    for img in images {
        let aliases =
            sqlx::query_scalar::<_, String>("SELECT alias FROM image_aliases WHERE image_id = $1")
                .bind(img.id)
                .fetch_all(&state.db)
                .await?;
        list.push(ImageDetail {
            id: img.id,
            sha256_hex: img.sha256_hex,
            name: img.name,
            ext: img.ext,
            width: img.width,
            height: img.height,
            size_bytes: img.size_bytes,
            metadata: img.metadata,
            created_at: img.created_at,
            updated_at: img.updated_at,
            aliases,
        });
    }

    Ok(Json(list))
}

// DELETE /galleries/:gallery_id/images/:image_id
#[utoipa::path(delete, path = "/galleries/{gallery_id}/images/{image_id}", params(("gallery_id" = Uuid, Path, description = "Gallery UUID"), ("image_id" = Uuid, Path, description = "Image UUID")), responses((status = 204, description = "Image unlinked successfully"), (status = 404, description = "Gallery or Image association not found")))]
pub async fn remove_image_from_gallery(
    State(state): State<AppState>,
    Path((gallery_id, image_id)): Path<(Uuid, Uuid)>,
) -> Result<impl IntoResponse, AppError> {
    let rows_affected =
        sqlx::query("DELETE FROM gallery_images WHERE gallery_id = $1 AND image_id = $2")
            .bind(gallery_id)
            .bind(image_id)
            .execute(&state.db)
            .await?
            .rows_affected();

    if rows_affected == 0 {
        return Err(AppError::NotFound);
    }

    Ok(StatusCode::NO_CONTENT)
}
