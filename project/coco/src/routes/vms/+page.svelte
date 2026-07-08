<script lang="ts">
	import { fly, fade, slide } from 'svelte/transition';
	import HexDump from '$lib/components/HexDump.svelte';

	let vms = $state<any[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let msg = $state('');

	let writeResults = $state<Record<string, any[]>>({});
	let writeLoading = $state<Record<string, boolean>>({});
	let customData = $state<Record<string, string>>({});
	let showWriteForm = $state<Record<string, boolean>>({});
	let showMemRegions = $state<Record<string, boolean>>({});

	let memModal = $state<{ vmKey: string; idx: number } | null>(null);
	function openMemModal(vmKey: string, idx: number) { memModal = { vmKey, idx }; }
	function closeMemModal() { memModal = null; }

	async function load() {
		loading = true;
		try { const r = await fetch('/api/vms'); const d = await r.json(); vms = d.vms || []; total = d.total || 0; } catch { msg = '加载失败'; }
		loading = false;
	}

	async function writeAndRead(pid: number) {
		const key = String(pid);
		writeLoading[key] = true; writeLoading = { ...writeLoading };
		const r = await fetch('/api/vms/write-and-read', {
			method: 'POST', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ pid, data: customData[key] || '' })
		});
		const d = await r.json();
		if (d.error) { msg = d.error; writeLoading[key] = false; writeLoading = { ...writeLoading }; return; }
		customData[key] = ''; customData = { ...customData };
		writeResults[key] = [...(writeResults[key] || []), d];
		writeResults = { ...writeResults };
		writeLoading[key] = false; writeLoading = { ...writeLoading };
	}

	async function readMemOnly(pid: number) {
		const key = String(pid);
		writeLoading[key] = true; writeLoading = { ...writeLoading };
		const r = await fetch('/api/vms/read-mem', {
			method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pid })
		});
		const d = await r.json();
		const arr = writeResults[key] || [];
		if (!d.plaintext || d.note?.includes('无数据')) { msg = d.note || '无数据'; writeLoading[key] = false; writeLoading = { ...writeLoading }; return; }
		if (d.entries?.length) {
			const existing = new Set(arr.map((a: any) => a.file_name));
			const newEntries = d.entries.filter((e: any) => !existing.has(e.file_name)).map((e: any) => ({ ...d, plaintext: e.content, file_name: e.file_name, memory_regions: e.memory_regions || d.memory_regions, guest_confirmed: true }));
			if (newEntries.length > 0) { writeResults[key] = [...arr, ...newEntries]; writeResults = { ...writeResults }; }
		} else {
			const last = arr[arr.length - 1];
			if (!last || last.plaintext !== d.plaintext) { writeResults[key] = [...arr, d]; writeResults = { ...writeResults }; }
		}
		writeLoading[key] = false; writeLoading = { ...writeLoading };
	}

	async function deleteProof(pid: number, fileName: string, idx: number) {
		const key = String(pid);
		await fetch('/api/vms/delete-proof', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ pid, file_name: fileName }) });
		const arr = (writeResults[key] || []).filter((_, i) => i !== idx);
		writeResults[key] = arr; writeResults = { ...writeResults };
		if (memModal?.vmKey === key && memModal.idx >= arr.length) closeMemModal();
	}

	function vmTypeLabel(t: string) { return t === 'tdx' ? '🟢 TDX' : t === 'cca' ? '🧬 CCA' : '⚪ 普通'; }
	function sizeFmt(mb: number) { return mb > 1024 ? (mb/1024).toFixed(1)+'GB' : mb+'MB'; }
	function runFmt(s: number) { if (s < 60) return s+'秒'; if (s < 3600) return (s/60).toFixed(0)+'分钟'; if (s < 86400) return (s/3600).toFixed(1)+'小时'; return (s/86400).toFixed(1)+'天'; }

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
	<button onclick={load} class="btn-refresh">🔄 刷新</button>
</div>

{#if msg}<div class="toast" in:fly={{ y: -8, duration: 200 }} out:fade>{msg}</div>{/if}

{#if loading}<div class="loading">加载中...</div>
{:else if vms.length === 0}<div class="empty" in:fade><span>📭</span><p>暂无独立机密虚拟机</p></div>
{:else}
	<div class="list">
		{#each vms as vm, i (vm.pid)}
			{@const key = String(vm.pid)}
			<div class="vm-card" in:fly={{ y: 10, delay: i * 40, duration: 250 }}>
				<div class="vm-card-top">
					<div class="vm-left"><span class="vm-type {vm.vm_type}">{vmTypeLabel(vm.vm_type)}</span></div>
					<div class="vm-main">
						<div class="vm-top-row"><span class="vm-name">{vm.name || '未命名'}</span><span class="vm-pid">PID {vm.pid}</span></div>
						<div class="vm-sub">
							<span class="chip">内存: {sizeFmt(vm.memory_mb || 0)}</span>
							<span class="chip">RSS: {sizeFmt(vm.rss_mb || 0)}</span>
							<span class="chip">运行: {runFmt(vm.running_sec)}</span>
							{#if vm.pod_name}<span class="chip pod-chip">📦 {vm.pod_ns}/{vm.pod_name}</span>{:else}<span class="chip standalone-chip">🔧 独立</span>{/if}
						</div>
					</div>
					<div class="vm-right">
						<button class="act-btn write-btn" onclick={() => { showWriteForm[key] = !showWriteForm[key]; showWriteForm = {...showWriteForm}; }}>📝</button>
					</div>
				</div>
				{#if showWriteForm[key]}
					<div class="write-panel" in:slide={{ duration: 200 }}>
						<div class="write-input-row">
							<input class="write-input" type="text" placeholder="输入数据，留空自动生成" value={customData[key] || ''} oninput={(e) => { customData[key] = e.target.value; customData = {...customData}; }} />
							<button class="write-act-btn" onclick={() => writeAndRead(vm.pid)} disabled={writeLoading[key]}>{writeLoading[key] ? '⏳' : '📝'} 写入</button>
							<button class="write-act-btn read" onclick={() => readMemOnly(vm.pid)} disabled={writeLoading[key]}>🔍 读取</button>
							<button class="write-act-btn close" onclick={() => { showWriteForm[key] = false; showWriteForm = {...showWriteForm}; }}>✕</button>
						</div>
						{#if writeResults[key]?.length}
							{#each writeResults[key] as wr, idx}
								{@const isLast = idx === writeResults[key].length - 1}
								<div class="write-result" class:latest={isLast} in:fade={{ delay: 80 }}>
									<div class="write-result-header">
										<span class="write-idx">#{idx + 1}</span>
										<code>{wr.plaintext}</code>
										{#if wr.guest_confirmed}<span class="write-badge safe">✅ 已写入</span>{/if}
										<span class="write-badge {wr.plaintext_found ? 'found' : 'safe'}">{wr.plaintext_found ? '⚠️ 宿主机可读' : '✅ 加密保护'}</span>
										<div class="write-row-actions">
											<button class="wr-act view" onclick={() => openMemModal(key, idx)}>📄 查看内存</button>
											<button class="wr-act del" onclick={() => deleteProof(vm.pid, wr.file_name || '', idx)}>🗑️</button>
										</div>
									</div>
									<div class="write-note">{wr.note}</div>
									{#if isLast && wr.memory_regions?.length}
										{@const regKey = key + '_' + idx}
										<button class="mem-toggle" onclick={() => { showMemRegions[regKey] = !showMemRegions[regKey]; showMemRegions = {...showMemRegions}; }}>{showMemRegions[regKey] ? '🔼' : '🔽'} 内存区域 (PID: {wr.host_pid}, {wr.memory_regions.length} 个)</button>
										{#if showMemRegions[regKey]}
											<div class="write-regions" in:fade>
												{#each wr.memory_regions.slice(0, 3) as region}
													{@const addrMatch = region.address?.match(/(0x[0-9a-f]+)/i)}
													{@const addr = addrMatch ? parseInt(addrMatch[1], 16) : 0}
													<div class="write-region">
														<div class="wr-addr">{region.address}</div>
														<HexDump hexData={region.hex_dump || ''} asciiSafe={region.ascii_safe || ''} label={region.name} entropy={region.entropy} variant={wr.is_tdx ? 'cipher' : 'plain'} baseAddr={addr} addrLabel="内存地址" entropyLabel={region.entropy > 0 ? 'MKTME 加密密文(非零)' : 'MKTME 加密密文(全零)'} />
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
			</div>
		{/each}
	</div>
{/if}

{#if memModal}
	{@const modalKey = memModal.vmKey}
	{@const modalIdx = memModal.idx}
	{@const modalWR = writeResults[modalKey]?.[modalIdx]}
	<div class="mem-modal-overlay" onclick={closeMemModal} in:fade>
		<div class="mem-modal" onclick={(e) => e.stopPropagation()} in:fly={{ y: 20, duration: 250 }}>
			<div class="mem-modal-header"><span>📄 内存数据 — {modalWR?.plaintext || ''}</span><button class="mem-modal-close" onclick={closeMemModal}>✕</button></div>
			<div class="mem-modal-body">
				{#if modalWR?.is_tdx}
					<div class="mem-modal-tdx">🔒 TDX MKTME 加密密文</div>
					<div class="mem-modal-info">明文: <code>{modalWR.plaintext}</code></div>
					<div class="mem-modal-info">宿主机视角 — MKTME 硬件加密后的密文 (全零):</div>
					{#if modalWR?.memory_regions?.length}
						{#each modalWR.memory_regions.slice(0, 1) as region}
							{@const addrMatch = region.address?.match(/(0x[0-9a-f]+)/i)}
							{@const addr = addrMatch ? parseInt(addrMatch[1], 16) : 0}
							<div class="write-region"><div class="wr-addr">{region.address}</div><HexDump hexData={region.hex_dump || ''} asciiSafe={region.ascii_safe || ''} label={region.name} entropy={region.entropy} variant="cipher" baseAddr={addr} addrLabel="内存地址" entropyLabel={region.entropy > 0 ? "MKTME 加密密文(非零)" : "MKTME 加密密文(全零)"} /></div>
						{/each}
					{/if}
				{:else if modalWR?.plaintext}
					{@const bytes = new TextEncoder().encode(modalWR.plaintext)}
					<div class="mem-modal-info">明文: <code>{modalWR.plaintext}</code> | 长度: {bytes.length} bytes</div>
					<div class="mem-modal-info" style="color:#ef4444">⚠️ 非 TDX VM — 宿主机可直接读取</div>
					<HexDump hexData={Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('')} asciiSafe={Array.from(bytes).map(b => (b >= 32 && b <= 126) ? String.fromCharCode(b) : '.').join('')} label="明文数据" variant="plain" addrLabel="内存地址" />
				{:else}
					<div class="mem-modal-empty">暂无数据</div>
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
	.toolbar { margin-bottom: 1rem; }
	.btn-refresh { padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer; font-size: 0.85rem; background: #e2e8f0; color: #475569; }
	.btn-refresh:hover { background: #cbd5e1; }
	.loading, .empty { text-align: center; padding: 3rem; color: #94a3b8; }
	.empty span { font-size: 3rem; display: block; margin-bottom: 0.5rem; }
	.toast { background: #fef3c7; color: #92400e; padding: 8px 16px; border-radius: 8px; margin-bottom: 0.8rem; font-size: 0.85rem; }
	.list { display: flex; flex-direction: column; gap: 8px; }
	.vm-card { background: #fff; border-radius: 10px; box-shadow: 0 1px 3px rgba(0,0,0,0.06); overflow: hidden; }
	.vm-card-top { display: flex; align-items: center; gap: 12px; padding: 12px 16px; }
	.vm-left { flex-shrink: 0; }
	.vm-type { font-size: 0.75rem; padding: 3px 10px; border-radius: 12px; font-weight: 600; }
	.vm-type.tdx { background: #dcfce7; color: #166534; }
	.vm-type.cca { background: #fae8ff; color: #7c3aed; }
	.vm-type.normal { background: #f1f5f9; color: #64748b; }
	.vm-main { flex: 1; }
	.vm-top-row { display: flex; align-items: center; gap: 8px; margin-bottom: 3px; }
	.vm-name { font-weight: 600; font-size: 0.9rem; }
	.vm-pid { font-size: 0.75rem; color: #94a3b8; font-family: monospace; }
	.vm-sub { display: flex; gap: 6px; flex-wrap: wrap; }
	.chip { font-size: 0.7rem; padding: 2px 8px; background: #f1f5f9; border-radius: 6px; color: #64748b; }
	.chip.pod-chip { background: #dbeafe; color: #1e40af; }
	.chip.standalone-chip { background: #fef3c7; color: #92400e; }
	.vm-right { flex-shrink: 0; }
	.act-btn { padding: 6px 10px; border: 1px solid #e2e8f0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 0.85rem; }
	.act-btn:hover { background: #f8fafc; }
	.write-panel { padding: 0 16px 12px; border-top: 1px solid #f1f5f9; }
	.write-input-row { display: flex; gap: 6px; margin-bottom: 8px; }
	.write-input { flex: 1; padding: 6px 10px; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 0.82rem; }
	.write-act-btn { padding: 6px 12px; border: 1px solid #e2e8f0; border-radius: 6px; background: #fff; cursor: pointer; font-size: 0.78rem; white-space: nowrap; }
	.write-act-btn.read { color: #3b82f6; }
	.write-act-btn.close { color: #ef4444; }
	.write-act-btn:disabled { opacity: 0.5; }
	.write-result { padding: 8px 10px; margin-bottom: 4px; border-radius: 6px; background: #f8fafc; border: 1px solid #f1f5f9; }
	.write-result.latest { background: #eff6ff; border-color: #bfdbfe; }
	.write-result-header { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
	.write-idx { font-size: 0.7rem; color: #94a3b8; font-weight: 600; }
	.write-result-header code { font-size: 0.8rem; background: #e2e8f0; padding: 1px 6px; border-radius: 4px; }
	.write-badge { font-size: 0.68rem; padding: 1px 7px; border-radius: 10px; font-weight: 500; }
	.write-badge.safe { background: #dcfce7; color: #166534; }
	.write-badge.found { background: #fef3c7; color: #92400e; }
	.write-row-actions { margin-left: auto; display: flex; gap: 4px; }
	.wr-act { padding: 3px 8px; border: 1px solid #e2e8f0; border-radius: 4px; background: #fff; cursor: pointer; font-size: 0.7rem; }
	.wr-act.view { color: #3b82f6; }
	.wr-act.del { color: #ef4444; }
	.wr-act:hover { background: #f1f5f9; }
	.write-note { font-size: 0.72rem; color: #64748b; margin-top: 4px; }
	.mem-toggle { background: none; border: 1px solid #e2e8f0; border-radius: 4px; padding: 3px 8px; font-size: 0.72rem; cursor: pointer; color: #64748b; margin-top: 4px; }
	.mem-toggle:hover { background: #f1f5f9; }
	.write-regions { margin-top: 6px; display: flex; flex-direction: column; gap: 6px; }
	.wr-addr { font-size: 0.7rem; color: #64748b; font-family: monospace; margin-bottom: 2px; }
	.mem-modal-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 1000; }
	.mem-modal { background: #fff; border-radius: 12px; width: 90vw; max-width: 800px; max-height: 85vh; overflow-y: auto; box-shadow: 0 20px 60px rgba(0,0,0,0.3); }
	.mem-modal-header { display: flex; justify-content: space-between; align-items: center; padding: 14px 18px; border-bottom: 1px solid #e2e8f0; font-weight: 600; }
	.mem-modal-close { border: none; background: none; font-size: 1.2rem; cursor: pointer; color: #94a3b8; }
	.mem-modal-close:hover { color: #ef4444; }
	.mem-modal-body { padding: 16px 18px; }
	.mem-modal-tdx { background: #dcfce7; color: #166534; padding: 6px 12px; border-radius: 6px; font-weight: 600; font-size: 0.85rem; margin-bottom: 8px; }
	.mem-modal-info { font-size: 0.82rem; color: #475569; margin-bottom: 4px; }
	.mem-modal-info code { background: #e2e8f0; padding: 1px 6px; border-radius: 4px; }
	.mem-modal-empty { text-align: center; color: #94a3b8; padding: 2rem; }
</style>
