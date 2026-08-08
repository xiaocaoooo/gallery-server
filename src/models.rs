use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use utoipa::ToSchema;
use uuid::Uuid;

// DTO structures for Requests and Responses

#[derive(Debug, Deserialize, ToSchema)]
pub struct CreateGalleryReq {
    pub name: String,
    pub aliases: Option<Vec<String>>,
}

#[derive(Debug, Deserialize, ToSchema)]
pub struct UpdateGalleryReq {
    pub name: String,
}

#[derive(Debug, Deserialize, ToSchema)]
pub struct AddAliasReq {
    pub alias: String,
}

#[derive(Debug, Deserialize, ToSchema)]
pub struct UpdateImageReq {
    pub name: Option<String>,
    pub metadata: Option<serde_json::Value>,
}

#[derive(Debug, Serialize, sqlx::FromRow, ToSchema)]
pub struct Gallery {
    pub id: Uuid,
    pub name: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, ToSchema)]
pub struct GalleryDetail {
    pub id: Uuid,
    pub name: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub aliases: Vec<String>,
}

#[derive(Debug, Serialize, sqlx::FromRow, ToSchema)]
pub struct Image {
    pub id: Uuid,
    pub sha256_hex: String,
    pub name: Option<String>,
    pub ext: String,
    pub width: i32,
    pub height: i32,
    pub size_bytes: i64,
    pub metadata: serde_json::Value,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, ToSchema)]
pub struct ImageDetail {
    pub id: Uuid,
    pub sha256_hex: String,
    pub name: Option<String>,
    pub ext: String,
    pub width: i32,
    pub height: i32,
    pub size_bytes: i64,
    pub metadata: serde_json::Value,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub aliases: Vec<String>,
}

#[derive(Debug, Deserialize, ToSchema)]
pub struct RefreshReq {
    pub clear_orphans: Option<bool>,
    pub refresh_hashes: Option<bool>,
    pub clear_temp: Option<bool>,
}

#[derive(Debug, sqlx::FromRow, ToSchema)]
#[allow(dead_code)]
pub struct PerceptualHashRow {
    pub image_id: Uuid,
    pub is_gif: bool,
    pub frame_count: i32,
    pub ahash: i64,
    pub dhash_h: i64,
    pub dhash_v: i64,
    pub phash: i64,
    pub bucket1: i32,
    pub bucket2: i32,
    pub bucket3: i32,
    pub bucket4: i32,
    pub frame_hashes: serde_json::Value,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, ToSchema)]
pub struct MatchedImage {
    pub image_id: Uuid,
    pub score: f64,
    pub dhash_distance: u32,
    pub phash_distance: u32,
}
