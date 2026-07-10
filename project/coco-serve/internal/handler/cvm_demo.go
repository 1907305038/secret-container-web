package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"coco-serve/internal/collector"
	"coco-serve/internal/model"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cvmDemoName      = "cvm-demo"
	cvmDemoNamespace = "default"
	cvmDemoImage     = "docker.m.daocloud.io/library/nginx:alpine"
	cvmDemoLabelKey  = "app"
	cvmDemoLabelVal  = "cvm-demo"
	cvmManagedByKey  = "managed-by"
	cvmManagedByVal  = "coco-panel"
)

// cocoRuntimeClasses 需要检测的 CoCo/Kata RuntimeClass 候选列表
var cocoRuntimeClasses = []string{
	"kata-qemu-tdx",
	"kata-qemu-snp",
	"kata-qemu-coco-dev",
	"kata-qemu",
	"kata",
	"kata-clh",
}

// GetCVMDemoStatus 返回 CVM Demo 的状态总览
func GetCVMDemoStatus(c *gin.Context) {
	status := model.CVMDemoStatus{
		TeeMode: "unknown",
		Message: "正在检测环境...",
	}

	// 1. 检测 TEE 模式
	tdx := collector.GetTDXStatus()
	if tdx.Enabled && tdx.ModuleInitialized {
		status.TeeMode = "real"
	} else {
		status.TeeMode = "dev"
	}

	// 如果 K8s 不可用，直接返回
	if h == nil || h.K8s == nil {
		status.Message = "K8s 集群未连接，无法检测 RuntimeClass"
		c.JSON(http.StatusOK, status)
		return
	}

	// 2. 获取 RuntimeClass 列表并检测 CoCo/Kata
	classes, err := h.K8s.GetRuntimeClasses()
	if err != nil {
		status.Message = fmt.Sprintf("获取 RuntimeClass 失败: %v", err)
		c.JSON(http.StatusOK, status)
		return
	}

	// 构建 RuntimeClass 名称集合，用于快速查找
	rcSet := map[string]bool{}
	for _, rc := range classes {
		rcSet[rc.Name] = true
	}

	// 按优先级检测可用的 CoCo/Kata RuntimeClass
	for _, candidate := range cocoRuntimeClasses {
		if rcSet[candidate] {
			status.AvailableClasses = append(status.AvailableClasses, candidate)
			if status.SelectedRuntimeClass == "" {
				status.SelectedRuntimeClass = candidate
			}
		}
	}

	if status.SelectedRuntimeClass != "" {
		status.RuntimeClassDetected = true
	} else {
		status.RuntimeClassDetected = false
		status.Message = "未检测到 CoCo / Kata RuntimeClass，启动按钮不可用"
	}

	// 3. 检查 Demo Pod 是否存在
	pods, err := h.K8s.GetPods()
	if err == nil {
		for _, p := range pods {
			if p.Name == cvmDemoName && p.Namespace == cvmDemoNamespace {
				status.DemoExists = true
				status.DemoStatus = p.Status
				status.DemoPodName = p.Name
				break
			}
		}
		// 如果 GetPods 没找到（可能被过滤掉了），尝试用 API 直接查
		if !status.DemoExists {
			if demoPod, getErr := h.K8s.GetPod(cvmDemoNamespace, cvmDemoName); getErr == nil {
				status.DemoExists = true
				status.DemoStatus = string(demoPod.Status.Phase)
				status.DemoPodName = demoPod.Name
				if demoPod.Spec.NodeName != "" {
					status.DemoNode = demoPod.Spec.NodeName
				}
			}
		}
	}

	// 4. 构建友好消息
	if status.DemoExists {
		status.Message = fmt.Sprintf("Demo Pod %s 当前状态: %s", status.DemoPodName, status.DemoStatus)
	} else if status.RuntimeClassDetected {
		status.Message = fmt.Sprintf("已检测到 RuntimeClass: %s，可以启动 Demo", status.SelectedRuntimeClass)
	}

	c.JSON(http.StatusOK, status)
}

// StartCVMDemo 创建固定的 CVM Demo Pod
func StartCVMDemo(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "K8s 集群不可用"})
		return
	}

	// 1. 检测可用的 CoCo/Kata RuntimeClass
	classes, err := h.K8s.GetRuntimeClasses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取 RuntimeClass 失败: %v", err)})
		return
	}

	rcSet := map[string]bool{}
	for _, rc := range classes {
		rcSet[rc.Name] = true
	}

	var selectedRC string
	for _, candidate := range cocoRuntimeClasses {
		if rcSet[candidate] {
			selectedRC = candidate
			break
		}
	}
	if selectedRC == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "未检测到 CoCo / Kata RuntimeClass",
			"message": "请先安装 kata-containers 或配置 CoCo RuntimeClass",
		})
		return
	}

	// 2. 检查 Demo Pod 是否已存在
	existingPod, getErr := h.K8s.GetPod(cvmDemoNamespace, cvmDemoName)
	if getErr == nil && existingPod != nil {
		// 已存在，直接返回当前状态（幂等）
		c.JSON(http.StatusOK, gin.H{
			"status":        "already_exists",
			"message":       fmt.Sprintf("Demo Pod 已存在，当前状态: %s", existingPod.Status.Phase),
			"pod_name":      existingPod.Name,
			"pod_status":    string(existingPod.Status.Phase),
			"runtime_class": selectedRC,
			"node":          existingPod.Spec.NodeName,
		})
		return
	}

	// 3. 创建 Demo Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cvmDemoName,
			Namespace: cvmDemoNamespace,
			Labels: map[string]string{
				cvmDemoLabelKey: cvmDemoLabelVal,
				cvmManagedByKey: cvmManagedByVal,
				"demo-type":     "confidential-vm",
			},
		},
		Spec: corev1.PodSpec{
			RuntimeClassName: &selectedRC,
			Containers: []corev1.Container{{
				Name:  "app",
				Image: cvmDemoImage,
			}},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	created, err := h.K8s.CreatePod(pod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   fmt.Sprintf("创建 Demo Pod 失败: %v", err),
			"message": "请检查 K8s 集群状态和 RuntimeClass 配置",
		})
		return
	}

	// 4. 通过 WebSocket 广播创建事件
	evt, _ := json.Marshal(map[string]interface{}{
		"type":      "pod_created",
		"name":      created.Name,
		"namespace": created.Namespace,
		"runtime":   selectedRC,
		"image":     cvmDemoImage,
		"message":   fmt.Sprintf("CVM Demo Pod %s 已创建", created.Name),
	})
	WsHub.Broadcast(evt)

	c.JSON(http.StatusOK, gin.H{
		"status":        "created",
		"message":       fmt.Sprintf("Demo Pod %s 已创建，Runtime: %s", created.Name, selectedRC),
		"pod_name":      created.Name,
		"pod_namespace": created.Namespace,
		"runtime_class": selectedRC,
	})
}

// CleanupCVMDemo 安全删除 CVM Demo Pod
func CleanupCVMDemo(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "K8s 集群不可用"})
		return
	}

	// 1. 先查找 Demo Pod 是否存在
	existingPod, err := h.K8s.GetPod(cvmDemoNamespace, cvmDemoName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "not_found",
			"message": "Demo Pod 不存在或已被删除",
		})
		return
	}

	// 2. 安全检查：确认 label 包含 managed-by=coco-panel
	labels := existingPod.Labels
	if labels == nil || labels[cvmManagedByKey] != cvmManagedByVal {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "安全检查失败",
			"message": fmt.Sprintf("Pod %s 不是由 CoCo Panel 管理的，拒绝删除", existingPod.Name),
		})
		return
	}

	// 3. 执行删除
	if err := h.K8s.DeletePod(cvmDemoNamespace, cvmDemoName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   fmt.Sprintf("删除 Demo Pod 失败: %v", err),
			"message": "请手动清理资源",
		})
		return
	}

	// 4. 通过 WebSocket 广播删除事件
	evt, _ := json.Marshal(map[string]interface{}{
		"type":      "pod_deleted",
		"name":      existingPod.Name,
		"namespace": existingPod.Namespace,
		"message":   "CVM Demo Pod 已清理",
	})
	WsHub.Broadcast(evt)

	c.JSON(http.StatusOK, gin.H{
		"status":  "deleted",
		"message": "Demo Pod 已成功清理",
	})
}

// GetCVMDemoLogs 获取 Demo Pod 容器日志
func GetCVMDemoLogs(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "K8s 集群不可用"})
		return
	}

	// 检查 Demo Pod 是否存在
	existingPod, err := h.K8s.GetPod(cvmDemoNamespace, cvmDemoName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "not_found",
			"message": "Demo Pod 不存在，请先启动",
		})
		return
	}

	podStatus := string(existingPod.Status.Phase)
	if podStatus != "Running" {
		c.JSON(http.StatusOK, gin.H{
			"status":  podStatus,
			"message": fmt.Sprintf("Pod 状态为 %s，日志暂不可用", podStatus),
			"logs":    "",
		})
		return
	}

	logs, err := h.K8s.GetPodLogs(cvmDemoNamespace, cvmDemoName, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": podStatus,
			"error":  fmt.Sprintf("获取日志失败: %v", err),
			"logs":   "",
		})
		return
	}

	trimmed := strings.TrimSpace(logs)
	if trimmed == "" {
		trimmed = "(容器日志为空)"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": podStatus,
		"logs":   trimmed,
	})
}
