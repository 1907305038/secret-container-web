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
