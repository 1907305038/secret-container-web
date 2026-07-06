package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gin-gonic/gin"
)

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PortMapping struct {
	Name          string `json:"name"`
	ContainerPort int32  `json:"container_port"`
	HostPort      int32  `json:"host_port,omitempty"`
	Protocol      string `json:"protocol"`
}

type ResourceSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type CreatePodRequest struct {
	Name      string            `json:"name" binding:"required"`
	Namespace string            `json:"namespace"`
	Image     string            `json:"image" binding:"required"`
	Runtime   string            `json:"runtime"`
	Command   string            `json:"command"`
	Args      string            `json:"args"`
	Env       []EnvVar          `json:"env"`
	Ports     []PortMapping     `json:"ports"`
	Requests  *ResourceSpec     `json:"requests"`
	Limits    *ResourceSpec     `json:"limits"`
	Labels    map[string]string `json:"labels"`
	NodeSel   map[string]string `json:"node_selector"`
}

func CreatePod(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "K8s not available"})
		return
	}
	var req CreatePodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ns := req.Namespace
	if ns == "" {
		ns = "default"
	}

	// 命令
	cmd := []string{"sleep", "infinity"}
	if req.Command != "" {
		cmd = []string{"sh", "-c", req.Command}
	}
	var cmdArgs []string
	if req.Args != "" {
		cmdArgs = strings.Fields(req.Args)
	}

	// 环境变量
	var envVars []corev1.EnvVar
	for _, e := range req.Env {
		envVars = append(envVars, corev1.EnvVar{Name: e.Name, Value: e.Value})
	}

	// 端口
	var ports []corev1.ContainerPort
	for _, p := range req.Ports {
		proto := corev1.ProtocolTCP
		if p.Protocol == "UDP" {
			proto = corev1.ProtocolUDP
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			Protocol:      proto,
		})
	}

	// 资源
	var resources corev1.ResourceRequirements
	if req.Requests != nil && (req.Requests.CPU != "" || req.Requests.Memory != "") {
		resources.Requests = corev1.ResourceList{}
		if req.Requests.CPU != "" {
			resources.Requests[corev1.ResourceCPU] = resource.MustParse(req.Requests.CPU)
		}
		if req.Requests.Memory != "" {
			resources.Requests[corev1.ResourceMemory] = resource.MustParse(req.Requests.Memory)
		}
	}
	if req.Limits != nil && (req.Limits.CPU != "" || req.Limits.Memory != "") {
		resources.Limits = corev1.ResourceList{}
		if req.Limits.CPU != "" {
			resources.Limits[corev1.ResourceCPU] = resource.MustParse(req.Limits.CPU)
		}
		if req.Limits.Memory != "" {
			resources.Limits[corev1.ResourceMemory] = resource.MustParse(req.Limits.Memory)
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   req.Name,
			Labels: req.Labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:      "app",
				Image:     req.Image,
				Command:   cmd,
				Args:      cmdArgs,
				Env:       envVars,
				Ports:     ports,
				Resources: resources,
			}},
			NodeSelector: req.NodeSel,
		},
	}
	if req.Runtime != "" {
		pod.Spec.RuntimeClassName = &req.Runtime
	}

	if req.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels["app"] = req.Name

	created, err := h.K8s.CreatePod(pod)
	if err != nil {
		log.Printf("create pod: %v", err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("create pod: %v", err)})
		return
	}
	c.JSON(201, gin.H{"status": "created", "name": created.Name, "ip": created.Status.PodIP, "namespace": ns})
}

func DeletePod(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(503, gin.H{"error": "K8s not available"})
		return
	}
	ns, name := c.Param("namespace"), c.Param("name")
	if err := h.K8s.DeletePod(ns, name); err != nil {
		log.Printf("delete pod: %v", err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("delete: %v", err)})
		return
	}
	c.JSON(200, gin.H{"status": "deleted", "name": name})
}

func GetPodLogs(c *gin.Context) {
	if h == nil || h.K8s == nil {
		c.JSON(503, gin.H{"error": "K8s not available"})
		return
	}
	ns, name := c.Param("namespace"), c.Param("name")
	tailStr := c.DefaultQuery("tail", "100")
	tail, err := strconv.ParseInt(tailStr, 10, 64)
	if err != nil {
		tail = 100
	}
	logs, err := h.K8s.GetPodLogs(ns, name, tail)
	if err != nil {
		log.Printf("get logs: %v", err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("logs: %v", err)})
		return
	}
	c.String(200, logs)
}
