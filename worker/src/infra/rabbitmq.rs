use anyhow::{Context, Result};
use lapin::{
    options::{BasicQosOptions, ExchangeDeclareOptions, QueueBindOptions, QueueDeclareOptions},
    types::{AMQPValue, FieldTable},
    Channel, Connection, ConnectionProperties, ExchangeKind,
};

use crate::config::Config;

/// 初始化 RabbitMQ 连接与队列声明
pub struct RabbitMq {
    #[allow(dead_code)]
    pub conn: Connection,
    pub channel: Channel,
}

impl RabbitMq {
    #[allow(dead_code)]
    pub async fn channel(&self) -> Result<Channel> {
        let ch = self.conn.create_channel().await.context("create channel")?;
        Ok(ch)
    }
}

pub async fn init_rabbitmq(cfg: &Config) -> Result<RabbitMq> {
    let conn = Connection::connect(&cfg.rabbitmq.url, ConnectionProperties::default())
        .await
        .context("connect rabbitmq")?;

    let channel = conn.create_channel().await.context("create channel")?;

    // DLX exchange + queue
    let dlx = "ttml.sync.dlx";
    channel
        .exchange_declare(
            dlx,
            ExchangeKind::Direct,
            ExchangeDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare dlx exchange")?;

    channel
        .queue_declare(
            &cfg.rabbitmq.dlq,
            QueueDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare dlq")?;
    channel
        .queue_bind(
            &cfg.rabbitmq.dlq,
            dlx,
            "sync.failed",
            QueueBindOptions::default(),
            FieldTable::default(),
        )
        .await
        .context("bind dlq")?;

    // 主 exchange + queue（绑定 DLX）
    let ex = "ttml.sync";
    channel
        .exchange_declare(
            ex,
            ExchangeKind::Direct,
            ExchangeDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare exchange")?;

    let mut args = FieldTable::default();
    args.insert(
        "x-dead-letter-exchange".into(),
        AMQPValue::LongString("ttml.sync.dlx".into()),
    );
    args.insert(
        "x-dead-letter-routing-key".into(),
        AMQPValue::LongString("sync.failed".into()),
    );
    channel
        .queue_declare(
            &cfg.rabbitmq.queue,
            QueueDeclareOptions {
                durable: true,
                ..Default::default()
            },
            args,
        )
        .await
        .context("declare queue")?;
    channel
        .queue_bind(
            &cfg.rabbitmq.queue,
            ex,
            "sync.request",
            QueueBindOptions::default(),
            FieldTable::default(),
        )
        .await
        .context("bind queue")?;

    channel
        .basic_qos(1, BasicQosOptions { global: false })
        .await
        .context("qos")?;

    // === not_found 解析队列（独立交换机/队列/DLQ） ===
    let nf_dlx = "ttml.not_found.dlx";
    channel
        .exchange_declare(
            nf_dlx,
            ExchangeKind::Direct,
            ExchangeDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare nf dlx exchange")?;

    channel
        .queue_declare(
            &cfg.rabbitmq.nf_dlq,
            QueueDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare nf dlq")?;
    channel
        .queue_bind(
            &cfg.rabbitmq.nf_dlq,
            nf_dlx,
            "not_found.failed",
            QueueBindOptions::default(),
            FieldTable::default(),
        )
        .await
        .context("bind nf dlq")?;

    let nf_ex = "ttml.not_found";
    channel
        .exchange_declare(
            nf_ex,
            ExchangeKind::Direct,
            ExchangeDeclareOptions {
                durable: true,
                ..Default::default()
            },
            FieldTable::default(),
        )
        .await
        .context("declare nf exchange")?;

    let mut nf_args = FieldTable::default();
    nf_args.insert(
        "x-dead-letter-exchange".into(),
        AMQPValue::LongString(nf_dlx.into()),
    );
    nf_args.insert(
        "x-dead-letter-routing-key".into(),
        AMQPValue::LongString("not_found.failed".into()),
    );
    channel
        .queue_declare(
            &cfg.rabbitmq.nf_queue,
            QueueDeclareOptions {
                durable: true,
                ..Default::default()
            },
            nf_args,
        )
        .await
        .context("declare nf queue")?;
    channel
        .queue_bind(
            &cfg.rabbitmq.nf_queue,
            nf_ex,
            "not_found.parse",
            QueueBindOptions::default(),
            FieldTable::default(),
        )
        .await
        .context("bind nf queue")?;

    Ok(RabbitMq { conn, channel })
}
