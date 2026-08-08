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
use crate::models::{CreateGalleryReq, UpdateGalleryReq, AddAliasReq, GalleryDetail, Gallery};

#[derive(Debug, Deserialize)]
pub struct SearchParams {
    pub search: Option<String>,
}

// POST /galleries
#[utoipa::path(post, path = "/galleries", request_body = CreateGalleryReq, responses((status = 201, description = "Gallery created successfully", body = GalleryDetail)))]
pub async fn create_gallery(
    State(state): State<AppState>,
    Json(payload): Json<CreateGalleryReq>,
) -> Result<impl IntoResponse, AppError> {
    let mut tx = state.db.begin().await?;

    let gallery = sqlx::query_as::<_, Gallery>(
        "INSERT INTO galleries (name) VALUES ($1) RETURNING *"
    )
    .bind(&payload.name)
    .fetch_one(&mut *tx)
    .await?;

    let mut aliases = Vec::new();
    if let Some(alias_list) = payload.aliases {
        for alias in alias_list {
            sqlx::query(
                "INSERT INTO gallery_aliases (alias, gallery_id) VALUES ($1, $2)"
            )
            .bind(&alias)
            .bind(gallery.id)
            .execute(&mut *tx)
            .await?;
            aliases.push(alias);
        }
    }

    tx.commit().await?;

    Ok((
        StatusCode::CREATED,
        Json(GalleryDetail {
            id: gallery.id,
            name: gallery.name,
            created_at: gallery.created_at,
            updated_at: gallery.updated_at,
            aliases,
        }),
    ))
}

// GET /galleries
#[utoipa::path(get, path = "/galleries", params(("search" = Option<String>, Query, description = "Query string to filter by name/alias")), responses((status = 200, description = "List galleries successfully", body = [GalleryDetail])))]
pub async fn list_galleries(
    State(state): State<AppState>,
    Query(params): Query<SearchParams>,
) -> Result<impl IntoResponse, AppError> {
    let galleries = if let Some(search) = params.search {
        sqlx::query_as::<_, Gallery>(
            "SELECT DISTINCT g.* FROM galleries g
             LEFT JOIN gallery_aliases ga ON g.id = ga.gallery_id
             WHERE g.name ILIKE '%' || $1 || '%' OR ga.alias ILIKE '%' || $1 || '%'"
        )
        .bind(search)
        .fetch_all(&state.db)
        .await?
    } else {
        sqlx::query_as::<_, Gallery>("SELECT * FROM galleries ORDER BY created_at DESC")
            .fetch_all(&state.db)
            .await?
    };

    let mut list = Vec::new();
    for g in galleries {
        let aliases = sqlx::query_scalar::<_, String>(
            "SELECT alias FROM gallery_aliases WHERE gallery_id = $1"
        )
        .bind(g.id)
        .fetch_all(&state.db)
        .await?;
        
        list.push(GalleryDetail {
            id: g.id,
            name: g.name,
            created_at: g.created_at,
            updated_at: g.updated_at,
            aliases,
        });
    }

    Ok(Json(list))
}

// GET /galleries/:id
#[utoipa::path(get, path = "/galleries/{id}", params(("id" = Uuid, Path, description = "Gallery UUID")), responses((status = 200, description = "Gallery detailed info", body = GalleryDetail), (status = 404, description = "Gallery not found")))]
pub async fn get_gallery(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
) -> Result<impl IntoResponse, AppError> {
    let gallery = sqlx::query_as::<_, Gallery>(
        "SELECT * FROM galleries WHERE id = $1"
    )
    .bind(id)
    .fetch_optional(&state.db)
    .await?
    .ok_or(AppError::NotFound)?;

    let aliases = sqlx::query_scalar::<_, String>(
        "SELECT alias FROM gallery_aliases WHERE gallery_id = $1"
    )
    .bind(gallery.id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(GalleryDetail {
        id: gallery.id,
        name: gallery.name,
        created_at: gallery.created_at,
        updated_at: gallery.updated_at,
        aliases,
    }))
}

// PATCH /galleries/:id
#[utoipa::path(patch, path = "/galleries/{id}", params(("id" = Uuid, Path, description = "Gallery UUID")), request_body = UpdateGalleryReq, responses((status = 200, description = "Gallery updated", body = GalleryDetail), (status = 404, description = "Gallery not found")))]
pub async fn update_gallery(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
    Json(payload): Json<UpdateGalleryReq>,
) -> Result<impl IntoResponse, AppError> {
    let gallery = sqlx::query_as::<_, Gallery>(
        "UPDATE galleries SET name = $1, updated_at = NOW() WHERE id = $2 RETURNING *"
    )
    .bind(&payload.name)
    .bind(id)
    .fetch_optional(&state.db)
    .await?
    .ok_or(AppError::NotFound)?;

    let aliases = sqlx::query_scalar::<_, String>(
        "SELECT alias FROM gallery_aliases WHERE gallery_id = $1"
    )
    .bind(gallery.id)
    .fetch_all(&state.db)
    .await?;

    Ok(Json(GalleryDetail {
        id: gallery.id,
        name: gallery.name,
        created_at: gallery.created_at,
        updated_at: gallery.updated_at,
        aliases,
    }))
}

// DELETE /galleries/:id
#[utoipa::path(delete, path = "/galleries/{id}", params(("id" = Uuid, Path, description = "Gallery UUID")), responses((status = 204, description = "Gallery unlinked and deleted successfully"), (status = 404, description = "Gallery not found")))]
pub async fn delete_gallery(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
) -> Result<impl IntoResponse, AppError> {
    let rows_affected = sqlx::query("DELETE FROM galleries WHERE id = $1")
        .bind(id)
        .execute(&state.db)
        .await?
        .rows_affected();

    if rows_affected == 0 {
        return Err(AppError::NotFound);
    }

    Ok(StatusCode::NO_CONTENT)
}

// POST /galleries/:id/aliases
#[utoipa::path(post, path = "/galleries/{id}/aliases", params(("id" = Uuid, Path, description = "Gallery UUID")), request_body = AddAliasReq, responses((status = 201, description = "Alias added successfully"), (status = 404, description = "Gallery not found")))]
pub async fn add_gallery_alias(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
    Json(payload): Json<AddAliasReq>,
) -> Result<impl IntoResponse, AppError> {
    // 确保 gallery 存在
    let exists = sqlx::query_scalar::<_, bool>(
        "SELECT EXISTS(SELECT 1 FROM galleries WHERE id = $1)"
    )
    .bind(id)
    .fetch_one(&state.db)
    .await?;

    if !exists {
        return Err(AppError::NotFound);
    }

    sqlx::query("INSERT INTO gallery_aliases (alias, gallery_id) VALUES ($1, $2)")
        .bind(&payload.alias)
        .bind(id)
        .execute(&state.db)
        .await?;

    Ok(StatusCode::CREATED)
}

// DELETE /galleries/:id/aliases/:alias
#[utoipa::path(delete, path = "/galleries/{id}/aliases/{alias}", params(("id" = Uuid, Path, description = "Gallery UUID"), ("alias" = String, Path, description = "Alias to delete")), responses((status = 204, description = "Alias unlinked successfully"), (status = 404, description = "Gallery/Alias not found")))]
pub async fn delete_gallery_alias(
    State(state): State<AppState>,
    Path((id, alias)): Path<(Uuid, String)>,
) -> Result<impl IntoResponse, AppError> {
    let rows_affected = sqlx::query(
        "DELETE FROM gallery_aliases WHERE gallery_id = $1 AND alias = $2"
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
