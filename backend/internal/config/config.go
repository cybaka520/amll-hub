package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	HTTP        HTTPConfig
	Database    DatabaseConfig
	Redis       RedisConfig
	MinIO       MinIOConfig
	RabbitMQ    RabbitMQConfig
	MeiliSearch MeiliSearchConfig
	GitHub      GitHubConfig
	Sync        SyncConfig
}

type HTTPConfig struct {
	Port string
}

type DatabaseConfig struct {
	Host         string
	Port         string
	User         string
	Password     string
	Name         string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type RabbitMQConfig struct {
	URL   string
	Queue string
	DLQ   string
}

type MeiliSearchConfig struct {
	Host   string
	APIKey string
	Index  string
}

type GitHubConfig struct {
	Token  string
	Repo   string
	Branch string
}

type SyncConfig struct {
	// Cron 兜底检查间隔（秒）
	CronIntervalSec int
}

// Load 从环境变量加载配置
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// HTTP
	v.SetDefault("PORT", "8080")

	// DB
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_USER", "ttml")
	v.SetDefault("DB_PASSWORD", "ttml")
	v.SetDefault("DB_NAME", "ttml_db")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("DB_MAX_OPEN_CONNS", 50)
	v.SetDefault("DB_MAX_IDLE_CONNS", 10)

	// Redis
	v.SetDefault("REDIS_HOST", "localhost")
	v.SetDefault("REDIS_PORT", "6379")
	v.SetDefault("REDIS_PASSWORD", "")
	v.SetDefault("REDIS_DB", 0)

	// MinIO
	v.SetDefault("MINIO_ENDPOINT", "localhost:9000")
	v.SetDefault("MINIO_ACCESS_KEY", "minioadmin")
	v.SetDefault("MINIO_SECRET_KEY", "minioadmin")
	v.SetDefault("MINIO_BUCKET", "ttml-db")
	v.SetDefault("MINIO_USE_SSL", false)

	// RabbitMQ
	v.SetDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	v.SetDefault("RABBITMQ_QUEUE", "sync_queue")
	v.SetDefault("RABBITMQ_DLQ", "sync_queue.dlq")

	// MeiliSearch
	v.SetDefault("MEILISEARCH_HOST", "http://localhost:7700")
	v.SetDefault("MEILISEARCH_API_KEY", "")
	v.SetDefault("MEILISEARCH_INDEX", "songs")

	// GitHub
	v.SetDefault("GITHUB_TOKEN", "")
	v.SetDefault("GITHUB_REPO", "amll-dev/amll-ttml-db")
	v.SetDefault("GITHUB_BRANCH", "main")

	// Sync
	v.SetDefault("SYNC_CRON_INTERVAL_SEC", 600)

	cfg := &Config{
		HTTP: HTTPConfig{
			Port: v.GetString("PORT"),
		},
		Database: DatabaseConfig{
			Host:         v.GetString("DB_HOST"),
			Port:         v.GetString("DB_PORT"),
			User:         v.GetString("DB_USER"),
			Password:     v.GetString("DB_PASSWORD"),
			Name:         v.GetString("DB_NAME"),
			SSLMode:      v.GetString("DB_SSLMODE"),
			MaxOpenConns: v.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns: v.GetInt("DB_MAX_IDLE_CONNS"),
		},
		Redis: RedisConfig{
			Host:     v.GetString("REDIS_HOST"),
			Port:     v.GetString("REDIS_PORT"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		MinIO: MinIOConfig{
			Endpoint:  v.GetString("MINIO_ENDPOINT"),
			AccessKey: v.GetString("MINIO_ACCESS_KEY"),
			SecretKey: v.GetString("MINIO_SECRET_KEY"),
			Bucket:    v.GetString("MINIO_BUCKET"),
			UseSSL:    v.GetBool("MINIO_USE_SSL"),
		},
		RabbitMQ: RabbitMQConfig{
			URL:   v.GetString("RABBITMQ_URL"),
			Queue: v.GetString("RABBITMQ_QUEUE"),
			DLQ:   v.GetString("RABBITMQ_DLQ"),
		},
		MeiliSearch: MeiliSearchConfig{
			Host:   v.GetString("MEILISEARCH_HOST"),
			APIKey: v.GetString("MEILISEARCH_API_KEY"),
			Index:  v.GetString("MEILISEARCH_INDEX"),
		},
		GitHub: GitHubConfig{
			Token:  v.GetString("GITHUB_TOKEN"),
			Repo:   v.GetString("GITHUB_REPO"),
			Branch: v.GetString("GITHUB_BRANCH"),
		},
		Sync: SyncConfig{
			CronIntervalSec: v.GetInt("SYNC_CRON_INTERVAL_SEC"),
		},
	}

	return cfg, nil
}
