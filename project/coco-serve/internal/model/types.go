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

// PodEvent K8s Event（用于创建时间线）
type PodEvent struct {
	Type      string `json:"type"`   // Normal / Warning
	Reason    string `json:"reason"` // Scheduled, Pulling, Started...
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// MemoryEncryptProof 内存加密验证结果
type MemoryEncryptProof struct {
	Pod       string          `json:"pod"`
	QemuPID   int             `json:"qemu_pid"`
	Plaintext string          `json:"plaintext"`
	AutoMode  bool            `json:"auto_mode"`
	HostView  MemoryRegionSet `json:"host_view"`
	GuestView MemoryRegionSet `json:"guest_view"`
}

// MemoryRegionSet 一侧（宿主机或容器内）的内存读取结果
type MemoryRegionSet struct {
	Found   bool           `json:"found"` // 是否找到明文字符串
	Regions []MemoryRegion `json:"regions"`
	Entropy float64        `json:"entropy"` // 平均熵值
	Note    string         `json:"note"`
}

// MemoryRegion 单个内存区域
type MemoryRegion struct {
	Name      string  `json:"name"`       // heap / anon / stack
	Address   string  `json:"address"`    // 起始-结束地址
	HexDump   string  `json:"hex_dump"`   // hex 字符串
	ASCIISafe string  `json:"ascii_safe"` // 可打印 ASCII
	Entropy   float64 `json:"entropy"`
	Readable  bool    `json:"readable"`
}

// WriteAndReadResult 写入数据后读取内存的结果
type WriteAndReadResult struct {
	Pod            string         `json:"pod"`
	Plaintext      string         `json:"plaintext"`
	IsTDX          bool           `json:"is_tdx"`
	HostPID        int            `json:"host_pid"`
	ProcessName    string         `json:"process_name"` // 宿主机上的进程名
	MemoryRegions  []MemoryRegion `json:"memory_regions"`
	PlaintextFound bool           `json:"plaintext_found"` // 宿主机内存中是否找到明文
	GuestConfirmed bool           `json:"guest_confirmed"` // 容器内确认数据存在
	AllWrites      int            `json:"all_writes"`      // 已写入的文件总数
	FileName       string         `json:"file_name"`       // 容器内文件名
	Note           string         `json:"note"`
}
