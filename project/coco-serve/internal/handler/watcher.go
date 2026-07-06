package handler

import (
	"encoding/json"
	"log"
	"time"

	"coco-serve/internal/collector"
)

// StartWatcher 启动后台监控，每隔 interval 秒采集状态并广播
func StartWatcher(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var lastPodCount int
		var lastTDX bool

		for range ticker.C {
			// TDX 状态变化检测
			tdx := collector.GetTDXStatus()
			if tdx.Enabled != lastTDX {
				lastTDX = tdx.Enabled
				data, _ := json.Marshal(map[string]interface{}{
					"type": "tdx_status",
					"data": tdx,
				})
				WsHub.Broadcast(data)
			}

			// Pod 数量变化检测
			if h != nil && h.K8s != nil {
				pods, err := h.K8s.GetPods()
				if err == nil {
					current := len(pods)
					if current != lastPodCount {
						lastPodCount = current
						data, _ := json.Marshal(map[string]interface{}{
							"type":    "pod_count",
							"count":   current,
							"message": "pod 状态已更新",
						})
						WsHub.Broadcast(data)
						log.Printf("[watcher] pod count changed: %d", current)
					}
				}
			}
		}
	}()

	log.Printf("[watcher] started, interval=%v", interval)
}
