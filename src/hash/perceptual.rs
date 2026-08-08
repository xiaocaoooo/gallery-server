use image::{DynamicImage, GrayImage};
use rustdct::DctPlanner;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PerceptualHash {
    pub ahash: i64,
    pub dhash_h: i64,
    pub dhash_v: i64,
    pub phash: i64,
    pub bucket1: i32,
    pub bucket2: i32,
    pub bucket3: i32,
    pub bucket4: i32,
    pub frame_hashes: Vec<(i64, i64)>, // GIF 多帧，静态图为空
}

pub fn compute(img: &DynamicImage) -> PerceptualHash {
    let gray = img.to_luma8();
    
    let ahash = compute_ahash(&gray);
    let dhash_h = compute_dhash_h(&gray);
    let dhash_v = compute_dhash_v(&gray);
    let phash = compute_phash(img);
    
    let buckets = get_buckets(dhash_h);
    
    PerceptualHash {
        ahash, dhash_h, dhash_v, phash,
        bucket1: buckets.0, bucket2: buckets.1,
        bucket3: buckets.2, bucket4: buckets.3,
        frame_hashes: vec![],
    }
}

fn compute_ahash(gray: &GrayImage) -> i64 {
    let resized = image::imageops::resize(gray, 8, 8, image::imageops::FilterType::Lanczos3);
    let pixels: Vec<u8> = resized.pixels().map(|p| p[0]).collect();
    let avg = pixels.iter().map(|&p| p as u32).sum::<u32>() as f32 / 64.0;
    
    let mut hash = 0i64;
    for p in pixels {
        hash = (hash << 1) | if p as f32 >= avg { 1 } else { 0 };
    }
    hash
}

fn compute_dhash_h(gray: &GrayImage) -> i64 {
    let resized = image::imageops::resize(gray, 9, 8, image::imageops::FilterType::Lanczos3);
    let mut hash = 0i64;
    for y in 0..8 {
        for x in 0..8 {
            let left = resized.get_pixel(x, y)[0];
            let right = resized.get_pixel(x + 1, y)[0];
            hash = (hash << 1) | if left < right { 1 } else { 0 };
        }
    }
    hash
}

fn compute_dhash_v(gray: &GrayImage) -> i64 {
    let resized = image::imageops::resize(gray, 8, 9, image::imageops::FilterType::Lanczos3);
    let mut hash = 0i64;
    for y in 0..8 {
        for x in 0..8 {
            let top = resized.get_pixel(x, y)[0];
            let bottom = resized.get_pixel(x, y + 1)[0];
            hash = (hash << 1) | if top < bottom { 1 } else { 0 };
        }
    }
    hash
}

fn compute_phash(img: &DynamicImage) -> i64 {
    let gray = img.to_luma8();
    let resized = image::imageops::resize(&gray, 32, 32, image::imageops::FilterType::Lanczos3);
    
    // 转为 f32 矩阵
    let mut matrix = vec![0f32; 32 * 32];
    for (i, p) in resized.pixels().enumerate() {
        matrix[i] = p[0] as f32;
    }
    
    // 2D DCT（先行后列）
    let mut planner = DctPlanner::new();
    let dct32 = planner.plan_dct2(32);
    
    let mut temp = vec![0f32; 32 * 32];
    let mut row = vec![0f32; 32];
    let mut col = vec![0f32; 32];
    
    // 行变换
    for y in 0..32 {
        row.copy_from_slice(&matrix[y * 32..(y + 1) * 32]);
        let mut row_buf = temp[y * 32..(y + 1) * 32].to_vec();
        dct32.process_dct2(&mut row_buf);
        temp[y * 32..(y + 1) * 32].copy_from_slice(&row_buf);
    }
    
    // 列变换
    for x in 0..32 {
        for y in 0..32 { col[y] = temp[y * 32 + x]; }
        dct32.process_dct2(&mut col);
        for y in 0..32 { matrix[y * 32 + x] = col[y]; }
    }
    
    // 取 8x8 低频（排除 0,0）
    let mut lowfreq = [[0f32; 8]; 8];
    for y in 0..8 {
        for x in 0..8 {
            lowfreq[y][x] = matrix[y * 32 + x];
        }
    }
    
    let sum = lowfreq.iter().flatten().sum::<f32>() - lowfreq[0][0];
    let avg = sum / 63.0;
    
    let mut hash = 0i64;
    for y in 0..8 {
        for x in 0..8 {
            if y == 0 && x == 0 { continue; }
            hash = (hash << 1) | if lowfreq[y][x] >= avg { 1 } else { 0 };
        }
    }
    hash
}

pub fn get_buckets(hash: i64) -> (i32, i32, i32, i32) {
    (
        ((hash >> 48) & 0xFFFF) as i32,
        ((hash >> 32) & 0xFFFF) as i32,
        ((hash >> 16) & 0xFFFF) as i32,
        (hash & 0xFFFF) as i32,
    )
}

pub fn hamming_distance(a: i64, b: i64) -> u32 {
    (a ^ b).count_ones()
}

use image::AnimationDecoder;

pub fn compute_gif(data: &[u8]) -> Result<PerceptualHash, image::ImageError> {
    let decoder = image::codecs::gif::GifDecoder::new(std::io::Cursor::new(data))?;
    let frames: Vec<_> = decoder.into_frames().collect_frames()?;
    
    let n_frames = frames.len();
    let sample_count = n_frames.min(5);
    let indices: Vec<usize> = (0..sample_count)
        .map(|i| i * (n_frames - 1) / sample_count.max(1))
        .collect();
    
    let mut frame_hashes = Vec::with_capacity(sample_count);
    let mut composite_h = 0i64;
    let mut composite_v = 0i64;
    
    for idx in indices {
        let frame = &frames[idx];
        let rgb = DynamicImage::ImageRgba8(frame.buffer().clone()).to_rgb8();
        let gray = DynamicImage::ImageRgb8(rgb).to_luma8();
        
        let dh = compute_dhash_h(&gray);
        let dv = compute_dhash_v(&gray);
        frame_hashes.push((dh, dv));
        composite_h ^= dh;
        composite_v ^= dv;
    }
    
    // GIF 使用综合 hash 作为主键存入 dhash_h/dhash_v 字段
    let first_frame_buffer = frames[0].buffer().clone();
    let mut hash = compute(&DynamicImage::ImageRgba8(first_frame_buffer)); // 首帧用于 ahash/phash
    hash.dhash_h = composite_h;
    hash.dhash_v = composite_v;
    hash.frame_hashes = frame_hashes;
    
    Ok(hash)
}
