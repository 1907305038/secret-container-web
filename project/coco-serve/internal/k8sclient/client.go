package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var kubeconfig = "/etc/kubernetes/admin.conf"

// Client 封装 K8s API 调用
type Client struct {
	cs *kubernetes.Clientset
}

func New() (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return &Client{cs: cs}, nil
}

type NodeInfo struct {
	Name    string
	OS      string
	Version string
	Runtime string
}

func (c *Client) GetNodeInfo() (*NodeInfo, error) {
	nodes, err := c.cs.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	n := nodes.Items[0]
	return &NodeInfo{
		Name:    n.Name,
		OS:      n.Status.NodeInfo.OSImage,
		Version: n.Status.NodeInfo.KubeletVersion,
		Runtime: n.Status.NodeInfo.ContainerRuntimeVersion,
	}, nil
}

type PodSummary struct {
	Name         string
	Namespace    string
	Status       string
	IP           string
	RuntimeClass string
	StartedAt    string
}

func (c *Client) GetPods() ([]PodSummary, error) {
	pods, err := c.cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	var result []PodSummary
	for _, p := range pods.Items {
		if p.Status.Phase == "Failed" || p.Status.Phase == "Succeeded" {
			continue
		}
		rc := ""
		if p.Spec.RuntimeClassName != nil {
			rc = *p.Spec.RuntimeClassName
		}
		started := ""
		if p.Status.StartTime != nil {
			started = p.Status.StartTime.Format("2006-01-02T15:04:05-07:00")
		}
		result = append(result, PodSummary{
			Name:         p.Name,
			Namespace:    p.Namespace,
			Status:       string(p.Status.Phase),
			IP:           p.Status.PodIP,
			RuntimeClass: rc,
			StartedAt:    started,
		})
	}
	return result, nil
}

type RuntimeInfo struct {
	Name    string
	Handler string
}

func (c *Client) GetRuntimeClasses() ([]RuntimeInfo, error) {
	list, err := c.cs.NodeV1().RuntimeClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list runtimeclasses: %w", err)
	}
	var result []RuntimeInfo
	for _, r := range list.Items {
		result = append(result, RuntimeInfo{
			Name:    r.Name,
			Handler: r.Handler,
		})
	}
	return result, nil
}

// CreatePod 在 default 命名空间创建 Pod
func (c *Client) CreatePod(pod *corev1.Pod) (*corev1.Pod, error) {
	return c.cs.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
}

// DeletePod 删除指定命名空间下的 Pod
func (c *Client) DeletePod(namespace, name string) error {
	return c.cs.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// GetPodLogs 获取 Pod 日志
func (c *Client) GetPodLogs(namespace, name string, tailLines int64) (string, error) {
	req := c.cs.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{TailLines: &tailLines})
	stream, err := req.Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("stream logs: %w", err)
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return string(data), nil
}

// GetPodYAML 获取 Pod 的 YAML 配置
func (c *Client) GetPodYAML(namespace, name string) (string, error) {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}
	pod.ManagedFields = nil
	// 先用 JSON 序列化再转 YAML
	jsonBytes, err := json.Marshal(pod)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return "", fmt.Errorf("json to yaml: %w", err)
	}
	return string(yamlBytes), nil
}

// GetRuntimeDetail 获取 RuntimeClass 详细信息
func (c *Client) GetRuntimeDetail(name string) (map[string]string, error) {
	rc, err := c.cs.NodeV1().RuntimeClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get runtimeclass: %w", err)
	}
	info := map[string]string{
		"name":    rc.Name,
		"handler": rc.Handler,
	}
	if rc.Overhead != nil {
		if cpu, ok := rc.Overhead.PodFixed[corev1.ResourceCPU]; ok {
			info["cpu_overhead"] = cpu.String()
		}
		if mem, ok := rc.Overhead.PodFixed[corev1.ResourceMemory]; ok {
			info["mem_overhead"] = mem.String()
		}
	}
	if rc.Scheduling != nil && rc.Scheduling.NodeSelector != nil {
		for k, v := range rc.Scheduling.NodeSelector {
			info["node_selector_"+k] = v
		}
	}
	return info, nil
}

// EventItem K8s Event 摘要
type EventItem struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// GetPodEvents 获取 Pod 的 K8s Events（用于创建时间线）
func (c *Client) GetPodEvents(namespace, podName string) ([]EventItem, error) {
	events, err := c.cs.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + podName,
	})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	var result []EventItem
	for _, e := range events.Items {
		ts := ""
		if !e.LastTimestamp.IsZero() {
			ts = e.LastTimestamp.Format("15:04:05")
		} else if !e.FirstTimestamp.IsZero() {
			ts = e.FirstTimestamp.Format("15:04:05")
		}
		result = append(result, EventItem{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Timestamp: ts,
		})
	}
	// 按时间排序（K8s 返回可能是乱序的）
	for i := 0; i < len(result)/2; i++ {
		result[i], result[len(result)-1-i] = result[len(result)-1-i], result[i]
	}
	return result, nil
}
