package router

import (
	"github.com/amll-dev/amll-hub/backend/internal/handler"
	"github.com/amll-dev/amll-hub/backend/internal/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// New 构建并返回 Gin 引擎
func New(
	syncH *handler.SyncHandler,
	lyricsH *handler.LyricsHandler,
	searchH *handler.SearchHandler,
	batchH *handler.BatchHandler,
	statsH *handler.StatsHandler,
	indexH *handler.IndexHandler,
	nfH *handler.NotFoundHandler,
	onlineSearchH *handler.OnlineSearchHandler,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Range", "Content-Length", "ETag", "X-Request-ID"},
		AllowCredentials: false,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	{
		// 同步触发/状态
		api.POST("/sync", syncH.Trigger)
		api.GET("/sync/status", syncH.Status)

		// 搜索
		api.GET("/search", searchH.Search)
		api.GET("/online-search", onlineSearchH.Search)
		api.GET("/online-song", onlineSearchH.GetSong)
		api.GET("/online-lyric", onlineSearchH.GetLyric)

		// 批量查询
		api.POST("/batch", batchH.Post)

		// 词库统计
		api.GET("/stats", statsH.Get)

		// 索引文件下载
		api.GET("/index/*path", indexH.GetIndex)

		// 无歌词记录系统
		api.GET("/not-found-ranking", nfH.GetRanking)
		api.GET("/not-found-stats", nfH.GetStats)
		api.GET("/pure-music-whitelist", nfH.ListPureMusicWhitelist)
		api.GET("/cloud-music-whitelist", nfH.ListCloudMusicWhitelist)

		// 歌词获取（注意：放在最末，避免与上面具名路由冲突）
		api.GET("/:folder/:filename", lyricsH.GetLyrics)
	}

	return r
}
