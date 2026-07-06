<script lang="ts">
	import { onMount } from 'svelte';
	import { fly, fade, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	let runtimes = $state<any[]>([]);
	let avail = $state(0);
	let total = $state(0);
	let expanded = $state<Record<string,boolean>>({});
	let detailData = $state<Record<string,any>>({});

	onMount(async () => {
		const res = await fetch('/api/runtimes');
		const d = await res.json();
		runtimes = d.runtimes;
		avail = d.available_count;
		total = d.total;
	});

	async function toggleDetail(name: string) {
		if (expanded[name]) { expanded[name] = false; expanded = {...expanded}; return; }
		expanded[name] = true; expanded = {...expanded};
		const r = await fetch(`/api/runtimes/${name}`);
		detailData[name] = await r.json();
		detailData = {...detailData};
	}

	function isTDX(name: string) { return name?.includes('tdx'); }
	function isCoco(name: string) { return name?.includes('coco'); }
</script>

<div class="page-header">
	<h2>🔄 运行时类</h2>
	<div class="stats-row">
		<div class="stat-badge green">可用: <b>{avail}</b></div>
		<div class="stat-badge dim">总计: <b>{total}</b></div>
	</div>
</div>

<div class="list">
	{#each runtimes as r, i (r.name)}
		<div class="card-wrapper" in:fly={{ y: 10, delay: i * 40, duration: 250 }}>
			<div class="card {isTDX(r.name)?'tdx':''} {isCoco(r.name)?'coco':''}"
				on:click={() => toggleDetail(r.name)} role="button" tabindex="0">
				<div class="card-left">
					<span class="status-dot {r.available?'on':'off'}"></span>
					<span class="badge {isTDX(r.name)?'tdx':isCoco(r.name)?'coco':'default'}">
						{isTDX(r.name)?'🔒 TDX':isCoco(r.name)?'🔐 CoCo':'⚪ 标准'}
					</span>
				</div>
				<div class="card-main">
					<div class="name-row">
						<span class="name">{r.name}</span>
						<span class="pod-count">{r.pod_count} 个 Pod</span>
					</div>
					<div class="desc">{r.description}</div>
				</div>
				<div class="card-right">
					<span class="arrow {expanded[r.name]?'open':''}">▸</span>
				</div>
			</div>

			{#if expanded[r.name] && detailData[r.name]}
				<div class="detail" in:slide={{ duration: 200 }}>
					<div class="detail-grid">
						<div class="detail-item" in:fade={{ delay: 50 }}>
							<div class="dl">Handler</div>
							<code>{detailData[r.name].handler}</code>
						</div>
						{#if detailData[r.name].cpu_overhead}
						<div class="detail-item" in:fade={{ delay: 80 }}>
							<div class="dl">CPU 开销</div>
							<code>{detailData[r.name].cpu_overhead}</code>
						</div>
						{/if}
						{#if detailData[r.name].mem_overhead}
						<div class="detail-item" in:fade={{ delay: 100 }}>
							<div class="dl">内存开销</div>
							<code>{detailData[r.name].mem_overhead}</code>
						</div>
						{/if}
						{#if detailData[r.name].node_selector}
						{#each Object.entries(detailData[r.name]) as [k, v]}
							{#if k.startsWith('node_selector_')}
								<div class="detail-item" in:fade={{ delay: 120 }}>
									<div class="dl">节点选择 {k.replace('node_selector_','')}</div>
									<code>{v}</code>
								</div>
							{/if}
						{/each}
						{/if}
						<div class="detail-item" in:fade={{ delay: 140 }}>
							<div class="dl">状态</div>
							<code>{r.available ? '✅ 可用（硬件支持）' : '⚠️ 已注册（可能缺少硬件支持）'}</code>
						</div>
					</div>
				</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	h2 { margin: 0; font-size: 1.4rem; }
	.page-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem; }
	.stats-row { display: flex; gap: 8px; }
	.stat-badge { padding: 3px 12px; border-radius: 20px; font-size: 0.8rem; font-weight: 500; }
	.stat-badge.green { background: #e8f5e9; color: #2e7d32; }
	.stat-badge.dim { background: #f0f0f0; color: #666; }
	.stat-badge b { font-weight: 700; }

	.list { display: flex; flex-direction: column; gap: 4px; }
	.card-wrapper { }
	.card {
		display: flex; align-items: center; gap: 12px;
		background: #fff; border: 1px solid #e8ecf1; border-radius: 10px;
		padding: 12px 14px; cursor: pointer; transition: all 0.2s;
	}
	.card:hover { border-color: #cbd5e1; box-shadow: 0 2px 8px rgba(0,0,0,0.04); transform: translateY(-1px); }
	.card.tdx { border-left: 4px solid #4caf50; background: linear-gradient(90deg, #f6fdf6 0%, #fff 100%); }
	.card.coco { border-left: 4px solid #7c3aed; background: linear-gradient(90deg, #faf5ff 0%, #fff 100%); }
	.card-left { display: flex; align-items: center; gap: 10px; min-width: 120px; }
	.status-dot { width: 8px; height: 8px; border-radius: 50%; }
	.status-dot.on { background: #4caf50; animation: pulse 2s infinite; }
	.status-dot.off { background: #cbd5e1; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
	.badge { font-size: 0.75rem; font-weight: 600; padding: 2px 8px; border-radius: 5px; }
	.badge.tdx { background: #e8f5e9; color: #2e7d32; }
	.badge.coco { background: #ede9fe; color: #5b21b6; }
	.badge.default { background: #f1f5f9; color: #64748b; }
	.card-main { flex: 1; min-width: 0; }
	.name-row { display: flex; align-items: center; gap: 10px; }
	.name { font-weight: 600; color: #1e293b; font-size: 0.9rem; font-family: monospace; }
	.pod-count { font-size: 0.73rem; background: #f1f5f9; padding: 2px 8px; border-radius: 4px; color: #64748b; }
	.desc { font-size: 0.76rem; color: #64748b; margin-top: 2px; }
	.card-right { min-width: 20px; }
	.arrow { color: #94a3b8; font-size: 0.9rem; display: inline-block; transition: transform 0.2s; }
	.arrow.open { transform: rotate(90deg); }

	.detail {
		background: #f8fafc; border: 1px solid #e2e8f0; border-top: none;
		border-radius: 0 0 10px 10px; padding: 0.8rem 1.2rem;
	}
	.detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; }
	@media (max-width: 600px) { .detail-grid { grid-template-columns: 1fr; } }
	.detail-item { }
	.dl { font-weight: 600; color: #4caf50; font-size: 0.72rem; margin-bottom: 2px; text-transform: uppercase; letter-spacing: 0.3px; }
	.detail code {
		display: block; background: #fff; padding: 5px 8px; border-radius: 6px;
		font-size: 0.75rem; border: 1px solid #e2e8f0; color: #475569; word-break: break-all;
	}
</style>
