use std::env;

#[derive(Debug, Clone)]
pub struct Config {
    pub bind_address: String,
    pub data_dir: std::path::PathBuf,
    pub tmp_dir: std::path::PathBuf,
    pub database_url: String,
    pub database_max_connections: u32,
    pub database_ssl_mode: String,
    pub request_body_limit_mb: usize,
    pub auto_migrate: bool,
    pub orphan_cleanup_interval_hours: u64,
    pub perceptual: PerceptualConfig,
    pub log_format: LogFormat,
}

#[derive(Debug, Clone)]
pub struct PerceptualConfig {
    pub ahash_threshold: u32,
    pub dhash_threshold: u32,
    pub phash_threshold: u32,
}

#[derive(Debug, Clone)]
pub enum LogFormat {
    Json,
    Pretty,
}

impl Config {
    pub fn from_env() -> Result<Self, String> {
        let data_dir = env::var("DATA_DIR").unwrap_or_else(|_| "data".to_string());
        let data_dir = std::path::PathBuf::from(data_dir);

        let tmp_dir = env::var("TMP_DIR")
            .map(std::path::PathBuf::from)
            .unwrap_or_else(|_| data_dir.join("tmp"));

        let log_format = match env::var("LOG_FORMAT").unwrap_or_else(|_| "pretty".to_string()).as_str() {
            "json" => LogFormat::Json,
            _ => LogFormat::Pretty,
        };

        Ok(Self {
            bind_address: env::var("BIND_ADDRESS").unwrap_or_else(|_| "0.0.0.0:3000".to_string()),
            data_dir: data_dir.clone(),
            tmp_dir,
            database_url: env::var("DATABASE_URL")
                .unwrap_or_else(|_| "postgres://gallery:changeme@localhost:5432/gallery".to_string()),
            database_max_connections: env::var("DATABASE_MAX_CONNECTIONS")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(10),
            database_ssl_mode: env::var("DATABASE_SSL_MODE").unwrap_or_else(|_| "prefer".to_string()),
            request_body_limit_mb: env::var("REQUEST_BODY_LIMIT_MB")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(100),
            auto_migrate: env::var("AUTO_MIGRATE")
                .ok()
                .map(|s| s == "true")
                .unwrap_or(true),
            orphan_cleanup_interval_hours: env::var("ORPHAN_CLEANUP_INTERVAL_HOURS")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(24),
            perceptual: PerceptualConfig {
                ahash_threshold: env::var("PERCEPTUAL_AHASH_THRESHOLD")
                    .ok()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(10),
                dhash_threshold: env::var("PERCEPTUAL_DHASH_THRESHOLD")
                    .ok()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(20),
                phash_threshold: env::var("PERCEPTUAL_PHASH_THRESHOLD")
                    .ok()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or(15),
            },
            log_format,
        })
    }

    /// 验证启动条件：确保 tmp 和 images 在同一文件系统
    pub fn validate(&self) -> Result<(), String> {
        if !self.data_dir.exists() {
            std::fs::create_dir_all(&self.data_dir)
                .map_err(|e| format!("Failed to create data_dir: {}", e))?;
        }
        if !self.tmp_dir.exists() {
            std::fs::create_dir_all(&self.tmp_dir)
                .map_err(|e| format!("Failed to create tmp_dir: {}", e))?;
        }

        // 检查 tmp 和 images 是否在同一挂载点
        let images_dir = self.data_dir.join("images");
        if !images_dir.exists() {
            std::fs::create_dir_all(&images_dir)
                .map_err(|e| format!("Failed to create images dir: {}", e))?;
        }

        // 通过创建测试文件验证 rename 原子性
        let test_tmp = self.tmp_dir.join(".fs_check_tmp");
        let test_dest = images_dir.join(".fs_check_dest");
        std::fs::write(&test_tmp, b"test").map_err(|e| format!("FS check failed: {}", e))?;
        std::fs::rename(&test_tmp, &test_dest).map_err(|e| {
            let _ = std::fs::remove_file(&test_tmp);
            format!("tmp_dir and images_dir must be on the same filesystem: {}", e)
        })?;
        let _ = std::fs::remove_file(&test_dest);

        Ok(())
    }
}
