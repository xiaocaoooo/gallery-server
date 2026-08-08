pub mod gallery;
pub mod image;
pub mod gallery_image;
pub mod file;
pub mod refresh;

use axum::{
    routing::{get, post, delete},
    Router,
};
use utoipa::OpenApi;
use utoipa_swagger_ui::SwaggerUi;
use crate::state::AppState;
use crate::models::*;

#[derive(OpenApi)]
#[openapi(
    paths(
        gallery::create_gallery,
        gallery::list_galleries,
        gallery::get_gallery,
        gallery::update_gallery,
        gallery::delete_gallery,
        gallery::add_gallery_alias,
        gallery::delete_gallery_alias,

        image::list_images,
        image::get_image,
        image::get_image_by_hash_prefix,
        image::update_image,
        image::add_image_alias,
        image::delete_image_alias,

        gallery_image::upload_image,
        gallery_image::list_gallery_images,
        gallery_image::remove_image_from_gallery,

        file::serve_file,
        refresh::refresh_tasks,
    ),
    components(
        schemas(
            CreateGalleryReq,
            UpdateGalleryReq,
            AddAliasReq,
            UpdateImageReq,
            Gallery,
            GalleryDetail,
            Image,
            ImageDetail,
            RefreshReq,
            PerceptualHashRow,
            MatchedImage
        )
    ),
    tags(
        (name = "gallery-service", description = "High-Performance Image Gallery & Perceptual Hashing duplicate check microservice")
    )
)]
pub struct ApiDoc;

pub fn all_routes() -> Router<AppState> {
    Router::new()
        // Swagger UI
        .merge(SwaggerUi::new("/swagger-ui").url("/api-docs/openapi.json", ApiDoc::openapi()))

        // 画廊管理
        .route("/galleries", post(gallery::create_gallery).get(gallery::list_galleries))
        .route("/galleries/:id", get(gallery::get_gallery).patch(gallery::update_gallery).delete(gallery::delete_gallery))
        .route("/galleries/:id/aliases", post(gallery::add_gallery_alias))
        .route("/galleries/:id/aliases/:alias", delete(gallery::delete_gallery_alias))

        // 全局图片管理
        .route("/images", get(image::list_images))
        .route("/images/:id", get(image::get_image).patch(image::update_image))
        .route("/images/by-hash/:prefix", get(image::get_image_by_hash_prefix))
        .route("/images/:id/aliases", post(image::add_image_alias))
        .route("/images/:id/aliases/:alias", delete(image::delete_image_alias))

        // 画廊-图片关联
        .route("/galleries/:gallery_id/images", post(gallery_image::upload_image).get(gallery_image::list_gallery_images))
        .route("/galleries/:gallery_id/images/:image_id", delete(gallery_image::remove_image_from_gallery))

        // 文件检索
        .route("/files/:image_id", get(file::serve_file))

        // 刷新与手工调度
        .route("/refresh", post(refresh::refresh_tasks))
}
