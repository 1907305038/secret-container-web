<script lang="ts">
	import { onMount } from 'svelte';
	import { fly, fade, slide } from 'svelte/transition';
	import { quintOut, cubicOut } from 'svelte/easing';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import type { PodInfo } from '$lib/types';

	let pods = $state<PodInfo[]>([]);
	let total = $state(0); let confCount = $state(0);
	let showForm = $state(false); let msg = $state('');
	let expanded = $state<any>({});
	let sysData = $state<any>({});
	let loading = $state(true);

	let filterRuntime = $state('');
	let filterNs = $state('');

	$effect(() => {
		filterRuntime = $page.url.searchParams.get('runtime') || '';
		filterNs = $page.url.searchParams.get('ns') || '';
	});

	async function load() {
		loading = true;
		const params = new URLSearchParams();
		if (filterRuntime) params.set('runtime', filterRuntime);
		if (filterNs) params.set('ns', filterNs);
		const qs = params.toString();
		const [r1, r2] = await Promise.all([
			fetch('/api/pods' + (qs ? '?' + qs : '')),
			fetch('/api/pods?confidential=true')
		]);
		const all = await r1.json(); const conf = await r2.json();
		pods = all.pods; total = all.total; confCount = conf.total;
		loading = false;
	}
	onMount(load);

	function clearFilter() { goto('/pods'); }

	let form = $state({
		name: '', namespace: 'default', image: 'nginx:alpine', runtime: '',
		command: 'sleep 3600', args: '', cpu_req: '100m', mem_req: '128Mi',
		cpu_lim: '500m', mem_lim: '256Mi', labels: '', port: '80'
	});
	let formTab = $state<'basic'|'advanced'>('basic');

	function buildForm() {
		const labels: Record<string,string> = {};
		if (form.labels) form.labels.split(',').forEach((p: string) => {
			const [k, v] = p.split('='); if (k && v) labels[k.trim()] = v.trim();
		});
		const ports = form.port ? [{ name: 'http', container_port: parseInt(form.port), protocol: 'TCP' }] : [];
		return {
			name: form.name, namespace: form.namespace, image: form.image,
			runtime: form.runtime, command: form.command, args: form.args,
			ports,
			requests: { cpu: form.cpu_req, memory: form.mem_req },
			limits: { cpu: form.cpu_lim, memory: form.mem_lim },
			labels,
		};
	}
	let memResults = $state<Record<string,any>>({});
	let yamlData = $state<Record<string,string>>({});

	async function readMem(pid: number, ns?: string, pod?: string) {
		const key = String(pid);
		if (memResults[key]) { memResults[key] = null; memResults = { ...memResults }; return; }
		const params = ns && pod ? `?ns=${ns}&pod=${pod}` : '';
		const r = await fetch(`/api/proc/${pid}/mem${params}`);
		memResults[key] = await r.json();
		memResults = { ...memResults };
	}

	async function showYaml(ns: string, name: string) {
		const key = ns + '/' + name;
		if (yamlData[key]) { yamlData[key] = ''; yamlData = { ...yamlData }; return; }
		const r = await fetch(`/api/pods/${ns}/${name}/yaml`);
		const d = await r.json();
		yamlData[key] = d.yaml || d.error || '获取失败';
		yamlData = { ...yamlData };
	}

	async function toggleSys(ns: string, name: string) {
		const key = `${ns}/${name}`;
		if (expanded[key]) { expanded[key] = false; expanded = { ...expanded }; return; }
		expanded[key] = true; expanded = { ...expanded };
		const res = await fetch(`/api/pods/info/${ns}/${name}`);
		sysData[key] = await res.json();
		sysData = { ...sysData };
	}

	async function createPod() {
		msg = '创建中...';
		const res = await fetch('/api/pods/create', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(buildForm()) });
		const r = await res.json();
		msg = res.ok ? `✅ ${r.name} 已创建` : `❌ ${r.error}`;
		if (res.ok) { showForm = false; setTimeout(load, 3000); }
	}
	async function deletePod(ns: string, name: string) {
		msg = `删除 ${name}...`; await fetch(`/api/pods/${ns}/${name}`, { method: 'DELETE' });
		msg = `✅ ${name} 已删除`; setTimeout(load, 2000);
	}
	function quickTdx() {
		form = { name: 'tdx-' + Date.now().toString(36), namespace: 'default', image: 'docker.m.daocloud.io/library/nginx:alpine', runtime: 'kata-qemu-tdx', command: 'sleep 3600', args: '', cpu_req: '100m', mem_req: '128Mi', cpu_lim: '500m', mem_lim: '256Mi', labels: '', port: '80' };
		showForm = true;
	}
	function isTdx(r: string) { return r?.includes('tdx'); }
	function sizeFmt(kb: number) { return kb > 1048576 ? (kb/1048576).toFixed(1)+'GB' : kb > 1024 ? (kb/1024).toFixed(0)+'MB' : kb+'KB'; }
</script>

<div class="page-header">
	<h2>🖥️ 机密容器</h2>
	<div class="stats-row">
		<div class="stat-badge">机密 Pod: <b>{confCount}</b></div>
		<div class="stat-badge dim">总计: <b>{total}</b></div>
	</div>
</div>

<div class="toolbar">
	<div class="btns">
		<button onclick={()=>{form={name:'',namespace:'default',image:'nginx:alpine',runtime:'',command:'sleep 3600',args:'',cpu_req:'100m',mem_req:'128Mi',cpu_lim:'500m',mem_lim:'256Mi',labels:'',port:'80'};showForm=!showForm;msg='';formTab='basic'}} class="btn-primary">➕ 创建 Pod</button>
		<button onclick={quickTdx} class="btn-tdx">🛡️ 快速 TDX</button>
		<button onclick={load} class="btn-refresh" title="刷新">🔄</button>
	</div>
</div>

{#if filterRuntime || filterNs}
	<div class="filter-bar" in:fly={{ y: -4, duration: 200 }}>
		<span>🔍 筛选: </span>
		{#if filterRuntime}<span class="filter-tag">运行时: {filterRuntime} <button onclick={clearFilter}>✕</button></span>{/if}
		{#if filterNs}<span class="filter-tag">命名空间: {filterNs} <button onclick={clearFilter}>✕</button></span>{/if}
	</div>
{/if}

{#if msg}
	<div class="toast" in:fly={{ y: -8, duration: 200 }} out:fade>{{msg}}</div>
{/if}

{#if showForm}
	<div class="form-overlay" in:fade={{ duration: 150 }} out:fade>
		<div class="form-card" in:fly={{ y: 20, duration: 300, easing: cubicOut }}>
			<div class="form-tabs">
				<button class:active={formTab==='basic'} onclick={()=>formTab='basic'}>📦 基本配置</button>
				<button class:active={formTab==='advanced'} onclick={()=>formTab='advanced'}>⚙️ 资源 & 高级</button>
			</div>

			{#if formTab === 'basic'}
			<div class="form-body" in:fade={{ duration: 150 }}>
				<div class="form-row"><label>名称 *</label><input bind:value={form.name} placeholder="my-pod"/></div>
				<div class="form-row"><label>命名空间</label><input bind:value={form.namespace} placeholder="default"/></div>
				<div class="form-row"><label>镜像 *</label><input bind:value={form.image} placeholder="nginx:alpine"/></div>
				<div class="form-row">
					<label>运行时</label>
					<select bind:value={form.runtime}>
						<option value="">默认 (runc)</option>
						<option value="kata-qemu-tdx">kata-qemu-tdx 🔒 TDX 加密</option>
						<option value="kata-qemu-coco-dev">kata-qemu-coco-dev 🔐 开发</option>
						<option value="kata-qemu">kata-qemu (无加密)</option>
					</select>
				</div>
				<div class="form-row"><label>命令</label><input bind:value={form.command} placeholder="sleep 3600"/></div>
				<div class="form-row"><label>参数</label><input bind:value={form.args} placeholder="-c 'echo hello'"/></div>
				<div class="form-row"><label>端口</label><input bind:value={form.port} placeholder="80"/></div>
				<div class="form-row"><label>标签</label><input bind:value={form.labels} placeholder="env=prod, tier=frontend"/></div>
			</div>
			{:else}
			<div class="form-body" in:fade={{ duration: 150 }}>
				<div class="form-section">
					<div class="form-section-title">📊 资源请求 (Requests)</div>
					<div class="form-row"><label>CPU</label><input bind:value={form.cpu_req} placeholder="100m"/></div>
					<div class="form-row"><label>内存</label><input bind:value={form.mem_req} placeholder="128Mi"/></div>
				</div>
				<div class="form-section">
					<div class="form-section-title">📈 资源限制 (Limits)</div>
					<div class="form-row"><label>CPU</label><input bind:value={form.cpu_lim} placeholder="500m"/></div>
					<div class="form-row"><label>内存</label><input bind:value={form.mem_lim} placeholder="256Mi"/></div>
				</div>
				<div class="preset-btns">
					<button class="preset" onclick={()=>{form.cpu_req='100m';form.mem_req='128Mi';form.cpu_lim='500m';form.mem_lim='256Mi'}}>💡 小型</button>
					<button class="preset" onclick={()=>{form.cpu_req='500m';form.mem_req='512Mi';form.cpu_lim='1';form.mem_lim='1Gi'}}>📦 中型</button>
					<button class="preset" onclick={()=>{form.cpu_req='1';form.mem_req='1Gi';form.cpu_lim='2';form.mem_lim='2Gi'}}>🚀 大型</button>
				</div>
			</div>
			{/if}

			<div class="form-actions">
				<button onclick={() => showForm = false} class="btn-cancel">取消</button>
				<button onclick={createPod} class="btn-submit">🚀 创建 Pod</button>
			</div>
		</div>
	</div>
{/if}

<div class="list">
	{#each pods as pod, i (pod.namespace + '/' + pod.name)}
		{@const key = pod.namespace + '/' + pod.name}
		<div class="card-wrapper" in:fly={{ y: 10, delay: i * 30, duration: 250 }}>
			<div class="card {isTdx(pod.runtime_class)?'tdx':''}" onclick="{() => toggleSys(pod.namespace,pod.name)}" role="button" tabindex="0">
				<div class="card-left">
					<span class="badge {isTdx(pod.runtime_class)?'tdx':'normal'}">{isTdx(pod.runtime_class)?'🟢 TDX':'⚪ 普通'}</span>
				</div>
				<div class="card-main">
					<div class="card-top">
						<span class="name">{pod.name}</span>
						<span class="ns">{pod.namespace}</span>
						<span class="status-dot {pod.status === 'Running' ? 'running' : ''}"></span>
						<span class="status">{pod.status}</span>
					</div>
					<div class="card-sub">
						<span class="chip">IP: {pod.ip}</span>
						<span class="chip {isTdx(pod.runtime_class)?'tdx-chip':''}">{isTdx(pod.runtime_class)?'🔒 进程隐藏':'👁️ 进程可见'}</span>
					</div>
				</div>
				<div class="card-right">
					<button class="yaml-btn" onclick={(e) => { e.stopPropagation(); showYaml(pod.namespace, pod.name); }} title="查看 YAML">📋</button>
					<span class="arrow {expanded[key]?'open':''}">▸</span>
				</div>
			</div>
			{#if yamlData[key]}
				<div class="yaml-view" in:slide={{ duration: 200 }}>
					<div class="yaml-header">
						<span>📋 {pod.name}.yaml</span>
						<button class="yaml-close" onclick={(e) => { e.stopPropagation(); yamlData[key] = ''; yamlData = {...yamlData}; }}>✕</button>
					</div>
					<pre class="yaml-content">{yamlData[key]}</pre>
				</div>
			{/if}
			{#if expanded[key] && sysData[key]}
				<div class="detail" in:slide={{ duration: 250 }}>
					<div class="detail-grid">
						{#each Object.entries(sysData[key].info) as [label, val]}
							<div class="detail-item" in:fade={{ delay: 80 }}>
								<div class="dl">{label}</div>
								<pre>{val}</pre>
							</div>
						{/each}
					</div>
					{#if sysData[key].host_procs?.length || sysData[key].guest_procs?.length}
						<div class="procs-box" in:fade={{ delay: 150 }}>
							{#if sysData[key].host_procs?.length}
								<div class="procs-title">🖥️ 宿主机可见进程 ({sysData[key].host_procs.length} 个)</div>
								{#each sysData[key].host_procs as proc}
									<div class="proc-line" onclick={(e) => { e.stopPropagation(); readMem(proc.pid); }} role="button" tabindex="0">
										<code>PID {proc.pid}</code>
										<span>{proc.comm}</span>
										<span class="rss">{sizeFmt(proc.rss_kb)}</span>
										<span class="mem-hint">📖</span>
									</div>
									{#if memResults[String(proc.pid)]}
										<div class="mem-dump" in:fade={{ duration: 150 }} onclick={(e) => e.stopPropagation()}>
											<div class="mem-note {memResults[String(proc.pid)].tag || ''}">
												{memResults[String(proc.pid)].note}
											</div>
											<div class="evidence">{memResults[String(proc.pid)].evidence}</div>
										</div>
									{/if}
								{/each}
							{/if}
							{#if sysData[key].guest_procs?.length}
								<div class="procs-title guest-title">
									📦 容器内实际进程 ({sysData[key].guest_procs.length} 个)
									{#if sysData[key].is_tdx}
										<span class="hidden-badge">宿主机不可见</span>
									{/if}
								</div>
								{#each sysData[key].guest_procs as proc}
									<div class="proc-line guest" onclick={(e) => { e.stopPropagation(); readMem(proc.pid, sysData[key].pod?.split('/')[0], sysData[key].pod?.split('/')[1]); }} role="button" tabindex="0">
										<code>PID {proc.pid}</code>
										<span>{proc.comm}</span>
										<span class="mem-hint verify">📖 验证</span>
									</div>
									{#if memResults[String(proc.pid)]}
										<div class="mem-dump" in:fade={{ duration: 150 }} onclick={(e) => e.stopPropagation()}>
											<div class="mem-note {memResults[String(proc.pid)].tag || ''}">
												{memResults[String(proc.pid)].note}
											</div>
											<div class="evidence">{memResults[String(proc.pid)].evidence}</div>
										</div>
									{/if}
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			{/if}
			<button class="del-inline" onclick={(e) => { e.stopPropagation(); deletePod(pod.namespace,pod.name); }} title="删除">🗑️</button>
		</div>
	{/each}
</div>

{#if loading}
	<div class="loading">加载中...</div>
{/if}

<style>
	h2 { margin: 0; font-size: 1.4rem; }
	.page-header { display: flex; align-items: center; gap: 1rem; margin-bottom: 1rem; }
	.stats-row { display: flex; gap: 8px; }
	.stat-badge { background: #e8f5e9; color: #2e7d32; padding: 3px 12px; border-radius: 20px; font-size: 0.8rem; font-weight: 500; }
	.stat-badge.dim { background: #f0f0f0; color: #666; }
	.stat-badge b { font-weight: 700; }

	.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
	.btns { display: flex; gap: 8px; }
	button {
		padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer;
		font-size: 0.85rem; font-weight: 500; transition: all 0.15s;
	}
	.btn-primary { background: #1e293b; color: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
	.btn-primary:hover { background: #334155; transform: translateY(-1px); box-shadow: 0 2px 6px rgba(0,0,0,0.15); }
	.btn-tdx { background: #e8f5e9; color: #2e7d32; border: 1px solid #a5d6a7; }
	.btn-tdx:hover { background: #c8e6c9; }
	.btn-refresh { background: #fff; color: #64748b; font-size: 1.1rem; padding: 6px 10px; border-radius: 50%; }
	.btn-refresh:hover { background: #e2e8f0; }

	.filter-bar { display: flex; align-items: center; gap: 8px; padding: 8px 14px; background: #eff6ff; border: 1px solid #bfdbfe; border-radius: 10px; margin-bottom: 0.8rem; font-size: 0.82rem; color: #3b82f6; }
	.filter-tag { display: flex; align-items: center; gap: 4px; background: #dbeafe; padding: 2px 10px; border-radius: 6px; font-family: monospace; font-size: 0.76rem; }
	.filter-tag button { padding: 0 4px; border: none; background: none; cursor: pointer; font-size: 0.8rem; color: #64748b; }
	.filter-tag button:hover { color: #ef4444; }

	.toast {
		padding: 10px 16px; background: #1e293b; color: #fff; border-radius: 10px;
		margin-bottom: 0.8rem; font-size: 0.85rem; box-shadow: 0 4px 12px rgba(0,0,0,0.1);
	}

	.form-overlay {
		position: fixed; inset: 0; background: rgba(0,0,0,0.3); z-index: 100;
		display: flex; align-items: center; justify-content: center;
		backdrop-filter: blur(4px);
	}
	.form-card {
		background: #fff; border-radius: 16px; padding: 0; width: 480px; max-height: 80vh; overflow-y: auto;
		box-shadow: 0 20px 60px rgba(0,0,0,0.15);
	}
	.form-tabs { display: flex; border-bottom: 1px solid #e2e8f0; }
	.form-tabs button {
		flex: 1; padding: 12px; border: none; background: none; font-size: 0.85rem;
		font-weight: 500; color: #94a3b8; cursor: pointer; transition: all 0.15s;
		border-bottom: 2px solid transparent;
	}
	.form-tabs button.active { color: #1e293b; border-bottom-color: #4caf50; }
	.form-tabs button:hover:not(.active) { color: #64748b; }
	.form-body { padding: 1.2rem 1.5rem; }
	.form-section { margin-bottom: 1rem; }
	.form-section-title { font-size: 0.8rem; font-weight: 600; color: #64748b; margin-bottom: 6px; }
	.preset-btns { display: flex; gap: 6px; margin-top: 0.5rem; }
	.preset { padding: 5px 12px; background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 0.75rem; cursor: pointer; }
	.preset:hover { background: #e2e8f0; }
	.form-row { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
	.form-row label { min-width: 60px; font-size: 0.85rem; color: #64748b; font-weight: 500; }
	.form-row input, .form-row select {
		flex: 1; padding: 9px 12px; border: 1px solid #e2e8f0; border-radius: 8px;
		font-size: 0.85rem; background: #f8fafc; transition: border 0.15s;
	}
	.form-row input:focus, .form-row select:focus { outline: none; border-color: #4caf50; box-shadow: 0 0 0 3px rgba(76,175,80,0.1); }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 1rem; }
	.btn-cancel { background: #f1f5f9; color: #64748b; }
	.btn-cancel:hover { background: #e2e8f0; }
	.btn-submit { background: #4caf50; color: #fff; }
	.btn-submit:hover { background: #43a047; }

	.list { display: flex; flex-direction: column; gap: 2px; }
	.card-wrapper { position: relative; }
	.card {
		display: flex; align-items: center; gap: 12px;
		background: #fff; border: 1px solid #e8ecf1; border-radius: 10px;
		padding: 10px 14px; cursor: pointer; transition: all 0.2s;
	}
	.card:hover { border-color: #cbd5e1; box-shadow: 0 2px 8px rgba(0,0,0,0.04); transform: translateY(-1px); }
	.card.tdx { border-left: 4px solid #4caf50; background: linear-gradient(90deg, #f6fdf6 0%, #fff 100%); }
	.card-left { min-width: 70px; }
	.badge { font-weight: 600; font-size: 0.8rem; padding: 2px 10px; border-radius: 6px; }
	.badge.tdx { background: #e8f5e9; color: #2e7d32; }
	.badge.normal { background: #e3f2fd; color: #1565c0; }
	.card-main { flex: 1; min-width: 0; }
	.card-top { display: flex; gap: 10px; align-items: center; font-size: 0.88rem; }
	.name { font-weight: 600; color: #1e293b; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ns { color: #94a3b8; font-size: 0.78rem; min-width: 80px; }
	.status-dot { width: 6px; height: 6px; border-radius: 50%; background: #cbd5e1; }
	.status-dot.running { background: #4caf50; animation: pulse 2s infinite; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
	.status { color: #64748b; font-size: 0.78rem; }
	.card-sub { display: flex; gap: 8px; margin-top: 3px; }
	.chip { font-size: 0.73rem; padding: 2px 8px; border-radius: 4px; background: #f1f5f9; color: #64748b; }
	.chip.tdx-chip { background: #e8f5e9; color: #2e7d32; }
	.card-right { display: flex; align-items: center; gap: 4px; }
	.yaml-btn { background: none; border: none; font-size: 0.9rem; cursor: pointer; padding: 2px 4px; border-radius: 4px; opacity: 0; transition: all 0.15s; }
	.card-wrapper:hover .yaml-btn { opacity: 0.5; }
	.yaml-btn:hover { opacity: 1 !important; background: #e8f5e9; }
	.arrow { color: #94a3b8; font-size: 0.9rem; display: inline-block; transition: transform 0.2s; }
	.arrow.open { transform: rotate(90deg); }

	.yaml-view { margin-top: -1px; background: #1e293b; border-radius: 0 0 10px 10px; overflow: hidden; }
	.yaml-header { display: flex; justify-content: space-between; align-items: center; padding: 8px 14px; background: #334155; color: #e2e8f0; font-size: 0.8rem; font-weight: 600; }
	.yaml-close { background: none; border: none; color: #94a3b8; cursor: pointer; font-size: 1rem; padding: 2px 6px; border-radius: 4px; }
	.yaml-close:hover { color: #fff; background: #475569; }
	.yaml-content { margin: 0; padding: 12px 14px; font-size: 0.68rem; line-height: 1.5; color: #a5d6a7; white-space: pre; overflow-x: auto; max-height: 400px; overflow-y: auto; }

	.detail {
		background: #f8fafc; border: 1px solid #e2e8f0; border-top: none;
		border-radius: 0 0 10px 10px; padding: 1rem 1.2rem; margin-top: -1px;
	}
	.detail-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.6rem; }
	@media (max-width: 600px) { .detail-grid { grid-template-columns: 1fr; } }
	.detail-item { }
	.dl { font-weight: 600; color: #4caf50; font-size: 0.76rem; margin-bottom: 2px; text-transform: uppercase; letter-spacing: 0.3px; }
	.detail pre {
		background: #fff; padding: 6px 10px; border-radius: 6px; font-size: 0.74rem;
		margin: 0; white-space: pre-wrap; word-break: break-all;
		border: 1px solid #e2e8f0; color: #475569;
	}

	.procs-box { margin-top: 0.8rem; border-top: 1px solid #e2e8f0; padding-top: 0.6rem; }
	.procs-title { font-size: 0.78rem; color: #475569; margin-bottom: 4px; font-weight: 600; }
	.guest-title { margin-top: 8px; }
	.hidden-badge { display: inline-block; background: #ff9800; color: #fff; font-size: 0.62rem; padding: 1px 7px; border-radius: 10px; margin-left: 6px; font-weight: 500; text-transform: uppercase; }
	.proc-line {
		display: flex; align-items: center; gap: 8px; padding: 5px 8px;
		font-size: 0.77rem; cursor: pointer; border-radius: 6px; transition: background 0.12s;
	}
	.proc-line:hover { background: #e8f0fe; }
	.proc-line.guest { background: #fffbf0; cursor: pointer; }
	.proc-line.guest:hover { background: #fff3d6; }
	.proc-line code { background: #e2e8f0; padding: 2px 6px; border-radius: 4px; font-size: 0.74rem; font-weight: 600; color: #334155; }
	.rss { color: #94a3b8; font-size: 0.72rem; }
	.mem-hint { color: #3b82f6; font-size: 0.74rem; margin-left: auto; cursor: pointer; }
	.mem-hint:hover { color: #2563eb; }
	.mem-hint.verify { color: #f59e0b; }
	.mem-hint.verify:hover { color: #d97706; }

	.mem-dump { margin: 3px 0 6px 12px; padding: 8px 12px; background: #1e293b; border-radius: 8px; }
	.mem-note { font-size: 0.76rem; font-weight: 600; margin-bottom: 4px; line-height: 1.4; }
	.mem-note.qemu { color: #fbbf24; }
	.mem-note.shim { color: #4ade80; }
	.evidence { font-size: 0.68rem; color: #94a3b8; font-family: monospace; line-height: 1.4; }

	.del-inline {
		position: absolute; right: 8px; top: 50%; transform: translateY(-50%);
		padding: 4px 8px; border: none; background: none; font-size: 0.85rem;
		cursor: pointer; opacity: 0; border-radius: 6px; transition: all 0.15s; z-index: 2;
	}
	.card-wrapper:hover .del-inline { opacity: 0.5; }
	.del-inline:hover { opacity: 1 !important; background: #fee2e2; color: #ef4444; }

	.loading { text-align: center; padding: 2rem; color: #94a3b8; animation: pulse 1.5s infinite; }
</style>
