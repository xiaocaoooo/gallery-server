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
    pub fn persist(
        &self,
        temp: NamedTempFile,
        hash_hex: &str,
        ext: &str,
    ) -> std::io::Result<PathBuf> {
        let dest = self.final_path(hash_hex, ext);
        if let Some(parent) = dest.parent() {
            std::fs::create_dir_all(parent)?;
        }

        // Linux 下同文件系统 rename 是原子的，且会覆盖已存在文件
        temp.persist(&dest).map_err(|e| e.error)?;
        Ok(dest)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::tempdir;

    #[test]
    fn test_storage_paths() {
        let base = PathBuf::from("/tmp/gallery_test");
        let storage = Storage::new(&base);

        assert_eq!(storage.tmp_dir, base.join("tmp"));

        let hash_hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
        let expected_path = base
            .join("images")
            .join("0123")
            .join("4567")
            .join("89abcdef0123456789abcdef0123456789abcdef0123456789abcdef.jpg");

        assert_eq!(storage.final_path(hash_hex, "jpg"), expected_path);
    }

    #[test]
    fn test_atomic_persist() {
        let temp_dir = tempdir().unwrap();
        let storage = Storage::new(temp_dir.path());

        // 1. 创建临时文件并写入数据
        let mut temp_file = storage.temp_file().unwrap();
        temp_file.write_all(b"image data").unwrap();

        // 2. 准备哈希和后缀
        let hash_hex = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789";
        let ext = "png";

        let final_path = storage.final_path(hash_hex, ext);
        assert!(!final_path.exists());

        // 3. 持久化移动文件
        let result_path = storage.persist(temp_file, hash_hex, ext).unwrap();
        assert_eq!(result_path, final_path);
        assert!(final_path.exists());

        // 4. 检验内容
        let content = std::fs::read_to_string(&final_path).unwrap();
        assert_eq!(content, "image data");
    }
}
