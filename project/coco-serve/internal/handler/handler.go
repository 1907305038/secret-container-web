package handler

import "coco-serve/internal/k8sclient"

// HandlerContext 被所有 handler 共享
type H struct {
	K8s *k8sclient.Client
}

var h *H

func Init(c *k8sclient.Client) {
	h = &H{K8s: c}
}
