<script lang="ts">
	import { formatInt } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type HourPoint = { hour: number; requests: number; ok: number; empty: number; fail: number };

	let { data = [], height = 150 }: { data?: HourPoint[]; height?: number } = $props();

	let width = $state(0);
	let hover = $state(-1);

	const pad = { top: 12, right: 8, bottom: 22, left: 8 };
	const innerW = $derived(Math.max(10, width - pad.left - pad.right));
	const innerH = $derived(height - pad.top - pad.bottom);
	const max = $derived(Math.max(1, ...data.map((d) => d.requests)));
	const band = $derived(data.length > 0 ? innerW / data.length : innerW);
	const barW = $derived(Math.max(2, Math.min(26, band * 0.7)));

	const peakHour = $derived.by(() => {
		let best = -1;
		let bestValue = 0;
		for (const item of data) {
			if (item.requests > bestValue) {
				bestValue = item.requests;
				best = item.hour;
			}
		}
		return best;
	});

	function barX(hour: number): number {
		return pad.left + hour * band + (band - barW) / 2;
	}
	function centerX(hour: number): number {
		return pad.left + hour * band + band / 2;
	}
	function barH(value: number): number {
		return (value / max) * innerH;
	}
</script>

<div class="relative w-full" bind:clientWidth={width}>
	{#if width > 0}
		<svg width={width} height={height} viewBox="0 0 {width} {height}" role="img" aria-label={t('analytics.chart.perHour')}>
			<defs>
				<linearGradient id="hour-accent" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--color-emerald-400);stop-opacity:0.95" />
					<stop offset="100%" style="stop-color:var(--color-emerald-600);stop-opacity:0.35" />
				</linearGradient>
				<linearGradient id="hour-peak" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--color-emerald-300);stop-opacity:1" />
					<stop offset="100%" style="stop-color:var(--color-emerald-500);stop-opacity:0.5" />
				</linearGradient>
			</defs>
			<line
				x1={pad.left}
				y1={pad.top + innerH}
				x2={pad.left + innerW}
				y2={pad.top + innerH}
				stroke="var(--color-slate-800)"
			/>
			{#each data as point (point.hour)}
				<rect
					x={barX(point.hour)}
					y={pad.top + innerH - barH(point.requests)}
					width={barW}
					height={Math.max(barH(point.requests), point.requests > 0 ? 2 : 0)}
					rx="2"
					fill={point.hour === peakHour && point.requests > 0 ? 'url(#hour-peak)' : 'url(#hour-accent)'}
					opacity={hover === -1 || hover === point.hour ? 1 : 0.45}
				/>
				{#if point.hour % 3 === 0}
					<text
						x={centerX(point.hour)}
						y={height - 6}
						text-anchor="middle"
						font-size="10"
						fill="var(--color-slate-500)"
					>
						{String(point.hour).padStart(2, '0')}
					</text>
				{/if}
				<rect
					x={pad.left + point.hour * band}
					y={pad.top}
					width={band}
					height={innerH}
					fill="transparent"
					role="presentation"
					onmouseenter={() => (hover = point.hour)}
					onmouseleave={() => (hover = -1)}
				/>
			{/each}
		</svg>
		{#if hover >= 0 && data[hover]}
			<div
				class="pointer-events-none absolute top-0 z-10 -translate-x-1/2 rounded-lg border border-slate-700 bg-slate-950/95 px-3 py-1.5 text-xs shadow-lg backdrop-blur"
				style="left:{Math.min(Math.max(centerX(hover), 60), Math.max(60, width - 60))}px"
			>
				<span class="font-semibold text-slate-200">
					{String(data[hover].hour).padStart(2, '0')}:00
				</span>
				<span class="ml-2 text-slate-400">{formatInt(data[hover].requests)}</span>
			</div>
		{/if}
	{/if}
</div>
