package handler

import (
	"coco-serve/internal/ws"

	"github.com/gin-gonic/gin"
)

var WsHub = ws.NewHub()

func SetupRouter() *gin.Engine {
	r := gin.Default()

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		api.GET("/overview", GetOverview)
		api.GET("/pods", GetPods)
		api.POST("/pods/create", CreatePod)
		api.DELETE("/pods/:namespace/:name", DeletePod)
		api.GET("/pods/:namespace/:name/logs", GetPodLogs)
		api.GET("/pods/info/:namespace/:name", GetPodSysInfo)
		api.GET("/pods/:namespace/:name/yaml", GetPodYaml)
		api.GET("/pods/:namespace/:name/events", GetPodEvents)
		api.GET("/proc/:pid/mem", GetProcMem)
		api.GET("/runtimes", GetRuntimes)
		api.GET("/runtimes/:name", GetRuntimeDetail)
		api.GET("/trustee", GetTrustee)

		// 内存加密验证
		demo := api.Group("/demo")
		{
			demo.GET("/memory-encrypt", GetMemoryEncryptProof) // 全自动
			demo.GET("/memory-compare", GetMemoryCompare)      // 半自动
		}
	}

	// WebSocket 实时状态推送
	r.GET("/ws/state", func(c *gin.Context) {
		WsHub.HandleWS(c.Writer, c.Request)
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	return r
}
