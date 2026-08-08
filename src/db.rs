use crate::hash::perceptual::{hamming_distance, PerceptualHash};
use crate::models::{MatchedImage, PerceptualHashRow};
use sqlx::{Postgres, Transaction};

pub async fn find_perceptual_duplicate(
    tx: &mut Transaction<'_, Postgres>,
    query: &PerceptualHash,
    ahash_threshold: u32,
    dhash_threshold: u32,
    phash_threshold: u32,
) -> Result<Option<MatchedImage>, sqlx::Error> {
    // L1: 前缀分桶（候选集缩小到 ~0.006%）
    let candidates = sqlx::query_as::<_, PerceptualHashRow>(
        "SELECT * FROM perceptual_hashes
         WHERE bucket1 = $1 OR bucket2 = $2 OR bucket3 = $3 OR bucket4 = $4",
    )
    .bind(query.bucket1)
    .bind(query.bucket2)
    .bind(query.bucket3)
    .bind(query.bucket4)
    .fetch_all(&mut **tx)
    .await?;

    let mut best: Option<MatchedImage> = None;
    let mut best_score = f64::INFINITY;

    for cand in candidates {
        // L2: aHash 预筛
        let a_dist = hamming_distance(query.ahash, cand.ahash);
        if a_dist > ahash_threshold * 2 {
            continue;
        }

        // L3: 128bit dHash 精确匹配
        let d_dist = hamming_distance(query.dhash_h, cand.dhash_h)
            + hamming_distance(query.dhash_v, cand.dhash_v);
        if d_dist > dhash_threshold {
            continue;
        }

        // L4: pHash 交叉验证
        let p_dist = hamming_distance(query.phash, cand.phash);
        if p_dist > phash_threshold {
            continue;
        }

        // 综合评分
        let score = d_dist as f64 * 2.0 + p_dist as f64 + a_dist as f64 * 0.5;
        if score < best_score {
            best_score = score;
            best = Some(MatchedImage {
                image_id: cand.image_id,
                score,
                dhash_distance: d_dist,
                phash_distance: p_dist,
            });
        }
    }
    Ok(best)
}
