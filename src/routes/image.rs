use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use serde::Deserialize;
use uuid::Uuid;
use crate::state::AppState;
use crate::error::AppError;
use crate::models::{UpdateImageReq, AddAliasReq, ImageDetail, Image};

#[derive(Debug, Deserialize)]
pub struct ImageQueryParams {
    pub search: Option<String>,
    pub gallery_id: Option<Uuid>,
}

// GET /images
#[utoipa::path(get, path = "/images", params(("search" = Option<String>, Query, description = "Search keyword"), ("gallery_id" = Option<Uuid>, Query, description = "Gallery ID Filter")), responses((status = 200, description = "List images successfully", body = [ImageDetail])))]
pub async fn list_images(
    State(state): State<AppState>,
    Query(params): Query<ImageQueryParams>,
) -> Result<impl IntoResponse, AppError> {
    let images = match (params.search, params.gallery_id) {
        (Some(search), Some(g_id)) => {
            sqlx::query_as::<_, Image>(
                "SELECT DISTINCT i.* FROM images i
                 JOIN gallery_images gi ON i.id = gi.image_id
                 LEFT JOIN image_aliases ia ON i.id = ia.image_id
                 WHERE gi.gallery_id = $1 AND (i.name ILIKE '%' || $2 || '%' OR ia.alias ILIKE '%' || $2 || '%')
                 ORDER BY i.created_at DESC"
            )
            .bind(g_id)
            .bind(search)
            .fetch_all(&state.db)
            .await?
        }
        (Some(search), None) => {
            sqlx::query_as::<_, Image>(
                "SELECT DISTINCT i.* FROM images i
                 LEFT JOIN image_aliases ia ON i.id = ia.image_id
                 WHERE i.name ILIKE '%' || $1 || '%' OR ia.alias ILIKE '%' || $1 || '%'
                 ORDER BY i.created_at DESC"
            )
            .bind(search)
            .fetch_all(&state.db)
            .await?
        }
        (None, Some(g_id)) => {
            sqlx::query_as::<_, Image>(
                "SELECT i.* FROM images i
                 JOIN gallery_images gi ON i.id = gi.image_id
                 WHERE gi.gallery_id = $1
                 ORDER BY i.created_at DESC"
            )
            .bind(g_id)
            .fetch_all(&state.db)
            .await?
        }
        (None, None) => {
            sqlx::query_as::<_, Image>(
                "SELECT * FROM images ORDER BY created_at DESC"
            )
            .fetch_all(&state.db)
            .await?
        }
    };

    let mut list = Vec::new();
    for img in images {
        let aliases = sqlx::query_scalar::<_, String>(
            "SELECT alias FROM image_aliases WHERE image_id = $1"
        )
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

// GET /images/:id
#[utoipa::path(get, path = "/images/{id}", params(("id" = Uuid, Path, description = "Image UUID")), responses((status = 200, description = "Image details", body = ImageDetail), (status = 404, description = "Image not found")))]
pub async fn get_image(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
) -> Result<impl IntoResponse, AppError> {
    let img = sqlx::query_as::<_, Image>(
        "SELECT * FROM images WHERE id = $1"
    )
    .bind(id)
    .fetch_optional(&state.db)
    .await?
    .ok_or(AppError::NotFound)?;

    let aliases = sqlx::query_scalar::<_, String>(
        "SELECT alias FROM image_aliases WHERE image_id = $1"
    )
    .bind(img.id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(ImageDetail {
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
    }))
}

// GET /images/by-hash/:prefix
#[utoipa::path(get, path = "/images/by-hash/{prefix}", params(("prefix" = String, Path, description = "Hexadecimal SHA256 prefix")), responses((status = 200, description = "Image matched", body = ImageDetail), (status = 404, description = "No image matched prefix"), (status = 409, description = "Ambiguous prefix matches multiple images")))]
pub async fn get_image_by_hash_prefix(
    State(state): State<AppState>,
    Path(prefix): Path<String>,
) -> Result<impl IntoResponse, AppError> {
    // 验证前缀是合法 hex
    let prefix_lower = prefix.to_lowercase();
    if !prefix_lower.chars().all(|c| c.is_ascii_hexdigit()) {
        return Err(AppError::BadRequest("Invalid hex prefix".into()));
    }

    let rows = sqlx::query_as::<_, Image>(
        "SELECT * FROM images WHERE sha256_hex LIKE $1 || '%' LIMIT 2"
    )
    .bind(&prefix_lower)
    .fetch_all(&state.db)
    .await?;

    match rows.len() {
        0 => Err(AppError::NotFound),
        1 => {
            let img = &rows[0];
            let aliases = sqlx::query_scalar::<_, String>(
                "SELECT alias FROM image_aliases WHERE image_id = $1"
            )
            .bind(img.id)
            .fetch_all(&state.db)
            .await?;
            Ok(Json(ImageDetail {
                id: img.id,
                sha256_hex: img.sha256_hex.clone(),
                name: img.name.clone(),
                ext: img.ext.clone(),
                width: img.width,
                height: img.height,
                size_bytes: img.size_bytes,
                metadata: img.metadata.clone(),
                created_at: img.created_at,
                updated_at: img.updated_at,
                aliases,
            }))
        }
        _ => Err(AppError::Conflict(
            format!("Prefix '{}' matches multiple images", prefix)
        )),
    }
}

// PATCH /images/:id
#[utoipa::path(patch, path = "/images/{id}", params(("id" = Uuid, Path, description = "Image UUID")), request_body = UpdateImageReq, responses((status = 200, description = "Image details updated successfully", body = ImageDetail), (status = 404, description = "Image not found")))]
pub async fn update_image(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
    Json(payload): Json<UpdateImageReq>,
) -> Result<impl IntoResponse, AppError> {
    // 先获取原信息
    let mut img = sqlx::query_as::<_, Image>(
        "SELECT * FROM images WHERE id = $1"
    )
    .bind(id)
    .fetch_optional(&state.db)
    .await?
    .ok_or(AppError::NotFound)?;

    let mut name = img.name;
    if let Some(new_name) = payload.name {
        name = if new_name.is_empty() { None } else { Some(new_name) };
    }

    let mut metadata = img.metadata;
    if let Some(new_meta) = payload.metadata {
        metadata = new_meta;
    }

    img = sqlx::query_as::<_, Image>(
        "UPDATE images SET name = $1, metadata = $2, updated_at = NOW() WHERE id = $3 RETURNING *"
    )
    .bind(name)
    .bind(metadata)
    .bind(id)
    .fetch_one(&state.db)
    .await?;

    let aliases = sqlx::query_scalar::<_, String>(
        "SELECT alias FROM image_aliases WHERE image_id = $1"
    )
    .bind(img.id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(ImageDetail {
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
    }))
}

// POST /images/:id/aliases
#[utoipa::path(post, path = "/images/{id}/aliases", params(("id" = Uuid, Path, description = "Image UUID")), request_body = AddAliasReq, responses((status = 201, description = "Alias associated to image successfully"), (status = 404, description = "Image not found")))]
pub async fn add_image_alias(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
    Json(payload): Json<AddAliasReq>,
) -> Result<impl IntoResponse, AppError> {
    // 确保 image 存在
    let exists = sqlx::query_scalar::<_, bool>(
        "SELECT EXISTS(SELECT 1 FROM images WHERE id = $1)"
    )
    .bind(id)
    .fetch_one(&state.db)
    .await?;

    if !exists {
        return Err(AppError::NotFound);
    }

    sqlx::query("INSERT INTO image_aliases (alias, image_id) VALUES ($1, $2)")
        .bind(&payload.alias)
        .bind(id)
        .execute(&state.db)
        .await?;

    Ok(StatusCode::CREATED)
}

// DELETE /images/:id/aliases/:alias
#[utoipa::path(delete, path = "/images/{id}/aliases/{alias}", params(("id" = Uuid, Path, description = "Image UUID"), ("alias" = String, Path, description = "Alias string")), responses((status = 204, description = "Alias unlinked successfully"), (status = 404, description = "Image/Alias not found")))]
pub async fn delete_image_alias(
    State(state): State<AppState>,
    Path((id, alias)): Path<(Uuid, String)>,
) -> Result<impl IntoResponse, AppError> {
    let rows_affected = sqlx::query(
        "DELETE FROM image_aliases WHERE image_id = $1 AND alias = $2"
    )
    .bind(id)
    .bind(&alias)
    .execute(&state.db)
    .await?
    .rows_affected();

    if rows_affected == 0 {
        return Err(AppError::NotFound);
    }

    Ok(StatusCode::NO_CONTENT)
}
