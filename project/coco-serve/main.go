package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"coco-serve/internal/handler"
	"coco-serve/internal/k8sclient"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var frontend embed.FS

func main() {
	kc, err := k8sclient.New()
	if err != nil {
		log.Printf("WARNING: K8s client init failed: %v (API will use system data only)", err)
	}
	handler.Init(kc)

	// 启动后台状态监控，每 10 秒采集并推送
	handler.StartWatcher(10 * time.Second)

	r := handler.SetupRouter()

	// 嵌入前端静态文件
	distFS, err := fs.Sub(frontend, "dist")
	if err != nil {
		log.Fatal(err)
	}
	// SPA 模式: 非 API 路由全部返回 index.html
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" {
			data, _ := fs.ReadFile(distFS, "index.html")
			c.Data(http.StatusOK, "text/html", data)
			return
		}
		// 尝试读取静态文件
		f, err := distFS.Open(path[1:])
		if err == nil {
			f.Close()
			c.FileFromFS(path, http.FS(distFS))
			return
		}
		// SPA fallback
		data, _ := fs.ReadFile(distFS, "index.html")
		c.Data(http.StatusOK, "text/html", data)
	})

	log.Println("CoCo Serve starting on 0.0.0.0:8080")
	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatal(err)
	}
}
