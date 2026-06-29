use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;

/// 确保 bucket 存在
pub async fn ensure_bucket(s3: &S3Client, bucket: &str) -> Result<()> {
    let exists = s3.head_bucket().bucket(bucket).send().await.is_ok();
    if !exists {
        s3.create_bucket()
            .bucket(bucket)
            .send()
            .await
            .with_context(|| format!("create bucket {}", bucket))?;
        tracing::info!(bucket, "created minio bucket");
    }
    Ok(())
}
