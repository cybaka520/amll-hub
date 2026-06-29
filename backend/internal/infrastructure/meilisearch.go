package infrastructure

import (
	"fmt"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	"github.com/meilisearch/meilisearch-go"
)

// NewMeiliSearch 初始化 MeiliSearch 客户端
func NewMeiliSearch(cfg config.MeiliSearchConfig) (*meilisearch.Client, error) {
	client := meilisearch.NewClient(meilisearch.ClientConfig{
		Host:   cfg.Host,
		APIKey: cfg.APIKey,
	})

	// 探活
	_, err := client.Health()
	if err != nil {
		return nil, fmt.Errorf("meilisearch health check: %w", err)
	}

	return client, nil
}

// EnsureMeiliSearchIndex 确保索引存在并按规范配置
// 注：Rust Worker 启动时也应调用类似配置；这里在 Go 启动时幂等设置
func EnsureMeiliSearchIndex(client *meilisearch.Client, indexName string) error {
	index := client.Index(indexName)

	// 创建/获取索引，主键 id
	_, err := client.GetIndex(indexName)
	if err != nil {
		_, err = client.CreateIndex(&meilisearch.IndexConfig{
			Uid:        indexName,
			PrimaryKey: "id",
		})
		if err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	searchable := []string{
		"musicNames", "musicNamesPinyin",
		"artists", "artistsPinyin",
		"albums", "albumsPinyin",
		"lyricText",
		"platformIds_ncm", "platformIds_qq", "platformIds_spotify", "platformIds_apple",
	}
	filterable := []string{
		"platformIds_ncm", "platformIds_qq", "platformIds_spotify", "platformIds_apple",
		"artists", "albums", "ttmlAuthorGithub",
	}
	sortable := []string{
		"commitTimestamp",
	}

	if _, err := index.UpdateSearchableAttributes(&searchable); err != nil {
		return fmt.Errorf("update searchable attributes: %w", err)
	}
	if _, err := index.UpdateFilterableAttributes(&filterable); err != nil {
		return fmt.Errorf("update filterable attributes: %w", err)
	}
	if _, err := index.UpdateSortableAttributes(&sortable); err != nil {
		return fmt.Errorf("update sortable attributes: %w", err)
	}

	return nil
}
