package infrastructure

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 封装连接与通道
type RabbitMQ struct {
	Conn    *amqp.Connection
	Channel *amqp.Channel
	Queue   amqp.Queue
	DLQ     amqp.Queue
	NFQueue amqp.Queue // 无歌词解析队列
	NFDLQ   amqp.Queue // 无歌词死信队列
	cfg     config.RabbitMQConfig
}

// 无歌词解析相关常量
const (
	NFExchange    = "ttml.not_found"
	NFRoutingKey  = "not_found.parse"
	NFDLXExchange = "ttml.not_found.dlx"
	NFDLQRouting  = "not_found.failed"
	NFQueueName   = "not_found_parse_queue"
	NFDLQName     = "not_found_parse_queue.dlq"
)

// NewRabbitMQ 初始化 RabbitMQ，声明主队列 + 死信队列
func NewRabbitMQ(cfg config.RabbitMQConfig) (*RabbitMQ, error) {
	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Heartbeat: 30 * time.Second,
		Locale:    "en_US",
	})
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	// 声明死信交换机与队列
	dlxName := "ttml.sync.dlx"
	if err := ch.ExchangeDeclare(
		dlxName, "direct", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare dlx exchange: %w", err)
	}

	dlq, err := ch.QueueDeclare(
		cfg.DLQ, true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("declare dlq: %w", err)
	}
	if err := ch.QueueBind(dlq.Name, "sync.failed", dlxName, false, nil); err != nil {
		return nil, fmt.Errorf("bind dlq: %w", err)
	}

	// 声明主交换机与队列（绑定 DLX）
	exName := "ttml.sync"
	if err := ch.ExchangeDeclare(
		exName, "direct", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare exchange: %w", err)
	}

	queue, err := ch.QueueDeclare(
		cfg.Queue, true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    dlxName,
			"x-dead-letter-routing-key": "sync.failed",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("declare queue: %w", err)
	}
	if err := ch.QueueBind(queue.Name, "sync.request", exName, false, nil); err != nil {
		return nil, fmt.Errorf("bind queue: %w", err)
	}

	// QoS：串行消费
	if err := ch.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("qos: %w", err)
	}

	// === 无歌词解析队列（独立交换机/队列/DLQ） ===
	nfDlxName := NFDLXExchange
	if err := ch.ExchangeDeclare(
		nfDlxName, "direct", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare nf dlx exchange: %w", err)
	}

	nfDlq, err := ch.QueueDeclare(
		NFDLQName, true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("declare nf dlq: %w", err)
	}
	if err := ch.QueueBind(nfDlq.Name, NFDLQRouting, nfDlxName, false, nil); err != nil {
		return nil, fmt.Errorf("bind nf dlq: %w", err)
	}

	nfExName := NFExchange
	if err := ch.ExchangeDeclare(
		nfExName, "direct", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare nf exchange: %w", err)
	}

	nfQueue, err := ch.QueueDeclare(
		NFQueueName, true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    nfDlxName,
			"x-dead-letter-routing-key": NFDLQRouting,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("declare nf queue: %w", err)
	}
	if err := ch.QueueBind(nfQueue.Name, NFRoutingKey, nfExName, false, nil); err != nil {
		return nil, fmt.Errorf("bind nf queue: %w", err)
	}

	return &RabbitMQ{
		Conn:    conn,
		Channel: ch,
		Queue:   queue,
		DLQ:     dlq,
		NFQueue: nfQueue,
		NFDLQ:   nfDlq,
		cfg:     cfg,
	}, nil
}

// PublishSyncRequest 发布一条同步任务到主队列
func (r *RabbitMQ) PublishSyncRequest(body []byte, requestID string, triggeredBy string) error {
	return r.Channel.Publish(
		"ttml.sync",
		"sync.request",
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    requestID,
			Timestamp:    time.Now(),
			Headers: amqp.Table{
				"x-request-id":   requestID,
				"x-triggered-by": triggeredBy,
			},
			Body: body,
		},
	)
}

// NotFoundParseMessage 无歌词解析消息体
type NotFoundParseMessage struct {
	Platform   string `json:"platform"`
	PlatformID string `json:"platformId"`
	ClientIP   string `json:"clientIp"`
}

// PublishNotFoundParse 发布一条无歌词解析任务
func (r *RabbitMQ) PublishNotFoundParse(msg NotFoundParseMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal nf message: %w", err)
	}
	return r.Channel.Publish(
		NFExchange,
		NFRoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			MessageId:    fmt.Sprintf("%s:%s", msg.Platform, msg.PlatformID),
			Body:         body,
		},
	)
}

// Close 关闭连接
func (r *RabbitMQ) Close() error {
	if r.Channel != nil {
		_ = r.Channel.Close()
	}
	if r.Conn != nil {
		return r.Conn.Close()
	}
	return nil
}
