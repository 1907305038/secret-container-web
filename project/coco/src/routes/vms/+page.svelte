<script lang="ts">
	import { fly, fade } from 'svelte/transition';

	let vms = $state<any[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let msg = $state('');

	async function load() {
		loading = true;
		try {
			const r = await fetch('/api/vms');
			const d = await r.json();
			vms = d.vms || [];
			total = d.total || 0;
		} catch { msg = '加载失败'; }
		loading = false;
	}

	function vmTypeLabel(t: string) {
		if (t === 'tdx') return '🟢 TDX';
		if (t === 'cca') return '🧬 CCA';
		return '⚪ 普通';
	}

	function sizeFmt(mb: number) { return mb > 1024 ? (mb/1024).toFixed(1)+'GB' : mb+'MB'; }

	function runFmt(s: number) {
		if (s < 60) return s + '秒';
		if (s < 3600) return (s/60).toFixed(0) + '分钟';
		if (s < 86400) return (s/3600).toFixed(1) + '小时';
		return (s/86400).toFixed(1) + '天';
	}

	$effect(() => { load(); });
</script>

<div class="page-header">
	<h2>🖥️ 机密虚拟机</h2>
	<div class="stats-row">
		<div class="stat-badge">TDX VM: <b>{vms.filter((v:any) => v.vm_type === 'tdx').length}</b></div>
		<div class="stat-badge dim">总计: <b>{total}</b></div>
	</div>
</div>

<div class="toolbar">
	<button onclick={load} class="btn-refresh" title="刷新">🔄 刷新</button>
</div>

{#if msg}
	<div class="toast" in:fly={{ y: -8, duration: 200 }} out:fade>{msg}</div>
{/if}

{#if loading}
	<div class="loading">加载中...</div>
{:else if vms.length === 0}
	<div class="empty" in:fade>
		<span>📭</span>
		<p>暂无独立机密虚拟机</p>
		<p class="sub">当前所有 QEMU 进程均关联 K8s Pod，请查看「机密容器」页面</p>
	</div>
{:else}
	<div class="list">
		{#each vms as vm, i (vm.pid)}
			<div class="vm-card" in:fly={{ y: 10, delay: i * 40, duration: 250 }}>
				<div class="vm-left">
					<span class="vm-type {vm.vm_type}">{vmTypeLabel(vm.vm_type)}</span>
				</div>
				<div class="vm-main">
					<div class="vm-top">
						<span class="vm-name">{vm.name || '未命名'}</span>
						<span class="vm-pid">PID {vm.pid}</span>
					</div>
					<div class="vm-sub">
						<span class="chip">内存: {sizeFmt(vm.memory_mb || 0)}</span>
						<span class="chip">RSS: {sizeFmt(vm.rss_mb || 0)}</span>
						<span class="chip">运行: {runFmt(vm.running_sec)}</span>
						{#if vm.pod_name}
							<span class="chip pod-chip">📦 {vm.pod_ns}/{vm.pod_name}</span>
						{:else}
							<span class="chip standalone-chip">🔧 独立运行</span>
						{/if}
						<span class="chip visibility">{vm.host_visible ? '👁️ 宿主机可见' : '🔒 宿主机不可见'}</span>
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	h2 { margin: 0; font-size: 1.4rem; }
	.page-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem; }
	.stats-row { display: flex; gap: 8px; }
	.stat-badge { background: #e8f5e9; color: #2e7d32; padding: 3px 12px; border-radius: 20px; font-size: 0.8rem; font-weight: 500; }
	.stat-badge.dim { background: #f0f0f0; color: #666; }
	.stat-badge b { font-weight: 700; }

	.toolbar { margin-bottom: 1rem; }
	.btn-refresh { padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer; font-size: 0.85rem; background: #e2e8f0; color: #475569; }
	.btn-refresh:hover { background: #cbd5e1; }

	.loading, .empty { text-align: center; padding: 3rem; color: #94a3b8; }
	.empty span { font-size: 3rem; display: block; margin-bottom: 0.5rem; }
	.empty .sub { font-size: 0.8rem; color: #cbd5e1; margin-top: 0.3rem; }

	.toast { background: #fef3c7; color: #92400e; padding: 8px 16px; border-radius: 8px; margin-bottom: 0.8rem; font-size: 0.85rem; }

	.list { display: flex; flex-direction: column; gap: 8px; }

	.vm-card {
		display: flex; align-items: center; gap: 12px;
		background: #fff; padding: 12px 16px; border-radius: 10px;
		box-shadow: 0 1px 3px rgba(0,0,0,0.06);
	}
	.vm-left { flex-shrink: 0; }
	.vm-type { font-size: 0.75rem; padding: 3px 10px; border-radius: 12px; font-weight: 600; }
	.vm-type.tdx { background: #dcfce7; color: #166534; }
	.vm-type.cca { background: #fae8ff; color: #7c3aed; }
	.vm-type.normal { background: #f1f5f9; color: #64748b; }

	.vm-main { flex: 1; }
	.vm-top { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
	.vm-name { font-weight: 600; font-size: 0.9rem; }
	.vm-pid { font-size: 0.75rem; color: #94a3b8; font-family: monospace; }

	.vm-sub { display: flex; gap: 6px; flex-wrap: wrap; }
	.chip { font-size: 0.7rem; padding: 2px 8px; background: #f1f5f9; border-radius: 6px; color: #64748b; }
	.chip.pod-chip { background: #dbeafe; color: #1e40af; }
	.chip.standalone-chip { background: #fef3c7; color: #92400e; }
	.chip.visibility { background: #f0fdf4; color: #166534; }
</style>
