<script lang="ts">
	import { onMount } from 'svelte';
	import { fly, fade, slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	let as = $state<any>({});
	let kbs = $state<any>({});
	let rvps = $state<any>({});
	let expanded = $state<Record<string,boolean>>({});

	onMount(async () => {
		const res = await fetch('/api/trustee');
		const d = await res.json();
		as = d.as; kbs = d.kbs; rvps = d.rvps;
	});

	function toggle(key: string) { expanded[key] = !expanded[key]; expanded = {...expanded}; }
</script>

<div class="page-header">
	<h2>🔐 证明链 (Trustee)</h2>
	<p class="sub">机密容器远程证明基础设施 — 确保工作负载在真实 TEE 中运行</p>
</div>

<div class="grid">
	<!-- AS -->
	<div class="card-wrapper" in:fly={{ y: 12, delay: 50, duration: 300 }}>
		<div class="card" on:click={() => toggle('as')} role="button" tabindex="0">
			<div class="card-icon">🏛️</div>
			<div class="card-main">
				<div class="card-title">AS <span class="tag">Attestation Service</span></div>
				<div class="card-desc">{as.description}</div>
				<div class="card-meta">
					<span class="status-dot running"></span>
					<span>{as.status}</span>
					<span class="sep">·</span>
					<span class="endpoint">{as.endpoint}</span>
				</div>
			</div>
			<span class="arrow {expanded['as']?'open':''}">▸</span>
		</div>
		{#if expanded['as']}
			<div class="detail" in:slide={{ duration: 200 }}>
				<div class="detail-title">📋 功能详情</div>
				<ul class="detail-list">
					{#each as.details as d}
						<li in:fade={{ delay: 50 }}>{d}</li>
					{/each}
				</ul>
				<div class="detail-flow">
					<div class="flow-step">容器启动 → 生成 TEE Quote</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">AS 验证 Quote 签名</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">比对 RVPS 参考值</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step success">✅ 签发 Attestation Token</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- KBS -->
	<div class="card-wrapper" in:fly={{ y: 12, delay: 100, duration: 300 }}>
		<div class="card" on:click={() => toggle('kbs')} role="button" tabindex="0">
			<div class="card-icon">🔑</div>
			<div class="card-main">
				<div class="card-title">KBS <span class="tag">Key Broker Service</span></div>
				<div class="card-desc">{kbs.description}</div>
				<div class="card-meta">
					<span class="status-dot running"></span>
					<span>{kbs.status}</span>
					<span class="sep">·</span>
					<span class="endpoint">{kbs.endpoint}</span>
				</div>
			</div>
			<span class="arrow {expanded['kbs']?'open':''}">▸</span>
		</div>
		{#if expanded['kbs']}
			<div class="detail" in:slide={{ duration: 200 }}>
				<div class="detail-title">📋 功能详情</div>
				<ul class="detail-list">
					{#each kbs.details as d}
						<li in:fade={{ delay: 50 }}>{d}</li>
					{/each}
				</ul>
				<div class="detail-flow">
					<div class="flow-step">容器携带 Attestation Token 请求</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">KBS 验证 Token 有效性</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">授权通过 → 释放密钥</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step success">✅ 容器解密镜像 / 获取密钥</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- RVPS -->
	<div class="card-wrapper" in:fly={{ y: 12, delay: 150, duration: 300 }}>
		<div class="card" on:click={() => toggle('rvps')} role="button" tabindex="0">
			<div class="card-icon">📋</div>
			<div class="card-main">
				<div class="card-title">RVPS <span class="tag">Reference Value Provider</span></div>
				<div class="card-desc">{rvps.description}</div>
				<div class="card-meta">
					<span class="status-dot running"></span>
					<span>{rvps.status}</span>
					<span class="sep">·</span>
					<span class="endpoint">{rvps.endpoint}</span>
				</div>
			</div>
			<span class="arrow {expanded['rvps']?'open':''}">▸</span>
		</div>
		{#if expanded['rvps']}
			<div class="detail" in:slide={{ duration: 200 }}>
				<div class="detail-title">📋 功能详情</div>
				<ul class="detail-list">
					{#each rvps.details as d}
						<li in:fade={{ delay: 50 }}>{d}</li>
					{/each}
				</ul>
				<div class="detail-flow">
					<div class="flow-step">管理员预先注册信任策略</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">存储固件/内核预期哈希</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step">AS 查询参考值进行比对</div>
					<div class="flow-arrow">↓</div>
					<div class="flow-step success">✅ 匹配通过 → 信任该 TEE</div>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	h2 { margin: 0; font-size: 1.4rem; }
	.page-header { margin-bottom: 1.2rem; }
	.sub { color: #64748b; font-size: 0.85rem; margin: 0.3rem 0 0; }
	.grid { display: flex; flex-direction: column; gap: 8px; }
	.card-wrapper { }
	.card {
		display: flex; align-items: flex-start; gap: 14px;
		background: #fff; border: 1px solid #e8ecf1; border-radius: 12px;
		padding: 14px 16px; cursor: pointer; transition: all 0.2s;
	}
	.card:hover { border-color: #cbd5e1; box-shadow: 0 2px 10px rgba(0,0,0,0.05); transform: translateY(-1px); }
	.card-icon { font-size: 1.6rem; min-width: 36px; text-align: center; }
	.card-main { flex: 1; }
	.card-title { font-weight: 700; font-size: 0.95rem; color: #1e293b; margin-bottom: 3px; }
	.tag { font-weight: 400; font-size: 0.72rem; color: #94a3b8; margin-left: 4px; }
	.card-desc { font-size: 0.8rem; color: #64748b; margin-bottom: 6px; line-height: 1.4; }
	.card-meta { display: flex; align-items: center; gap: 6px; font-size: 0.78rem; color: #475569; }
	.status-dot { width: 7px; height: 7px; border-radius: 50%; display: inline-block; }
	.status-dot.running { background: #4caf50; animation: pulse 2s infinite; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
	.sep { color: #cbd5e1; }
	.endpoint { font-family: monospace; font-size: 0.75rem; color: #94a3b8; }
	.arrow { color: #94a3b8; font-size: 0.9rem; margin-top: 4px; display: inline-block; transition: transform 0.2s; }
	.arrow.open { transform: rotate(90deg); }

	.detail {
		background: #f8fafc; border: 1px solid #e2e8f0; border-top: none;
		border-radius: 0 0 12px 12px; padding: 1rem 1.4rem;
	}
	.detail-title { font-size: 0.8rem; font-weight: 600; color: #475569; margin-bottom: 8px; }
	.detail-list { margin: 0; padding: 0 0 0 1.2rem; }
	.detail-list li { font-size: 0.8rem; color: #475569; padding: 2px 0; }
	.detail-flow { margin-top: 12px; padding: 10px 14px; background: #fff; border-radius: 8px; border: 1px solid #e2e8f0; }
	.flow-step { font-size: 0.78rem; color: #475569; padding: 3px 0; }
	.flow-step.success { color: #2e7d32; font-weight: 600; }
	.flow-arrow { text-align: center; color: #94a3b8; font-size: 0.7rem; padding: 1px 0; }
</style>
