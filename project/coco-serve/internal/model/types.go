package model

// OverviewResponse 总览数据
type OverviewResponse struct {
	TDX  TDXStatus  `json:"tdx"`
	SGX  SGXStatus  `json:"sgx"`
	Node NodeInfo   `json:"node"`
	K8s  K8sSummary `json:"k8s"`
}

type TDXStatus struct {
	Enabled           bool   `json:"enabled"`
	KeyIDRange        string `json:"keyid_range"`
	PAMTKB            int    `json:"pamt_kb"`
	ModuleInitialized bool   `json:"module_initialized"`
}

type SGXStatus struct {
	Enabled bool     `json:"enabled"`
	Devices []string `json:"devices"`
}

type NodeInfo struct {
	Name      string `json:"name"`
	OS        string `json:"os"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
	CPUCores  int    `json:"cpu_cores"`
	MemoryGiB int    `json:"memory_gib"`
}

type K8sSummary struct {
	Version     string `json:"version"`
	Runtime     string `json:"runtime"`
	PodsTotal   int    `json:"pods_total"`
	PodsRunning int    `json:"pods_running"`
	// 按运行时分类统计
	ByRuntime map[string]int `json:"by_runtime"`
	// 按命名空间统计
	ByNamespace map[string]int `json:"by_namespace"`
	// 节点数
	NodeCount int `json:"node_count"`
}

// PodInfo Pod 信息
type PodInfo struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	IP           string `json:"ip,omitempty"`
	RuntimeClass string `json:"runtime_class,omitempty"`
	GuestKernel  string `json:"guest_kernel,omitempty"`
	QemuPID      int    `json:"qemu_pid,omitempty"`
	QemuRSSMB    int    `json:"qemu_rss_mb,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	// 隔离证明
	HostVisible      bool     `json:"host_visible"`
	HostView         string   `json:"host_view,omitempty"`
	HostProcesses    []string `json:"host_processes,omitempty"`
	MemoryAccessible string   `json:"memory_accessible,omitempty"`
}

type PodListResponse struct {
	Pods  []PodInfo `json:"pods"`
	Total int       `json:"total"`
}

// RuntimeInfo RuntimeClass 信息
type RuntimeInfo struct {
	Name        string `json:"name"`
	Handler     string `json:"handler"`
	Available   bool   `json:"available"`
	PodCount    int    `json:"pod_count"`
	Description string `json:"description"`
}

type RuntimeListResponse struct {
	Runtimes       []RuntimeInfo `json:"runtimes"`
	AvailableCount int           `json:"available_count"`
	Total          int           `json:"total"`
}

// TrusteeResponse Trustee 证明链状态
type TrusteeResponse struct {
	AS   TrusteeComponent `json:"as"`
	KBS  TrusteeComponent `json:"kbs"`
	RVPS TrusteeComponent `json:"rvps"`
}

type TrusteeComponent struct {
	Status      string   `json:"status"`
	Endpoint    string   `json:"endpoint"`
	Description string   `json:"description"`
	Details     []string `json:"details"`
}

type QemuProcess struct {
	PID   int `json:"pid"`
	RSSKB int `json:"rss_kb"`
}
