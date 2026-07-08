<script lang="ts">
	import { fly } from 'svelte/transition';
	import { page } from '$app/stores';
	let { children } = $props();
	let currentPath = $state('');
	$effect(() => { currentPath = $page.url.pathname; });
</script>

<div class="app">
	<nav class="sidebar">
		<div class="logo">
			<span class="logo-icon">🔐</span>
			<div>
				<div class="logo-title">CoCo Panel</div>
				<div class="logo-sub">机密容器可视化</div>
			</div>
		</div>
		<a href="/" class="nav-item" class:active={currentPath === '/'}>📊 总览</a>
		<a href="/pods" class="nav-item" class:active={currentPath === '/pods'}>🖥️ 机密容器</a>
		<a href="/vms" class="nav-item" class:active={currentPath === '/vms'}>🖥️ 机密虚拟机</a>
		<!-- 暂时隐藏
		<a href="/runtimes" class="nav-item" class:active={currentPath === '/runtimes'}>🔄 运行时</a>
		<a href="/trustee" class="nav-item" class:active={currentPath === '/trustee'}>🔐 证明链</a>
		-->
		<div class="sidebar-footer">
			<div class="dot"></div>
			<span>TDX · SGX · Kata</span>
		</div>
	</nav>
	<main class="content">
		{#key currentPath}
			<div in:fly={{ y: 12, duration: 250 }}>
				{@render children()}
			</div>
		{/key}
	</main>
</div>

<style>
	:global(body) { margin: 0; font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f0f2f5; color: #1e293b; }
	.app { display: flex; min-height: 100vh; }
	.sidebar {
		width: 220px; background: linear-gradient(180deg, #0f172a 0%, #1e293b 100%); color: #fff;
		padding: 1.2rem 0; display: flex; flex-direction: column; gap: 2px;
		position: sticky; top: 0; height: 100vh; overflow-y: auto;
	}
	.logo {
		display: flex; align-items: center; gap: 10px;
		padding: 0 1rem 1.2rem; margin-bottom: 0.3rem;
		border-bottom: 1px solid rgba(255,255,255,0.08);
	}
	.logo-icon { font-size: 1.6rem; }
	.logo-title { font-size: 1rem; font-weight: 700; letter-spacing: 0.3px; }
	.logo-sub { font-size: 0.65rem; color: #64748b; margin-top: 1px; }
	.nav-item {
		padding: 10px 1rem; margin: 1px 8px; border-radius: 8px;
		color: #94a3b8; text-decoration: none; font-size: 0.88rem;
		transition: all 0.2s ease; border-left: none;
	}
	.nav-item:hover { color: #e2e8f0; background: rgba(255,255,255,0.06); }
	.nav-item.active { color: #fff; background: rgba(76,175,80,0.15); font-weight: 600; }
	.sidebar-footer {
		margin-top: auto; padding: 1rem;
		display: flex; align-items: center; gap: 8px;
		font-size: 0.72rem; color: #475569; border-top: 1px solid rgba(255,255,255,0.06);
	}
	.dot { width: 6px; height: 6px; border-radius: 50%; background: #4caf50; animation: pulse 2s infinite; }
	@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
	.content { flex: 1; padding: 1.8rem 2rem; max-width: 1100px; }
	@media (max-width: 768px) { .sidebar { width: 60px; } .logo-title, .logo-sub, .sidebar-footer span { display: none; } .logo { justify-content: center; } }
</style>
