use axum::{http::StatusCode, response::IntoResponse, Json};
use serde_json::json;

#[derive(Debug, thiserror::Error)]
pub enum AppError {
    #[error("Not found")]
    NotFound,

    #[error("Conflict: {0}")]
    Conflict(String),

    #[error("Bad request: {0}")]
    BadRequest(String),

    #[error("Duplicate in gallery")]
    AlreadyInGallery,

    #[error("Perceptual duplicate detected: {image_id}")]
    PerceptualDuplicate { image_id: uuid::Uuid },

    #[error(transparent)]
    Sqlx(#[from] sqlx::Error),

    #[error(transparent)]
    Io(#[from] std::io::Error),

    #[error(transparent)]
    Multipart(#[from] axum::extract::multipart::MultipartError),

    #[error(transparent)]
    Image(#[from] image::ImageError),

    #[error("Task join error: {0}")]
    JoinError(String),
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        let (status, msg) = match &self {
            Self::NotFound => (StatusCode::NOT_FOUND, self.to_string()),
            Self::Conflict(m) => (StatusCode::CONFLICT, m.clone()),
            Self::BadRequest(m) => (StatusCode::BAD_REQUEST, m.clone()),
            Self::AlreadyInGallery => (StatusCode::CONFLICT, self.to_string()),
            Self::PerceptualDuplicate { image_id } => {
                return (
                    StatusCode::CONFLICT,
                    Json(json!({
                        "error": "Perceptual duplicate detected",
                        "image_id": image_id
                    })),
                )
                    .into_response();
            }
            Self::Image(e) => (StatusCode::BAD_REQUEST, format!("Invalid image: {}", e)),
            Self::Multipart(e) => (StatusCode::BAD_REQUEST, format!("Multipart error: {}", e)),
            Self::Sqlx(e) => {
                if let sqlx::Error::Database(db) = e {
                    if db.constraint().is_some() {
                        return (StatusCode::CONFLICT, Json(json!({ "error": db.message() })))
                            .into_response();
                    }
                }
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    format!("Database error: {}", e),
                )
            }
            _ => (StatusCode::INTERNAL_SERVER_ERROR, self.to_string()),
        };
        (status, Json(json!({ "error": msg }))).into_response()
    }
}
