use std::sync::Arc;

use anyhow::{Context, Result};
use futures_lite::StreamExt;
use lapin::{
    options::{BasicAckOptions, BasicConsumeOptions},
    types::FieldTable,
    Channel,
};
use tracing::{error, info};

use crate::app::AppState;

use super::sync_task::SyncTaskRunner;

/// 启动 RabbitMQ 消费循环
///
/// 返回时表示收到 shutdown 信号或消费者异常
pub async fn consume_loop(
    channel: Channel,
    queue_name: String,
    app: Arc<AppState>,
    shutdown: Arc<tokio::sync::Notify>,
) -> Result<()> {
    let mut consumer = channel
        .basic_consume(
            &queue_name,
            "ttml-worker",
            BasicConsumeOptions::default(),
            FieldTable::default(),
        )
        .await
        .context("basic_consume")?;

    info!(queue = %queue_name, "consumer started");

    let notified = shutdown.notified();
    tokio::pin!(notified);
    loop {
        tokio::select! {
            _ = &mut notified => {
                info!("shutdown signal received, stopping consumer");
                break;
            }
            msg = consumer.next() => {
                let Some(delivery) = msg else { break; };
                let delivery = match delivery {
                    Ok(d) => d,
                    Err(e) => {
                        error!(error = %e, "consumer error");
                        break;
                    }
                };
                // handle_message 始终返回 Ok（失败也 ACK 以避免队列阻塞），
                // 因此无需 nack 分支
                let _ = handle_message(&channel, delivery, &app).await;
            }
        }
    }

    Ok(())
}

async fn handle_message(
    channel: &Channel,
    delivery: lapin::message::Delivery,
    app: &Arc<AppState>,
) -> Result<()> {
    let tag = delivery.delivery_tag;
    let body = &delivery.data;
    let request_id = delivery
        .properties
        .correlation_id()
        .as_ref()
        .map(|s| s.as_str().to_string())
        .or_else(|| {
            delivery
                .properties
                .message_id()
                .as_ref()
                .map(|s| s.as_str().to_string())
        })
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    let triggered_by = delivery
        .properties
        .headers()
        .as_ref()
        .and_then(|h| h.inner().get("x-triggered-by"))
        .and_then(|v| v.as_long_string())
        .map(|s| String::from_utf8_lossy(s.as_bytes()).to_string())
        .unwrap_or_else(|| "api".to_string());

    let payload: serde_json::Value = serde_json::from_slice(body).unwrap_or_else(|_| {
        serde_json::json!({
            "request_id": request_id,
            "triggered_by": triggered_by,
        })
    });

    info!(request_id = %request_id, triggered_by = %triggered_by, "received sync message");

    let runner = SyncTaskRunner::new(app.clone());
    let result = runner.run(&request_id, &triggered_by, &payload).await;

    match result {
        Ok(skipped) => {
            if skipped {
                info!(request_id = %request_id, "sync skipped (already up-to-date)");
            }
            let _ = channel
                .basic_ack(tag, BasicAckOptions::default())
                .await
                .context("basic_ack");
            Ok(())
        }
        Err(e) => {
            error!(request_id = %request_id, error = %e, "sync task failed");
            // 失败也 ACK，由 sync_history 内部记录 failed 状态；
            // 避免无限重试导致队列阻塞
            let _ = channel
                .basic_ack(tag, BasicAckOptions::default())
                .await
                .context("basic_ack");
            Ok(())
        }
    }
}
