package handler

import (
	"encoding/json"
	"time"

	"coco-serve/internal/collector"
	"coco-serve/internal/logger"

	"go.uber.org/zap"
)

// StartWatcher 启动后台监控，每隔 interval 秒采集状态并广播
func StartWatcher(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var lastPodCount int
		var lastTDX bool
		lastPodPhases := map[string]string{} // key: "ns/name" → phase

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

			// Pod 数量及状态变化检测
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
						logger.Pod.Info("pod count changed", zap.Int("count", current))
					}

					// 单 Pod 状态追踪
					for _, p := range pods {
						key := p.Namespace + "/" + p.Name
						prev, exists := lastPodPhases[key]
						if !exists || prev != p.Status {
							lastPodPhases[key] = p.Status
							data, _ := json.Marshal(map[string]interface{}{
								"type":      "pod_phase",
								"name":      p.Name,
								"namespace": p.Namespace,
								"phase":     p.Status,
							})
							WsHub.Broadcast(data)
							if exists {
								logger.Pod.Info("pod phase changed",
									zap.String("name", p.Name),
									zap.String("from", prev),
									zap.String("to", p.Status),
								)
							}
						}
					}
				}
			}
		}
	}()

	logger.System.Info("watcher started", zap.Duration("interval", interval))
}
