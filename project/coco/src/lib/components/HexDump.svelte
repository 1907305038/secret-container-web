<script lang="ts">
	interface Props {
		hexData: string;
		asciiSafe?: string;
		label?: string;
		entropy?: number;
		variant?: 'cipher' | 'plain' | 'unknown';
		note?: string;
		baseAddr?: number;
		addrLabel?: string;      // 地址列标题
		entropyLabel?: string;    // 自定义熵值标签
	}

	let { hexData, asciiSafe = '', label = '', entropy = 0, variant = 'unknown', note = '', baseAddr = 0, addrLabel = '内存地址', entropyLabel = '' }: Props = $props();

	// 每 32 个 hex 字符 (16 字节) 一行
	let rows: { hex: string; ascii: string }[] = $derived.by(() => {
		const r: { hex: string; ascii: string }[] = [];
		for (let i = 0; i < hexData.length; i += 32) {
			const chunk = hexData.slice(i, i + 32);
			const hexPairs: string[] = [];
			for (let j = 0; j < chunk.length; j += 2) {
				hexPairs.push(chunk.slice(j, j + 2).toUpperCase());
			}
			const ascii = asciiSafe.slice(i / 2, i / 2 + 16);
			r.push({ hex: hexPairs.join(' '), ascii });
		}
		return r;
	});

	let variantClass = $derived(`variant-${variant}`);
</script>

<div class="hex-container {variantClass}">
	{#if label}
		<div class="hex-label">{label}</div>
	{/if}
	<div class="hex-body">
		<div class="hex-row hex-header">
			<span class="hex-offset">{addrLabel}</span>
			<span class="hex-values">16进制数据</span>
			<span class="hex-ascii">ASCII</span>
		</div>
		{#each rows as row, idx}
			<div class="hex-row">
				<span class="hex-offset">{(baseAddr + idx * 16).toString(16).padStart(8, '0')}</span>
				<span class="hex-values">{row.hex}</span>
				<span class="hex-ascii">|{row.ascii}|</span>
			</div>
		{/each}
	</div>
	{#if entropy > 0}
		<div class="hex-entropy">
			熵值: <b>{entropy.toFixed(2)}</b>
			<span class="entropy-bar">
				<span class="entropy-fill" style="width: {Math.min(entropy / 8 * 100, 100)}%"></span>
			</span>
			<span class="entropy-label">{entropyLabel || (entropy > 6.5 ? '🔴 高熵(密文特征)' : entropy > 4 ? '🟡 中等' : '🟢 低熵(明文特征)')}</span>
		</div>
	{/if}
	{#if note}
		<div class="hex-note">{note}</div>
	{/if}
</div>

<style>
	.hex-container {
		background: #1e293b;
		border-radius: 8px;
		padding: 10px 12px;
		margin: 6px 0;
		font-family: 'Courier New', monospace;
		overflow-x: auto;
	}
	.variant-cipher {
		border-left: 3px solid #ef4444;
	}
	.variant-plain {
		border-left: 3px solid #4caf50;
	}
	.variant-unknown {
		border-left: 3px solid #64748b;
	}

	.hex-label {
		font-size: 0.72rem;
		color: #94a3b8;
		margin-bottom: 6px;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}
	.hex-body {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}
	.hex-header {
		color: #64748b; font-size: 0.65rem; font-weight: 600;
		border-bottom: 1px solid #334155; padding-bottom: 4px; margin-bottom: 2px;
	}
	.hex-row {
		display: flex;
		gap: 12px;
		font-size: 0.7rem;
		line-height: 1.5;
	}
	.hex-offset {
		color: #475569;
		min-width: 64px;
	}
	.variant-cipher .hex-values {
		color: #fca5a5;
	}
	.variant-plain .hex-values {
		color: #86efac;
	}
	.variant-unknown .hex-values {
		color: #e2e8f0;
	}
	.hex-ascii {
		color: #94a3b8;
	}

	.hex-entropy {
		margin-top: 8px;
		font-size: 0.72rem;
		color: #cbd5e1;
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.entropy-bar {
		display: inline-block;
		width: 80px;
		height: 6px;
		background: #334155;
		border-radius: 3px;
		overflow: hidden;
	}
	.entropy-fill {
		display: block;
		height: 100%;
		background: linear-gradient(90deg, #4caf50, #facc15, #ef4444);
		border-radius: 3px;
		transition: width 0.3s;
	}
	.entropy-label {
		color: #94a3b8;
		font-size: 0.68rem;
	}

	.hex-note {
		margin-top: 8px;
		font-size: 0.76rem;
		font-weight: 600;
		color: #fbbf24;
	}
</style>
