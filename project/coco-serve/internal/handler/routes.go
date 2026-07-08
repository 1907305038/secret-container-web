package handler

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"coco-serve/internal/collector"
	"coco-serve/internal/model"

	"github.com/gin-gonic/gin"
)

func GetOverview(c *gin.Context) {
	tdx := collector.GetTDXStatus()
	cca := collector.GetCCAStatus()

	node := model.NodeInfo{
		Name:      hostname(),
		OS:        osInfo(),
		Kernel:    kernel(),
		Arch:      runtime.GOARCH,
		CPUCores:  runtime.NumCPU(),
		MemoryGiB: memTotalGiB(),
	}

	k8s := model.K8sSummary{}
	if h != nil && h.K8s != nil {
		if ni, err := h.K8s.GetNodeInfo(); err == nil {
			node.Name = ni.Name
			node.OS = ni.OS
			k8s.Version = strings.TrimPrefix(ni.Version, "v")
			k8s.Runtime = ni.Runtime
		}
		if pods, err := h.K8s.GetPods(); err == nil {
			k8s.PodsTotal = len(pods)
			k8s.ByRuntime = map[string]int{}
			k8s.ByNamespace = map[string]int{}
			for _, p := range pods {
				if p.Status == "Running" {
					k8s.PodsRunning++
				}
				rt := p.RuntimeClass
				if rt == "" {
					rt = "runc (默认)"
				}
				k8s.ByRuntime[rt]++
				k8s.ByNamespace[p.Namespace]++
			}
		}
		k8s.NodeCount = 1
	}

	c.JSON(http.StatusOK, model.OverviewResponse{
		TDX:  tdx,
		CCA:  cca,
		Node: node,
		K8s:  k8s,
	})
}

func GetPods(c *gin.Context) {
	filter := c.DefaultQuery("runtime", "")
	confOnly := c.Query("confidential") == "true"
	nsFilter := c.Query("ns")

	pods := []model.PodInfo{}
	if h != nil && h.K8s != nil {
		if kpods, err := h.K8s.GetPods(); err == nil {
			for _, p := range kpods {
				if filter != "" && p.RuntimeClass != filter {
					continue
				}
				if nsFilter != "" && p.Namespace != nsFilter {
					continue
				}
				if confOnly && !strings.Contains(p.RuntimeClass, "tdx") {
					continue
				}
				pi := model.PodInfo{
					Name:         p.Name,
					Namespace:    p.Namespace,
					Status:       p.Status,
					IP:           p.IP,
					RuntimeClass: p.RuntimeClass,
					StartedAt:    p.StartedAt,
				}
				if strings.Contains(p.RuntimeClass, "tdx") {
					pi.GuestKernel = collector.GetGuestKernel(p.Name, p.Namespace)
				}
				// 隔离证明
				fillProof(&pi, p.RuntimeClass, p.Namespace, p.Name)
				pods = append(pods, pi)
			}
		}
	}

	c.JSON(http.StatusOK, model.PodListResponse{
		Pods:  pods,
		Total: len(pods),
	})
}

func fillProof(pi *model.PodInfo, runtime, ns, name string) {
	isTdx := strings.Contains(runtime, "tdx")
	if isTdx {
		pi.HostVisible = false
		pi.HostView = "宿主机 root 看不到容器内进程 — 只能看到加密边界 QEMU"
		pi.MemoryAccessible = "不可读 — QEMU 内存全程 TDX 硬件加密"
		// 找 QEMU 进程
		if out, err := execCmd("sh", "-c", "ps -eo pid,rss,args | grep qemu-system | grep -v grep | head -2"); err == nil {
			pi.HostProcesses = splitLines(out)
		}
	} else {
		pi.HostVisible = true
		pi.HostView = "宿主机 root 可以看到容器对应的 containerd-shim 进程"
		pi.MemoryAccessible = "可读 — root 可通过 /proc/<pid>/mem dump 进程内存"
		pi.HostProcesses = []string{"可通过 containerd-shim 追踪到容器内进程"}
	}
}

var execCmd = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

func splitLines(s string) []string {
	var r []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			r = append(r, l)
		}
	}
	return r
}

func GetRuntimes(c *gin.Context) {
	runtimes := []model.RuntimeInfo{}
	available := 0
	podByRuntime := map[string]int{}

	if h != nil && h.K8s != nil {
		if pods, err := h.K8s.GetPods(); err == nil {
			for _, p := range pods {
				podByRuntime[p.RuntimeClass]++
			}
		}
		if list, err := h.K8s.GetRuntimeClasses(); err == nil {
			for _, r := range list {
				avail := r.Name == "kata-qemu-tdx" || r.Name == "kata-qemu-coco-dev" ||
					r.Name == "kata-qemu-coco-dev-runtime-rs" || r.Name == "kata-qemu"
				if avail {
					available++
				}
				rt := model.RuntimeInfo{
					Name:      r.Name,
					Handler:   r.Handler,
					Available: avail,
					PodCount:  podByRuntime[r.Name],
				}
				// 注入描述
				switch {
				case strings.Contains(r.Name, "tdx"):
					rt.Description = "Intel TDX 硬件加密，Guest 内核隔离，内存不可被 root 读取"
				case strings.Contains(r.Name, "coco-dev"):
					rt.Description = "Confidential Containers 开发模式，支持远程证明"
				case strings.Contains(r.Name, "qemu"):
					rt.Description = "Kata Containers QEMU 虚拟化，无硬件加密"
				default:
					rt.Description = "标准 runc 容器运行时"
				}
				runtimes = append(runtimes, rt)
			}
		}
	}

	c.JSON(http.StatusOK, model.RuntimeListResponse{
		Runtimes:       runtimes,
		AvailableCount: available,
		Total:          len(runtimes),
	})
}

func GetTrustee(c *gin.Context) {
	c.JSON(http.StatusOK, collector.GetTrusteeEndpoints())
}

// GetPodYaml 返回 Pod 的 YAML 配置
func GetPodYaml(c *gin.Context) {
	ns := c.Param("namespace")
	name := c.Param("name")
	if h == nil || h.K8s == nil {
		c.JSON(503, gin.H{"error": "K8s not available"})
		return
	}
	yamlStr, err := h.K8s.GetPodYAML(ns, name)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"yaml": yamlStr})
}

// GetRuntimeDetail 返回 RuntimeClass 详细信息
func GetRuntimeDetail(c *gin.Context) {
	name := c.Param("name")
	if h == nil || h.K8s == nil {
		c.JSON(503, gin.H{"error": "K8s not available"})
		return
	}
	detail, err := h.K8s.GetRuntimeDetail(name)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, detail)
}

// GetPodEvents 返回 Pod 的 K8s Events（创建时间线）
func GetPodEvents(c *gin.Context) {
	ns := c.Param("namespace")
	name := c.Param("name")
	if h == nil || h.K8s == nil {
		c.JSON(503, gin.H{"error": "K8s not available"})
		return
	}
	events, err := h.K8s.GetPodEvents(ns, name)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"events": events})
}

// helpers
func hostname() string {
	n, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return n
}
func kernel() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	// 截取内核版本号
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return fields[2]
	}
	return "unknown"
}
func osInfo() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(line[13:], "\"")
		}
	}
	return "Fedora Linux 44"
}
func memTotalGiB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.Atoi(fields[1])
				return kb / 1024 / 1024
			}
		}
	}
	return 0
}
