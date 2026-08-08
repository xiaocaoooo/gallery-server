use crate::error::AppError;
use crate::state::AppState;
use axum::{
    body::Body,
    extract::{Path, State},
    http::{header, Response, StatusCode},
    response::IntoResponse,
};
use uuid::Uuid;

// GET /files/:image_id
#[utoipa::path(get, path = "/files/{image_id}", params(("image_id" = Uuid, Path, description = "Image UUID")), responses((status = 200, description = "Original image physical file stream retrieved successfully"), (status = 404, description = "Image file not found")))]
pub async fn serve_file(
    State(state): State<AppState>,
    Path(image_id): Path<Uuid>,
) -> Result<impl IntoResponse, AppError> {
    let row = sqlx::query("SELECT sha256_hex, ext FROM images WHERE id = $1")
        .bind(image_id)
        .fetch_optional(&state.db)
        .await?
        .ok_or(AppError::NotFound)?;

    use sqlx::Row;
    let sha256_hex: String = row.try_get("sha256_hex")?;
    let ext: String = row.try_get("ext")?;

    let path = state.storage.final_path(&sha256_hex, &ext);

    if !path.exists() {
        return Err(AppError::NotFound);
    }

    let file = tokio::fs::File::open(&path).await?;
    let stream = tokio_util::io::ReaderStream::new(file);
    let body = Body::from_stream(stream);

    let content_type = match ext.as_str() {
        "jpg" | "jpeg" => "image/jpeg",
        "png" => "image/png",
        "gif" => "image/gif",
        "webp" => "image/webp",
        "bmp" => "image/bmp",
        _ => "application/octet-stream",
    };

    let response = Response::builder()
        .status(StatusCode::OK)
        .header(header::CONTENT_TYPE, content_type)
        .body(body)
        .unwrap();

    Ok(response)
}
