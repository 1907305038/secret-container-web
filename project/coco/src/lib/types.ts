export interface OverviewData {
	tdx: { enabled: boolean; keyid_range: string; pamt_kb: number; module_initialized: boolean };
	sgx: { enabled: boolean; devices: string[] };
	node: { name: string; os: string; kernel: string; arch: string; cpu_cores: number; memory_gib: number };
	k8s: { version: string; runtime: string; pods_total: number; pods_running: number };
}

export interface PodInfo {
	name: string; namespace: string; status: string; ip: string;
	runtime_class: string; guest_kernel?: string;
	qemu_pid?: number; qemu_rss_mb?: number; started_at?: string;
}

export interface RuntimeInfo {
	name: string; handler: string; available: boolean;
}

export interface TrusteeComponent {
	status: string; endpoint: string;
}

export interface CompareData {
	comparisons: Record<string, { normal: string; confidential: string }>;
	running_pods: { normal: number; confidential: number };
}

// WebSocket 事件
export interface WsEvent {
	type: 'pod_created' | 'pod_deleted' | 'pod_phase' | 'pod_count' | 'tdx_status';
	name?: string;
	namespace?: string;
	runtime?: string;
	image?: string;
	phase?: string;
	count?: number;
	message?: string;
	data?: unknown;
}

// K8s Event（创建时间线）
export interface PodEvent {
	type: string;
	reason: string;
	message: string;
	timestamp: string;
}

// 内存加密验证
export interface MemoryRegion {
	name: string;
	address: string;
	hex_dump: string;
	ascii_safe: string;
	entropy: number;
	readable: boolean;
}

export interface MemoryRegionSet {
	found: boolean;
	regions: MemoryRegion[];
	entropy: number;
	note: string;
}

export interface MemoryEncryptProof {
	pod: string;
	qemu_pid: number;
	plaintext: string;
	auto_mode: boolean;
	host_view: MemoryRegionSet;
	guest_view: MemoryRegionSet;
}

// Pod 系统信息（来自 /api/pods/info）
export interface PodSysInfo {
	pod: string;
	info: Record<string, string>;
	is_tdx: boolean;
	host_procs: { pid: number; comm: string; rss_kb: number }[];
	guest_procs: { pid: number; comm: string }[];
}

// 写入+读取结果
export interface WriteAndReadResult {
	pod: string;
	plaintext: string;
	is_tdx: boolean;
	host_pid: number;
	process_name: string;
	memory_regions: MemoryRegion[];
	plaintext_found: boolean;
	note: string;
}
