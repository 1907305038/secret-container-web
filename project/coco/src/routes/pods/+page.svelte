<script lang="ts">
	import { onMount } from 'svelte';
	import { fly, fade, slide } from 'svelte/transition';
	import { quintOut, cubicOut } from 'svelte/easing';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import type { PodInfo, WsEvent, PodEvent, MemoryEncryptProof, WriteAndReadResult, MemoryRegion } from '$lib/types';
	import HexDump from '$lib/components/HexDump.svelte';

	let pods = $state<PodInfo[]>([]);
	let total = $state(0); let confCount = $state(0);
	let showForm = $state(false); let msg = $state('');
	let expanded = $state<any>({});
	let sysData = $state<any>({});
	let loading = $state(true);

	let filterRuntime = $state('');
	let filterNs = $state('');

	// WebSocket 实时状态
	let ws: WebSocket;
	let podPhases = $state<Record<string, string>>({});
	let deletingPods = $state<Set<string>>(new Set());

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

	function connectWS() {
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		ws = new WebSocket(`${proto}//${location.host}/ws/state`);
		ws.onmessage = (e) => {
			try {
				const evt: WsEvent = JSON.parse(e.data);
				if (evt.type === 'pod_created') {
					msg = `✅ ${String(evt.name || '')} 已创建，等待启动...`;
					setTimeout(load, 1500);
				}
				if (evt.type === 'pod_deleted') {
					msg = `🗑️ ${String(evt.name || '')} 已删除`;
					const key = (evt.namespace || 'default') + '/' + (evt.name || '');
					// 仅当不是自己触发的删除时才处理
					if (!deletingPods.has(key)) {
						deletingPods.add(key);
						deletingPods = new Set(deletingPods);
						setTimeout(() => {
							deletingPods.delete(key);
							deletingPods = new Set(deletingPods);
							load();
						}, 600);
					}
				}
				if (evt.type === 'pod_phase' && evt.name && evt.namespace) {
					const key = evt.namespace + '/' + evt.name;
					podPhases[key] = evt.phase || '';
					podPhases = { ...podPhases };
				}
				if (evt.type === 'pod_count') { load(); }
			} catch { /* ignore parse errors */ }
		};
		ws.onclose = () => { setTimeout(connectWS, 5000); };
	}
	onMount(() => { load(); connectWS(); return () => ws?.close(); });

	function clearFilter() { goto('/pods'); }

	let form = $state({
		name: '', namespace: 'default', image: 'docker.m.daocloud.io/library/nginx:alpine', runtime: '',
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

	// K8s Events 时间线
	let podEvents = $state<Record<string, PodEvent[]>>({});
	// 内存加密验证
	let encryptProofs = $state<Record<string, MemoryEncryptProof>>({});
	let proofLoading = $state<Record<string, boolean>>({});

	// 写入数据 + 读取内存对比（数组，保留所有写入历史）
	let writeResults = $state<Record<string, WriteAndReadResult[]>>({});
	let writeLoading = $state<Record<string, boolean>>({});
	let customData = $state<Record<string, string>>({});
	let showWriteForm = $state<Record<string, boolean>>({});
	let showMemRegions = $state<Record<string, boolean>>({}); // 内存区域折叠

	async function writeAndRead(ns: string, name: string) {
		const key = `${ns}/${name}`;
		writeLoading[key] = true;
		writeLoading = { ...writeLoading };
		const r = await fetch('/api/demo/write-and-read', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ pod: name, ns, data: customData[key] || '' })
		});
		const d = await r.json();
		// 追加到数组，不覆盖
		writeResults[key] = [...(writeResults[key] || []), d];
		writeResults = { ...writeResults };
		writeLoading[key] = false;
		writeLoading = { ...writeLoading };
	}

	async function readMemOnly(ns: string, name: string) {
		const key = `${ns}/${name}`;
		writeLoading[key] = true;
		writeLoading = { ...writeLoading };
		const r = await fetch('/api/demo/read-mem', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ pod: name, ns })
		});
		const d = await r.json();
		const arr = writeResults[key] || [];
		// 去重：如果最新一条数据相同，不追加
		const last = arr[arr.length - 1];
		if (!last || last.plaintext !== d.plaintext || last.plaintext_found !== d.plaintext_found) {
			writeResults[key] = [...arr, d];
			writeResults = { ...writeResults };
		}
		writeLoading[key] = false;
		writeLoading = { ...writeLoading };
	}

	// 查看内存数据弹窗
	let memModal = $state<{ podKey: string; idx: number } | null>(null);

	function openMemModal(podKey: string, idx: number) { memModal = { podKey, idx }; }
	function closeMemModal() { memModal = null; }

	// 删除某条写入数据
	async function deleteProof(ns: string, name: string, idx: number) {
		const key = `${ns}/${name}`;
		await fetch('/api/demo/delete-proof', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ pod: name, ns, idx: idx + 1 }) // idx+1 = proof_N 的 N
		});
		const arr = (writeResults[key] || []).filter((_, i) => i !== idx);
		writeResults[key] = arr;
		writeResults = { ...writeResults };
		if (memModal?.podKey === key && memModal.idx >= arr.length) closeMemModal();
	}

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

	// 获取 K8s Events 时间线
	async function fetchEvents(ns: string, name: string) {
		const key = `${ns}/${name}`;
		if (podEvents[key]) return;
		const r = await fetch(`/api/pods/${ns}/${name}/events`);
		const d = await r.json();
		podEvents[key] = d.events || [];
		podEvents = { ...podEvents };
	}

	// 全自动内存加密验证
	async function autoEncryptProof(ns: string, name: string) {
		const key = `${ns}/${name}`;
		proofLoading[key] = true;
		proofLoading = { ...proofLoading };
		const r = await fetch(`/api/demo/memory-encrypt?pod=${encodeURIComponent(name)}&ns=${encodeURIComponent(ns)}`);
		const d = await r.json();
		encryptProofs[key] = d;
		encryptProofs = { ...encryptProofs };
		proofLoading[key] = false;
		proofLoading = { ...proofLoading };
	}

	// 半自动内存加密验证（指定宿主机 PID）
	async function manualEncryptCompare(ns: string, name: string, pid: number) {
		const key = `${ns}/${name}`;
		proofLoading[key] = true;
		proofLoading = { ...proofLoading };
		const r = await fetch(`/api/demo/memory-compare?pod=${encodeURIComponent(name)}&ns=${encodeURIComponent(ns)}&pid=${pid}`);
		const d = await r.json();
		encryptProofs[key] = d;
		encryptProofs = { ...encryptProofs };
		proofLoading[key] = false;
		proofLoading = { ...proofLoading };
	}

	async function toggleSys(ns: string, name: string) {
		const key = `${ns}/${name}`;
		if (expanded[key]) { expanded[key] = false; expanded = { ...expanded }; return; }
		expanded[key] = true; expanded = { ...expanded };
		const res = await fetch(`/api/pods/info/${ns}/${name}`);
		sysData[key] = await res.json();
		sysData = { ...sysData };
		// 同时获取 Events 时间线
		fetchEvents(ns, name);
	}

	async function createPod() {
		msg = '创建中...';
		try {
			const res = await fetch('/api/pods/create', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(buildForm()) });
			const r = await res.json();
			const name = String(r.name || '');
			const err = String(r.error || '');
			msg = res.ok ? `✅ ${name} 已创建` : `❌ ${err}`;
			if (res.ok) { showForm = false; setTimeout(load, 1500); }
		} catch (e) {
			msg = `❌ 创建失败: ${String(e)}`;
		}
	}
	async function deletePod(ns: string, name: string) {
		// 立即触发删除动画，不等网络响应
		const key = ns + '/' + name;
		deletingPods.add(key);
		deletingPods = new Set(deletingPods);
		msg = `删除 ${String(name)}...`;
		try {
			await fetch(`/api/pods/${ns}/${name}`, { method: 'DELETE' });
			msg = `✅ ${String(name)} 已删除`;
		} catch (e) {
			msg = `❌ 删除失败: ${String(e)}`;
			deletingPods.delete(key);
			deletingPods = new Set(deletingPods);
		}
		// 0.8 秒后刷新列表（等 K8s 完成清理）
		setTimeout(() => { deletingPods.delete(key); deletingPods = new Set(deletingPods); load(); }, 800);
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
		<button onclick={()=>{form={name:'',namespace:'default',image:'docker.m.daocloud.io/library/nginx:alpine',runtime:'',command:'sleep 3600',args:'',cpu_req:'100m',mem_req:'128Mi',cpu_lim:'500m',mem_lim:'256Mi',labels:'',port:'80'};showForm=!showForm;msg='';formTab='basic'}} class="btn-primary">➕ 创建 Pod</button>
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
		{@const phase = podPhases[key]}
		{@const isDeleting = deletingPods.has(key)}
		<div class="card-wrapper" class:deleting={isDeleting} in:fly={{ y: 10, delay: i * 30, duration: 250 }} out:fly={{ y: -10, duration: 200 }}>
			<div class="card {isTdx(pod.runtime_class)?'tdx':''}" onclick="{() => toggleSys(pod.namespace,pod.name)}" role="button" tabindex="0">
				<div class="card-left">
					<span class="badge {isTdx(pod.runtime_class)?'tdx':'normal'}">{isTdx(pod.runtime_class)?'🟢 TDX':'⚪ 普通'}</span>
				</div>
				<div class="card-main">
					<div class="card-top">
						<span class="name">{pod.name}</span>
						<span class="ns">{pod.namespace}</span>
						{#if phase && phase !== 'Running'}
							<span class="phase-badge phase-{phase.toLowerCase()}">{phase}</span>
						{/if}
						<span class="status-dot {pod.status === 'Running' ? 'running' : ''}"></span>
						<span class="status">{pod.status}</span>
					</div>
					<div class="card-sub">
						<span class="chip">IP: {pod.ip}</span>
						<span class="chip {isTdx(pod.runtime_class)?'tdx-chip':''}">{isTdx(pod.runtime_class)?'🔒 进程隐藏':'👁️ 进程可见'}</span>
					</div>
				</div>
				<div class="card-right">
					<button class="act-btn write-btn" onclick={(e) => { e.stopPropagation(); showWriteForm[key] = !showWriteForm[key]; showWriteForm = {...showWriteForm}; }} title="写入数据">📝</button>
					<button class="act-btn yaml-btn" onclick={(e) => { e.stopPropagation(); showYaml(pod.namespace, pod.name); }} title="YAML">📋</button>
					<button class="act-btn del-btn" onclick={(e) => { e.stopPropagation(); deletePod(pod.namespace,pod.name); }} title="删除">🗑️</button>
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
			<!-- 写入数据面板 -->
			{#if showWriteForm[key]}
				<div class="write-panel" in:slide={{ duration: 200 }}>
					<div class="write-input-row">
						<input class="write-input" type="text" placeholder="输入要写入的数据，留空自动生成" value={customData[key] || ''} oninput={(e) => { customData[key] = e.target.value; customData = {...customData}; }} />
						<button class="write-act-btn" onclick={(e) => { e.stopPropagation(); writeAndRead(pod.namespace, pod.name); }} disabled={writeLoading[key]}>
							{writeLoading[key] ? '⏳' : '📝'} 写入
						</button>
						<button class="write-act-btn read" onclick={(e) => { e.stopPropagation(); readMemOnly(pod.namespace, pod.name); }} disabled={writeLoading[key]}>
							🔍 读取
						</button>
						<button class="write-act-btn close" onclick={(e) => { e.stopPropagation(); showWriteForm[key] = false; showWriteForm = {...showWriteForm}; }}>✕</button>
					</div>
					{#if writeResults[key]?.length}
						{#each writeResults[key] as wr, idx}
							{@const isLast = idx === writeResults[key].length - 1}
							<div class="write-result" class:latest={isLast} in:fade={{ delay: 80 }}>
								<div class="write-result-header">
									<span class="write-idx">#{idx + 1}</span>
									<code>{wr.plaintext}</code>
									{#if wr.guest_confirmed}
										<span class="write-badge safe">✅ 容器内存在</span>
									{/if}
									<span class="write-badge {wr.plaintext_found ? 'found' : 'safe'}">
										{wr.plaintext_found ? '⚠️ 宿主机可读' : '✅ 加密保护'}
									</span>
									<div class="write-row-actions">
										{#if wr.memory_regions?.length}
											<button class="wr-act view" onclick={(e) => { e.stopPropagation(); openMemModal(key, idx); }}>📄 查看内存</button>
										{/if}
										<button class="wr-act del" onclick={(e) => { e.stopPropagation(); deleteProof(pod.namespace, pod.name, idx); }}>🗑️</button>
									</div>
								</div>
								<div class="write-note">{wr.note}</div>
								{#if isLast && wr.memory_regions?.length}
									{@const regKey = key + '_' + idx}
									<button class="mem-toggle" onclick={(e) => { e.stopPropagation(); showMemRegions[regKey] = !showMemRegions[regKey]; showMemRegions = {...showMemRegions}; }}>
										{showMemRegions[regKey] ? '🔼' : '🔽'} 内存区域 (PID: {wr.host_pid}, {wr.memory_regions.length} 个)
									</button>
									{#if showMemRegions[regKey]}
										<div class="write-regions" in:fade={{ duration: 150 }}>
											{#each wr.memory_regions.slice(0, 4) as region}
												<div class="write-region">
													<div class="wr-addr">{region.address}</div>
													<HexDump hexData={region.hex_dump || ''} asciiSafe={region.ascii_safe || ''} label={region.name} entropy={region.entropy} variant={wr.plaintext_found ? 'plain' : 'cipher'} />
												</div>
											{/each}
										</div>
									{/if}
								{/if}
							</div>
						{/each}
					{/if}
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
								{#if sysData[key].is_tdx}
									<div class="procs-title">🖥️ 宿主机可见 — QEMU 虚拟机壳 ({sysData[key].host_procs.length} 个)</div>
									<div class="procs-hint">⚠️ 这只是 Kata 虚拟机的 QEMU 外壳进程，不是容器内的进程。宿主机无法看到容器内真实进程。</div>
								{:else}
									<div class="procs-title">🖥️ 宿主机可见进程 ({sysData[key].host_procs.length} 个)</div>
									<div class="procs-hint normal">普通容器共享宿主机内核，宿主机可直接看到容器对应的 containerd-shim 进程。</div>
								{/if}
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

					<!-- K8s Events 时间线 -->
					{#if podEvents[key]?.length}
						<div class="timeline" in:fade={{ delay: 200 }}>
							<div class="timeline-title">📜 生命周期</div>
							{#each podEvents[key] as evt}
								<div class="timeline-item">
									<span class="tl-dot {evt.type === 'Warning' ? 'warn' : 'normal'}"></span>
									<span class="tl-time">{evt.timestamp}</span>
									<span class="tl-reason">{evt.reason}</span>
									<span class="tl-msg">{evt.message}</span>
								</div>
							{/each}
						</div>
					{/if}

					<!-- 内存加密验证面板 (仅 TDX Pod) -->
					{#if sysData[key]?.is_tdx}
						<div class="encrypt-panel" in:fade={{ delay: 250 }}>
							<div class="encrypt-header">
								<span>🔐 内存加密验证</span>
								<div class="encrypt-btns">
									<button onclick={(e) => { e.stopPropagation(); autoEncryptProof(pod.namespace, pod.name); }}
										disabled={proofLoading[key]}>
										{proofLoading[key] ? '⏳ 验证中...' : '🔄 一键验证 (全自动)'}
									</button>
									{#if sysData[key].host_procs?.[0]?.pid}
										<button onclick={(e) => { e.stopPropagation(); manualEncryptCompare(pod.namespace, pod.name, sysData[key].host_procs[0].pid); }}
											disabled={proofLoading[key]}>
											🔍 手动对比 (半自动)
										</button>
									{/if}
								</div>
							</div>

							{#if encryptProofs[key]}
								{@const proof = encryptProofs[key]}
								{#if proof.error}
									<div class="encrypt-error">{proof.error}</div>
								{:else if proof.hint}
									<div class="encrypt-hint">{proof.hint}</div>
								{:else}
									{#if proof.auto_mode && proof.plaintext}
										<div class="encrypt-plaintext">
											测试明文: <code>{proof.plaintext}</code>
										</div>
									{/if}
									<div class="encrypt-compare">
										<div class="encrypt-col host-col">
											<div class="encrypt-col-title">🖥️ 宿主机视角 (Host)</div>
											<div class="encrypt-col-sub">QEMU PID: {proof.qemu_pid}</div>
											{#if proof.host_view.regions?.length}
												{#each proof.host_view.regions as region}
													<HexDump
														hexData={region.hex_dump || ''}
														asciiSafe={region.ascii_safe || ''}
														label={region.name + ' ' + region.address}
														entropy={region.entropy}
														variant={region.readable ? 'cipher' : 'unknown'}
													/>
												{/each}
											{/if}
											<div class="encrypt-summary {proof.host_view.found ? 'found' : 'not-found'}">
												{proof.host_view.note}
											</div>
										</div>
										<div class="encrypt-col guest-col">
											<div class="encrypt-col-title">📦 容器内视角 (Guest)</div>
											<div class="encrypt-col-sub">kubectl exec</div>
											{#if proof.guest_view.regions?.length}
												{#each proof.guest_view.regions as region}
													<HexDump
														hexData={region.hex_dump || ''}
														asciiSafe={region.ascii_safe || ''}
														label={region.name}
														entropy={region.entropy}
														variant={region.readable ? 'plain' : 'unknown'}
													/>
												{/each}
											{/if}
											<div class="encrypt-summary {proof.guest_view.found ? 'found' : 'not-found'}">
												{proof.guest_view.note}
											</div>
										</div>
									</div>
									{#if !proof.auto_mode}
										<div class="encrypt-semi-hint">
											💡 半自动模式：请在容器内执行 <code>echo "TEST" &gt; /dev/shm/proof.txt</code> 后使用全自动验证
										</div>
									{/if}
								{/if}
							{/if}
						</div>
					{/if}
				</div>
			{/if}
	
		</div>
	{/each}
</div>

{#if loading}
	<div class="loading">加载中...</div>
{/if}

<!-- 内存数据弹窗 -->
{#if memModal}
	{@const modalKey = memModal.podKey}
	{@const modalIdx = memModal.idx}
	{@const modalWR = writeResults[modalKey]?.[modalIdx]}
	<div class="mem-modal-overlay" onclick={closeMemModal} in:fade={{ duration: 150 }}>
		<div class="mem-modal" onclick={(e) => e.stopPropagation()} in:fly={{ y: 20, duration: 250 }}>
			<div class="mem-modal-header">
				<span>📄 内存数据 — {modalWR?.plaintext || ''}</span>
				<button class="mem-modal-close" onclick={closeMemModal}>✕</button>
			</div>
			<div class="mem-modal-body">
				{#if modalWR?.memory_regions?.length}
					<div class="write-regions-title">PID: {modalWR.host_pid} — {modalWR.memory_regions.length} 个区域</div>
					{#each modalWR.memory_regions as region}
						<div class="write-region">
							<div class="wr-addr">{region.address}</div>
							<HexDump hexData={region.hex_dump || ''} asciiSafe={region.ascii_safe || ''} label={region.name} entropy={region.entropy} variant={modalWR.plaintext_found ? 'plain' : 'cipher'} />
						</div>
					{/each}
				{:else}
					<div class="mem-modal-empty">暂无内存数据</div>
				{/if}
			</div>
		</div>
	</div>
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
	.card-left { min-width: 60px; }
	.badge { font-weight: 600; font-size: 0.75rem; padding: 2px 8px; border-radius: 6px; }
	.badge.tdx { background: #e8f5e9; color: #2e7d32; }
	.badge.normal { background: #e3f2fd; color: #1565c0; }
	.card-main { flex: 1; min-width: 0; padding-right: 8px; }
	.card-top { display: flex; gap: 5px; align-items: center; font-size: 0.83rem; }
	.name { font-weight: 600; color: #1e293b; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 140px; }
	.ns { color: #94a3b8; font-size: 0.7rem; min-width: 50px; flex-shrink: 0; }
	.status-dot { width: 5px; height: 5px; border-radius: 50%; background: #cbd5e1; flex-shrink: 0; }
	.status-dot.running { background: #4caf50; animation: pulse 2s infinite; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
	.status { color: #64748b; font-size: 0.74rem; flex-shrink: 0; }
	.card-sub { display: flex; gap: 6px; margin-top: 2px; }
	.chip { font-size: 0.7rem; padding: 1px 6px; border-radius: 4px; background: #f1f5f9; color: #64748b; }
	.chip.tdx-chip { background: #e8f5e9; color: #2e7d32; }
	.card-right { display: flex; align-items: center; gap: 8px; flex-shrink: 0; padding-left: 6px; }
	.act-btn { background: none; border: none; cursor: pointer; border-radius: 4px; opacity: 0; transition: all 0.15s; padding: 3px 4px; font-size: 0.82rem; line-height: 1; }
	.card-wrapper:hover .act-btn { opacity: 0.5; }
	.act-btn:hover { opacity: 1 !important; }
	.write-btn:hover { background: #e8f5e9; }

	/* 写入输入框 + 按钮行 */
	.write-input-row { display: flex; gap: 6px; margin: 8px 0; align-items: center; }
	.write-input { flex: 1; padding: 6px 10px; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 0.76rem; background: #f8fafc; font-family: monospace; }
	.write-input:focus { outline: none; border-color: #4caf50; }
	.write-act-btn { padding: 6px 12px; border: 1px solid #e2e8f0; border-radius: 6px; background: #fff; font-size: 0.74rem; cursor: pointer; white-space: nowrap; transition: all 0.15s; }
	.write-act-btn:hover:not(:disabled) { background: #e8f5e9; border-color: #4caf50; }
	.write-act-btn.read:hover:not(:disabled) { background: #eff6ff; border-color: #3b82f6; }
	.write-act-btn.close { background: none; border: none; color: #94a3b8; font-size: 0.9rem; }
	.write-act-btn.close:hover { color: #ef4444; background: #fee2e2 !important; }
	.write-act-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	/* 写入面板 */
	.write-panel { background: #f8fafc; border: 1px solid #e2e8f0; border-top: none; border-radius: 0 0 10px 10px; padding: 10px 14px; margin-top: -1px; }
	.yaml-btn:hover { background: #e8f5e9; }
	.del-btn:hover { background: #fee2e2; color: #ef4444; }
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
	.procs-hint {
		font-size: 0.68rem; color: #b45309; background: #fffbeb; padding: 4px 10px;
		border-radius: 6px; margin-bottom: 6px; line-height: 1.4;
		border-left: 3px solid #f59e0b;
	}
	.procs-hint.normal {
		color: #64748b; background: #f8fafc; border-left-color: #94a3b8;
	}
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



	/* 删除动画 */
	.card-wrapper.deleting {
		opacity: 0.3; transform: scale(0.95); pointer-events: none;
		transition: all 0.4s ease;
	}

	/* Pod 阶段徽章 */
	.phase-badge {
		font-size: 0.68rem; padding: 1px 8px; border-radius: 10px;
		font-weight: 600; text-transform: uppercase; letter-spacing: 0.3px;
	}
	.phase-badge.phase-pending { background: #fff3e0; color: #e65100; }
	.phase-badge.phase-containercreating { background: #e3f2fd; color: #1565c0; animation: pulse 1.5s infinite; }
	.phase-badge.phase-terminating { background: #fce4ec; color: #c62828; }

	/* K8s Events 时间线 */
	.timeline {
		margin-top: 0.8rem; border-top: 1px solid #e2e8f0; padding-top: 0.6rem;
	}
	.timeline-title {
		font-size: 0.78rem; font-weight: 600; color: #475569; margin-bottom: 6px;
	}
	.timeline-item {
		display: flex; align-items: baseline; gap: 8px; padding: 3px 0;
		font-size: 0.72rem; position: relative; padding-left: 16px;
	}
	.timeline-item::before {
		content: ''; position: absolute; left: 4px; top: 8px; bottom: -4px;
		width: 1px; background: #e2e8f0;
	}
	.timeline-item:last-child::before { display: none; }
	.tl-dot {
		position: absolute; left: 0; top: 6px;
		width: 8px; height: 8px; border-radius: 50%;
	}
	.tl-dot.normal { background: #4caf50; }
	.tl-dot.warn { background: #f59e0b; }
	.tl-time { color: #94a3b8; min-width: 56px; font-family: monospace; }
	.tl-reason { color: #334155; font-weight: 600; min-width: 80px; }
	.tl-msg { color: #64748b; flex: 1; }

	/* 内存加密验证面板 */
	.encrypt-panel {
		margin-top: 0.8rem; border-top: 1px solid #e2e8f0; padding-top: 0.6rem;
	}
	.encrypt-header {
		display: flex; justify-content: space-between; align-items: center;
		font-size: 0.78rem; font-weight: 600; color: #475569; margin-bottom: 8px;
		flex-wrap: wrap; gap: 6px;
	}
	.encrypt-btns {
		display: flex; gap: 6px;
	}
	.encrypt-btns button {
		padding: 5px 10px; border: 1px solid #e2e8f0; border-radius: 6px;
		background: #fff; font-size: 0.7rem; font-weight: 500; cursor: pointer;
		transition: all 0.15s;
	}
	.encrypt-btns button:hover:not(:disabled) { background: #f1f5f9; border-color: #cbd5e1; }
	.encrypt-btns button:disabled { opacity: 0.5; cursor: not-allowed; }
	.encrypt-plaintext {
		font-size: 0.74rem; color: #64748b; margin-bottom: 6px;
	}
	.encrypt-plaintext code {
		background: #e8f5e9; color: #2e7d32; padding: 2px 6px; border-radius: 4px;
		font-size: 0.72rem; font-weight: 600;
	}
	.encrypt-compare {
		display: grid; grid-template-columns: 1fr 1fr; gap: 8px;
	}
	@media (max-width: 700px) { .encrypt-compare { grid-template-columns: 1fr; } }
	.encrypt-col {
		border: 1px solid #e2e8f0; border-radius: 8px; padding: 8px;
	}
	.host-col { border-left: 3px solid #ef4444; }
	.guest-col { border-left: 3px solid #4caf50; }
	.encrypt-col-title {
		font-size: 0.72rem; font-weight: 600; color: #334155; margin-bottom: 2px;
	}
	.encrypt-col-sub {
		font-size: 0.66rem; color: #94a3b8; margin-bottom: 6px;
	}
	.encrypt-summary {
		font-size: 0.72rem; font-weight: 600; padding: 6px 8px; border-radius: 6px; margin-top: 4px;
	}
	.encrypt-summary.found { background: #fef3c7; color: #92400e; }
	.encrypt-summary.not-found { background: #dcfce7; color: #166534; }
	.encrypt-error { color: #ef4444; font-size: 0.74rem; }
	.encrypt-hint { color: #f59e0b; font-size: 0.74rem; }
	.encrypt-semi-hint {
		margin-top: 8px; font-size: 0.7rem; color: #94a3b8;
		background: #f8fafc; padding: 6px 10px; border-radius: 6px;
	}
	.encrypt-semi-hint code {
		background: #e2e8f0; padding: 1px 4px; border-radius: 3px; font-size: 0.68rem;
	}

	/* 写入按钮 */


	/* 写入结果面板 */
	.write-result {
		margin-top: 4px; background: #fff; border: 1px solid #e2e8f0;
		border-radius: 8px; padding: 8px 10px;
	}
	.write-result.latest { border-color: #3b82f6; box-shadow: 0 0 0 1px #bfdbfe; }
	.write-result-header {
		font-size: 0.78rem; font-weight: 600; margin-bottom: 4px;
		display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
	}
	.write-result-header code {
		background: #1e293b; color: #4ade80; padding: 2px 8px;
		border-radius: 4px; font-size: 0.72rem;
	}
	.write-idx {
		font-size: 0.6rem; background: #e2e8f0; color: #64748b;
		padding: 1px 5px; border-radius: 4px; font-weight: 700; margin-right: 2px;
	}
	.write-badge {
		font-size: 0.68rem; padding: 2px 8px; border-radius: 10px; font-weight: 600;
	}
	.write-badge.found { background: #fef3c7; color: #92400e; }
	.write-badge.safe { background: #dcfce7; color: #166534; }
	.write-note {
		font-size: 0.74rem; color: #475569; margin-bottom: 6px;
		padding: 4px 8px; background: #fff; border-radius: 4px;
		border-left: 3px solid #3b82f6;
	}
	.write-regions { margin-top: 4px; }
	.write-regions-title {
		font-size: 0.7rem; color: #94a3b8; margin-bottom: 4px;
		font-family: monospace;
	}
	.write-region { margin-bottom: 3px; }
	.wr-addr {
		font-size: 0.65rem; color: #64748b; font-family: monospace;
		margin-bottom: 2px;
	}
	.mem-toggle {
		font-size: 0.7rem; color: #64748b; background: #f1f5f9; border: 1px solid #e2e8f0;
		border-radius: 4px; padding: 3px 8px; cursor: pointer; margin-top: 4px; transition: all 0.15s;
	}
	.mem-toggle:hover { background: #e2e8f0; border-color: #94a3b8; }

	/* 行内操作按钮 */
	.write-row-actions { display: flex; gap: 4px; margin-left: auto; }
	.wr-act {
		font-size: 0.65rem; padding: 2px 6px; border: 1px solid #e2e8f0;
		border-radius: 4px; background: #fff; cursor: pointer; transition: all 0.15s;
	}
	.wr-act.view:hover { background: #eff6ff; border-color: #3b82f6; color: #2563eb; }
	.wr-act.del:hover { background: #fee2e2; border-color: #ef4444; color: #dc2626; }

	/* 内存数据弹窗 */
	.mem-modal-overlay {
		position: fixed; inset: 0; background: rgba(0,0,0,0.4); z-index: 200;
		display: flex; align-items: center; justify-content: center;
		backdrop-filter: blur(4px);
	}
	.mem-modal {
		background: #fff; border-radius: 12px; width: 90vw; max-width: 700px;
		max-height: 80vh; overflow-y: auto; box-shadow: 0 20px 60px rgba(0,0,0,0.2);
	}
	.mem-modal-header {
		display: flex; justify-content: space-between; align-items: center;
		padding: 12px 16px; border-bottom: 1px solid #e2e8f0;
		font-size: 0.85rem; font-weight: 600; color: #1e293b;
	}
	.mem-modal-close {
		background: none; border: none; font-size: 1.1rem; cursor: pointer;
		color: #94a3b8; border-radius: 4px; padding: 2px 6px;
	}
	.mem-modal-close:hover { background: #fee2e2; color: #ef4444; }
	.mem-modal-body { padding: 12px 16px; }
	.mem-modal-empty { text-align: center; color: #94a3b8; padding: 2rem; }

	.loading { text-align: center; padding: 2rem; color: #94a3b8; animation: pulse 1.5s infinite; }
</style>
