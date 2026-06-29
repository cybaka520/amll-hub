package infrastructure

import (
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
	cfg     config.RabbitMQConfig
}

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

	return &RabbitMQ{
		Conn:    conn,
		Channel: ch,
		Queue:   queue,
		DLQ:     dlq,
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
				"x-request-id":    requestID,
				"x-triggered-by":  triggeredBy,
			},
			Body: body,
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
