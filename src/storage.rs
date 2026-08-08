use std::path::{Path, PathBuf};
use tempfile::NamedTempFile;

#[derive(Clone)]
pub struct Storage {
    pub base_dir: PathBuf,
    pub tmp_dir: PathBuf,
}

impl Storage {
    pub fn new(base: impl AsRef<Path>) -> Self {
        let base = base.as_ref().to_path_buf();
        Self {
            tmp_dir: base.join("tmp"),
            base_dir: base,
        }
    }

    /// 创建临时文件（在同一文件系统内，保证 rename 原子性）
    pub fn temp_file(&self) -> std::io::Result<NamedTempFile> {
        std::fs::create_dir_all(&self.tmp_dir)?;
        NamedTempFile::new_in(&self.tmp_dir)
    }

    /// 生成最终路径: ./data/images/<h[0:4]>/<h[4:8]>/<h[8:]>.<ext>
    pub fn final_path(&self, hash_hex: &str, ext: &str) -> PathBuf {
        let p1 = &hash_hex[0..4];
        let p2 = &hash_hex[4..8];
        let rest = &hash_hex[8..];
        self.base_dir
            .join("images")
            .join(p1)
            .join(p2)
            .join(format!("{}.{}", rest, ext))
    }

    /// 原子移动。如果目标已存在（相同 SHA256），静默成功。
    pub fn persist(&self, temp: NamedTempFile, hash_hex: &str, ext: &str) -> std::io::Result<PathBuf> {
        let dest = self.final_path(hash_hex, ext);
        if let Some(parent) = dest.parent() {
            std::fs::create_dir_all(parent)?;
        }
        
        // Linux 下同文件系统 rename 是原子的，且会覆盖已存在文件
        temp.persist(&dest).map_err(|e| e.error)?;
        Ok(dest)
    }
}
