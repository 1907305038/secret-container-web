<script lang="ts">
	import type { OverviewData } from '$lib/types';
	import { onMount } from 'svelte';
	import { fly, fade, slide } from 'svelte/transition';
	import { goto } from '$app/navigation';

	let d = $state<OverviewData | null>(null);
	let expanded = $state<Record<string,boolean>>({});
	let hoveredNs = $state('');

	function toggle(k: string) { expanded[k] = !expanded[k]; expanded = {...expanded}; }

	onMount(async () => {
		const res = await fetch('/api/overview');
		d = await res.json();
	});

	function runtimeLabel(rt: string) {
		if (rt.includes('tdx')) return '🔒 TDX';
		if (rt.includes('coco')) return '🔐 CoCo';
		if (rt.includes('qemu')) return '📦 Kata';
		return '⚪ 标准';
	}
	function runtimeColor(rt: string) {
		if (rt.includes('tdx')) return 'tdx';
		if (rt.includes('coco')) return 'coco';
		return 'default';
	}

	function goRuntime(rt: string) {
		if (rt === 'runc (默认)') rt = '';
		goto(`/pods?runtime=${encodeURIComponent(rt)}`);
	}
	function goNamespace(ns: string) {
		goto(`/pods?ns=${encodeURIComponent(ns)}`);
	}
</script>

{#if d}
<div class="page-header" in:fly={{ y: -8, duration: 300 }}>
	<h2>📊 系统总览</h2>
	<p class="sub">Confidential Containers · TDX 硬件加密 · Kata 安全运行时</p>
</div>

<!-- K8s 集群信息 -->
<div class="section" in:fly={{ y: 12, delay: 50, duration: 300 }}>
	<div class="section-header" on:click={() => toggle('cluster')} role="button" tabindex="0">
		<h3>☸️ Kubernetes 集群</h3>
		<span class="arrow {expanded['cluster']?'open':''}">▸</span>
	</div>
	<div class="k8s-stats">
		<div class="stat-card clickable" on:click={() => toggle('cluster')}>
			<div class="stat-val">{d.k8s.version}</div>
			<div class="stat-label">版本</div>
		</div>
		<div class="stat-card clickable" on:click={() => toggle('cluster')}>
			<div class="stat-val">{d.k8s.node_count}</div>
			<div class="stat-label">节点 <span class="hint">▸</span></div>
		</div>
		<div class="stat-card green clickable" on:click|stopPropagation={() => goto('/pods')}>
			<div class="stat-val">{d.k8s.pods_running}</div>
			<div class="stat-label">运行中 <span class="hint">▸</span></div>
		</div>
		<div class="stat-card dim clickable" on:click|stopPropagation={() => goto('/pods')}>
			<div class="stat-val">{d.k8s.pods_total}</div>
			<div class="stat-label">总 Pod <span class="hint">▸</span></div>
		</div>
	</div>
	{#if expanded['cluster']}
		<div class="detail-panel" in:slide={{ duration: 200 }}>
			<div class="detail-row"><span>运行时</span><code>{d.k8s.runtime}</code></div>
			<div class="detail-row"><span>节点名</span><code>{d.node.name}</code></div>
			<div class="detail-row"><span>OS</span><span>{d.node.os}</span></div>
			<div class="detail-row"><span>内核</span><code>{d.node.kernel}</code></div>
			<div class="detail-row"><span>架构</span><code>{d.node.arch}</code></div>
			<div class="detail-row"><span>CPU / 内存</span><span>{d.node.cpu_cores} 核 / {d.node.memory_gib} GB</span></div>
		</div>
	{/if}
</div>

<!-- TEE 硬件 -->
<div class="section" in:fly={{ y: 12, delay: 80, duration: 300 }}>
	<div class="section-header" on:click={() => toggle('hw')} role="button" tabindex="0">
		<h3>🛡️ TEE 硬件</h3>
		<span class="arrow {expanded['hw']?'open':''}">▸</span>
	</div>
	<div class="hw-row">
		<div class="hw-card tdx clickable" on:click|stopPropagation={() => goto('/trustee')} title="查看证明链">
			<div class="hw-icon">🔒</div>
			<div class="hw-info">
				<div class="hw-name">Intel TDX <span class="hw-link">查看证明 →</span></div>
				<div class="hw-status on">✅ 已启用</div>
				<div class="hw-detail">KeyID: {d.tdx.keyid_range}</div>
				<div class="hw-detail">PAMT: {(d.tdx.pamt_kb / 1024 / 1024).toFixed(1)} GB</div>
			</div>
		</div>
		<div class="hw-card cca {d.cca.enabled ? 'clickable' : ''}" title="ARM CCA 需要 ARM 平台 (如 Ampere Altra)">
			<div class="hw-icon">🧬</div>
			<div class="hw-info">
				<div class="hw-name">ARM CCA <span class="hw-link">{d.cca.enabled ? '查看证明 →' : '（需要 ARM 平台）'}</span></div>
				<div class="hw-status {d.cca.enabled ? 'on' : 'off'}">{d.cca.enabled ? '✅ Realm 可用' : '❌ ' + d.cca.arch}</div>
				{#if d.cca.enabled}
					<div class="hw-detail">RMM: {d.cca.rmm_available ? '✅' : '❌'} | Realm: {d.cca.realm_supported ? '✅' : '❌'}</div>
					<div class="hw-detail">Granule: {d.cca.granule_size}</div>
				{/if}
			</div>
		</div>
	</div>
</div>

<!-- Pod 部署分类 -->
<div class="section" in:fly={{ y: 12, delay: 110, duration: 300 }}>
	<div class="section-header">
		<h3>📦 Pod 部署分类</h3>
		<span class="hint-link" on:click|stopPropagation={() => goto('/runtimes')}>查看运行时 →</span>
	</div>
	<div class="runtime-grid">
		{#each Object.entries(d.k8s.by_runtime) as [rt, cnt]}
			<div class="runtime-card {runtimeColor(rt)} clickable" in:fly={{ y: 8, delay: 150 }}
				on:click={() => goRuntime(rt)} title={`查看 ${runtimeLabel(rt)} Pod 列表`}>
				<div class="rt-badge {runtimeColor(rt)}">{runtimeLabel(rt)}</div>
				<div class="rt-name">{rt}</div>
				<div class="rt-count">{cnt} 个 Pod <span class="rt-arrow">→</span></div>
				<div class="rt-bar">
					<div class="rt-fill {runtimeColor(rt)}" style="width: {cnt/d.k8s.pods_total*100}%"></div>
				</div>
			</div>
		{/each}
	</div>
</div>

<!-- 命名空间 -->
<div class="section" in:fly={{ y: 12, delay: 140, duration: 300 }}>
	<div class="section-header" on:click={() => toggle('ns')} role="button" tabindex="0">
		<h3>📂 命名空间分布</h3>
		<span class="arrow {expanded['ns']?'open':''}">▸</span>
	</div>
	<div class="ns-grid">
		{#each Object.entries(d.k8s.by_namespace) as [ns, cnt]}
			<button class="ns-chip" class:hovered={hoveredNs === ns}
				on:click|stopPropagation={() => goNamespace(ns)}
				on:mouseenter={() => hoveredNs = ns}
				on:mouseleave={() => hoveredNs = ''}
				title="查看 {ns} 命名空间 Pod">
				{ns}<b>{cnt}</b><span class="ns-arrow">↗</span>
			</button>
		{/each}
	</div>
	{#if expanded['ns']}
		<div class="detail-panel" in:slide={{ duration: 200 }}>
			{#each Object.entries(d.k8s.by_namespace) as [ns, cnt]}
				<button class="detail-row clickable" on:click|stopPropagation={() => goNamespace(ns)}>
					<span>{ns}</span><code>{cnt} Pods →</code>
				</button>
			{/each}
		</div>
	{/if}
</div>
{:else}
	<div class="loading">加载中...</div>
{/if}

<style>
	h2 { margin: 0; font-size: 1.4rem; }
	.page-header { margin-bottom: 1.2rem; }
	.sub { color: #64748b; font-size: 0.85rem; margin: 0.2rem 0 0; }
	.loading { text-align: center; padding: 3rem; color: #94a3b8; animation: pulse 1.5s infinite; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }

	.section { background: #fff; border: 1px solid #e8ecf1; border-radius: 12px; padding: 1rem 1.2rem; margin-bottom: 0.8rem; transition: box-shadow 0.2s; }
	.section:hover { box-shadow: 0 2px 12px rgba(0,0,0,0.04); }
	.section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.6rem; cursor: pointer; }
	.section-header h3 { margin: 0; font-size: 0.95rem; color: #1e293b; }
	.arrow { color: #94a3b8; font-size: 0.85rem; transition: transform 0.2s; }
	.arrow.open { transform: rotate(90deg); }
	.hint-link { font-size: 0.73rem; color: #3b82f6; cursor: pointer; }
	.hint-link:hover { text-decoration: underline; }

	/* K8s stats */
	.k8s-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; }
	@media (max-width: 500px) { .k8s-stats { grid-template-columns: repeat(2, 1fr); } }
	.stat-card { text-align: center; padding: 10px; border-radius: 10px; background: #f8fafc; border: 1px solid #e2e8f0; transition: all 0.2s; }
	.stat-card.clickable { cursor: pointer; }
	.stat-card.clickable:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,0.06); border-color: #cbd5e1; }
	.stat-card.green { background: #f0fdf4; border-color: #bbf7d0; }
	.stat-card.green.clickable:hover { background: #dcfce7; border-color: #86efac; }
	.stat-card.dim { background: #f1f5f9; }
	.stat-card.dim.clickable:hover { background: #e2e8f0; }
	.stat-val { font-size: 1.5rem; font-weight: 700; color: #1e293b; }
	.stat-card.green .stat-val { color: #16a34a; }
	.stat-label { font-size: 0.72rem; color: #94a3b8; margin-top: 2px; }
	.hint { opacity: 0; transition: opacity 0.2s; }
	.stat-card:hover .hint { opacity: 1; }

	.detail-panel { margin-top: 0.6rem; padding: 0.8rem; background: #f8fafc; border-radius: 8px; border: 1px solid #e2e8f0; }
	.detail-row { display: flex; justify-content: space-between; padding: 5px 0; font-size: 0.82rem; color: #475569; border-bottom: 1px solid #f1f5f9; width: 100%; background: none; border-left: none; border-right: none; border-top: none; border-radius: 0; cursor: default; }
	.detail-row.clickable { cursor: pointer; transition: background 0.15s; }
	.detail-row.clickable:hover { background: #fff; }
	.detail-row:last-child { border-bottom: none; }
	.detail-row code { font-size: 0.78rem; }

	/* HW cards */
	.hw-row { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
	@media (max-width: 500px) { .hw-row { grid-template-columns: 1fr; } }
	.hw-card { display: flex; gap: 12px; padding: 12px; border-radius: 10px; align-items: center; transition: all 0.25s; }
	.hw-card.tdx { background: linear-gradient(135deg, #f0fdf4, #dcfce7); border: 1px solid #bbf7d0; }
	.hw-card.cca { background: linear-gradient(135deg, #fdf4ff, #fae8ff); border: 1px solid #f0abfc; }
	.hw-card.clickable { cursor: pointer; }
	.hw-card.clickable:hover { transform: translateY(-2px); box-shadow: 0 6px 20px rgba(0,0,0,0.06); }
	.hw-card.tdx.clickable:hover { box-shadow: 0 6px 20px rgba(76,175,80,0.12); }
	.hw-card.cca.clickable:hover { box-shadow: 0 6px 20px rgba(168,85,247,0.12); }
	.hw-icon { font-size: 1.8rem; transition: transform 0.2s; }
	.hw-card:hover .hw-icon { transform: scale(1.15); }
	.hw-name { font-weight: 700; font-size: 0.9rem; color: #1e293b; }
	.hw-link { font-size: 0.7rem; color: #3b82f6; opacity: 0; transition: opacity 0.2s; font-weight: 400; }
	.hw-card:hover .hw-link { opacity: 1; }
	.hw-status.on { color: #16a34a; font-size: 0.8rem; font-weight: 600; }
	.hw-detail { font-size: 0.75rem; color: #64748b; margin-top: 2px; }

	/* Runtime grid */
	.runtime-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 8px; }
	.runtime-card { padding: 10px; border-radius: 10px; border: 1px solid #e2e8f0; transition: all 0.2s; }
	.runtime-card.clickable { cursor: pointer; }
	.runtime-card.clickable:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,0.06); }
	.runtime-card.tdx { border-left: 3px solid #4caf50; background: #f6fdf6; }
	.runtime-card.tdx.clickable:hover { box-shadow: 0 4px 14px rgba(76,175,80,0.12); border-color: #86efac; }
	.runtime-card.coco { border-left: 3px solid #7c3aed; background: #faf5ff; }
	.runtime-card.coco.clickable:hover { box-shadow: 0 4px 14px rgba(124,58,237,0.12); }
	.rt-badge { display: inline-block; font-size: 0.68rem; padding: 1px 7px; border-radius: 4px; font-weight: 600; margin-bottom: 4px; }
	.rt-badge.tdx { background: #e8f5e9; color: #2e7d32; }
	.rt-badge.coco { background: #ede9fe; color: #5b21b6; }
	.rt-badge.default { background: #f1f5f9; color: #64748b; }
	.rt-name { font-size: 0.72rem; color: #64748b; font-family: monospace; }
	.rt-count { font-size: 1.1rem; font-weight: 700; color: #1e293b; margin: 2px 0 6px; }
	.rt-arrow { font-size: 0.75rem; color: #94a3b8; opacity: 0; transition: all 0.2s; }
	.runtime-card:hover .rt-arrow { opacity: 1; }
	.rt-bar { height: 4px; background: #e2e8f0; border-radius: 2px; overflow: hidden; }
	.rt-fill { height: 100%; border-radius: 2px; transition: width 0.5s ease; }
	.rt-fill.tdx { background: #4caf50; }
	.rt-fill.coco { background: #7c3aed; }
	.rt-fill.default { background: #94a3b8; }

	/* NS chips */
	.ns-grid { display: flex; flex-wrap: wrap; gap: 6px; }
	.ns-chip { padding: 5px 10px; border-radius: 6px; background: #f1f5f9; font-size: 0.78rem; color: #475569; border: 1px solid #e2e8f0; cursor: pointer; transition: all 0.2s; }
	.ns-chip:hover, .ns-chip.hovered { background: #e2e8f0; border-color: #cbd5e1; transform: translateY(-1px); box-shadow: 0 2px 6px rgba(0,0,0,0.04); }
	.ns-chip b { margin-left: 4px; color: #1e293b; }
	.ns-arrow { font-size: 0.65rem; color: #94a3b8; opacity: 0; margin-left: 3px; transition: opacity 0.2s; }
	.ns-chip:hover .ns-arrow { opacity: 1; }
</style>