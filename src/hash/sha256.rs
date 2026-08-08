use sha2::{Digest, Sha256};
use std::io::{self, Write};

pub struct StreamSha256 {
    hasher: Sha256,
    size: u64,
}

impl StreamSha256 {
    pub fn new() -> Self {
        Self {
            hasher: Sha256::new(),
            size: 0,
        }
    }

    pub fn update(&mut self, data: &[u8]) {
        self.hasher.update(data);
        self.size += data.len() as u64;
    }

    pub fn finalize(self) -> (Vec<u8>, String, u64) {
        let result = self.hasher.finalize();
        let bytes = result.to_vec();
        let hex_str = hex::encode(&bytes);
        (bytes, hex_str, self.size)
    }
}

impl Write for StreamSha256 {
    fn write(&mut self, buf: &[u8]) -> io::Result<usize> {
        self.update(buf);
        Ok(buf.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}
