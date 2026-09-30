<script lang="ts">
	import { dayLabel, formatInt, formatPct, ms } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type Point = {
		date: string;
		requests: number;
		ok: number;
		empty: number;
		fail: number;
		avg_ms: number;
	};

	let {
		data = [],
		mode = 'volume',
		height = 300
	}: { data?: Point[]; mode?: 'volume' | 'latency' | 'success'; height?: number } = $props();

	let width = $state(0);
	let hover = $state(-1);

	const uid = `trend-${Math.random().toString(36).slice(2, 9)}`;
	const pad = { top: 16, right: 16, bottom: 32, left: 46 };
	const innerW = $derived(Math.max(10, width - pad.left - pad.right));
	const innerH = $derived(height - pad.top - pad.bottom);
	const count = $derived(data.length);
	const band = $derived(count > 0 ? innerW / count : innerW);

	function niceStep(raw: number): number {
		if (raw <= 1) return 1;
		const exponent = Math.floor(Math.log10(raw));
		const base = Math.pow(10, exponent);
		const fraction = raw / base;
		const step = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 2.5 ? 2.5 : fraction <= 5 ? 5 : 10;
		// Metrics are whole numbers (counts / milliseconds), so keep ticks integer.
		return Math.max(1, Math.ceil(step * base));
	}

	// Volume chart scale (4 intervals, so ticks land on round integers).
	const maxTotal = $derived(Math.max(1, ...data.map((d) => d.requests)));
	const yMaxVolume = $derived(niceStep(maxTotal / 4) * 4);

	// Latency / success line scale.
	const values = $derived(
		data.map((d) => {
			if (mode === 'latency') return d.avg_ms;
			const attempts = d.ok + d.empty + d.fail;
			return attempts > 0 ? (d.ok / attempts) * 100 : 0;
		})
	);
	const lineMax = $derived(mode === 'success' ? 100 : niceStep(Math.max(1, ...values) / 4) * 4);
	const axisMax = $derived(mode === 'volume' ? yMaxVolume : lineMax);

	const ticks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => ({ f, value: axisMax * f })));

	function barX(i: number): number {
		return pad.left + i * band;
	}
	function centerX(i: number): number {
		return pad.left + i * band + band / 2;
	}
	function x(i: number): number {
		if (count <= 1) return pad.left + innerW / 2;
		return pad.left + (i / (count - 1)) * innerW;
	}
	function y(value: number): number {
		return pad.top + innerH - (value / lineMax) * innerH;
	}

	const linePath = $derived(
		values.length
			? values.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
			: ''
	);
	const areaPath = $derived(
		values.length
			? `${linePath} L${x(values.length - 1).toFixed(1)},${(pad.top + innerH).toFixed(1)} L${x(0).toFixed(1)},${(pad.top + innerH).toFixed(1)} Z`
			: ''
	);

	const labelStep = $derived(Math.max(1, Math.ceil(count / Math.max(2, Math.floor(innerW / 58)))));
	const barW = $derived(Math.max(2, Math.min(30, band * 0.62)));

	function segmentY(value: number): number {
		return pad.top + innerH - (value / yMaxVolume) * innerH;
	}

	const hoverPoint = $derived(hover >= 0 ? data[hover] : null);

	const tooltipStyle = $derived.by(() => {
		if (hover < 0 || count === 0) return 'display:none';
		const cx = centerX(hover);
		const left = Math.min(Math.max(cx, 70), Math.max(70, width - 70));
		return `left:${left}px; top:${pad.top + 8}px;`;
	});

	function fmtAxis(value: number): string {
		if (mode === 'latency') return ms(value);
		return formatInt(value);
	}

	function fmtSuccess(value: number): string {
		return `${Math.round(value)}%`;
	}
</script>

<div class="relative w-full" bind:clientWidth={width}>
	{#if width > 0}
		<svg
			width={width}
			height={height}
			viewBox="0 0 {width} {height}"
			class="overflow-visible"
			role="img"
			aria-label={t('analytics.chart.perDay')}
			onmouseleave={() => (hover = -1)}
		>
			<defs>
				<linearGradient id="{uid}-ok" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--status-ok);stop-opacity:1" />
					<stop offset="100%" style="stop-color:var(--status-ok);stop-opacity:0.55" />
				</linearGradient>
				<linearGradient id="{uid}-empty" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--status-empty);stop-opacity:1" />
					<stop offset="100%" style="stop-color:var(--status-empty);stop-opacity:0.55" />
				</linearGradient>
				<linearGradient id="{uid}-fail" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--status-fail);stop-opacity:1" />
					<stop offset="100%" style="stop-color:var(--status-fail);stop-opacity:0.55" />
				</linearGradient>
				<linearGradient id="{uid}-area" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" style="stop-color:var(--color-emerald-400);stop-opacity:0.38" />
					<stop offset="100%" style="stop-color:var(--color-emerald-400);stop-opacity:0.02" />
				</linearGradient>
			</defs>

			<!-- Horizontal grid + axis labels -->
			{#each ticks as tick (tick.f)}
				<line
					x1={pad.left}
					y1={pad.top + innerH - tick.f * innerH}
					x2={pad.left + innerW}
					y2={pad.top + innerH - tick.f * innerH}
					stroke="var(--color-slate-800)"
					stroke-dasharray={tick.f === 0 ? '0' : '3 5'}
					stroke-width="1"
				/>
				<text
					x={pad.left - 8}
					y={pad.top + innerH - tick.f * innerH + 3}
					text-anchor="end"
					font-size="10"
					fill="var(--color-slate-500)"
				>
					{mode === 'success' ? fmtSuccess(tick.value) : fmtAxis(tick.value)}
				</text>
			{/each}

			{#if count === 0}
				<text
					x={width / 2}
					y={height / 2}
					text-anchor="middle"
					font-size="13"
					fill="var(--color-slate-500)"
				>
					{t('analytics.chart.noData')}
				</text>
			{:else if mode === 'volume'}
				{#each data as point, i (point.date)}
					<!-- hover band -->
					<rect
						x={barX(i)}
						y={pad.top}
						width={band}
						height={innerH}
						fill="var(--color-slate-600)"
						opacity={hover === i ? 0.16 : 0}
					/>
					{@const okH = (point.ok / yMaxVolume) * innerH}
					{@const emptyH = (point.empty / yMaxVolume) * innerH}
					{@const failH = (point.fail / yMaxVolume) * innerH}
					{@const bx = barX(i) + (band - barW) / 2}
					<g>
						<rect
							x={bx}
							y={segmentY(point.ok)}
							width={barW}
							height={Math.max(okH, point.ok > 0 ? 1.5 : 0)}
							rx="2"
							fill="url(#{uid}-ok)"
						/>
						<rect
							x={bx}
							y={segmentY(point.ok + point.empty)}
							width={barW}
							height={Math.max(emptyH, point.empty > 0 ? 1.5 : 0)}
							rx="2"
							fill="url(#{uid}-empty)"
						/>
						<rect
							x={bx}
							y={segmentY(point.ok + point.empty + point.fail)}
							width={barW}
							height={Math.max(failH, point.fail > 0 ? 1.5 : 0)}
							rx="2"
							fill="url(#{uid}-fail)"
						/>
					</g>
					<rect
						x={barX(i)}
						y={pad.top}
						width={band}
						height={innerH}
						fill="transparent"
						role="presentation"
						onmouseenter={() => (hover = i)}
					/>
				{/each}
			{:else}
				<path d={areaPath} fill="url(#{uid}-area)" />
				<path
					d={linePath}
					fill="none"
					stroke="var(--color-emerald-400)"
					stroke-width="2.5"
					stroke-linecap="round"
					stroke-linejoin="round"
				/>
				{#each values as value, i (data[i].date)}
					<circle
						cx={x(i)}
						cy={y(value)}
						r={hover === i ? 5 : 3}
						fill="var(--color-slate-900)"
						stroke="var(--color-emerald-400)"
						stroke-width="2"
					/>
					<rect
						x={x(i) - Math.max(band, 8) / 2}
						y={pad.top}
						width={Math.max(band, 8)}
						height={innerH}
						fill="transparent"
						role="presentation"
						onmouseenter={() => (hover = i)}
					/>
				{/each}
			{/if}

			<!-- X labels -->
			{#each data as point, i (point.date)}
				{#if i % labelStep === 0 || i === count - 1}
					<text
						x={centerX(i)}
						y={height - 10}
						text-anchor="middle"
						font-size="10"
						fill="var(--color-slate-500)"
					>
						{dayLabel(point.date)}
					</text>
				{/if}
			{/each}
		</svg>

		{#if hoverPoint}
			<div
				class="pointer-events-none absolute z-10 -translate-x-1/2 rounded-lg border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs shadow-lg backdrop-blur"
				style={tooltipStyle}
			>
				<p class="mb-1 font-semibold text-slate-200">{dayLabel(hoverPoint.date, 'long')}</p>
				{#if mode === 'volume'}
					<p class="text-slate-300">{t('analytics.chart.dayTotal', { n: formatInt(hoverPoint.requests) })}</p>
					<p class="mt-1 text-slate-400">
						{t('analytics.chart.dayBreakdown', {
							ok: formatInt(hoverPoint.ok),
							empty: formatInt(hoverPoint.empty),
							fail: formatInt(hoverPoint.fail),
							ms: ms(hoverPoint.avg_ms)
						})}
					</p>
				{:else if mode === 'latency'}
					<p class="text-slate-300">
						{t('analytics.chart.avgLatency')}: <span class="font-semibold">{ms(hoverPoint.avg_ms)}</span>
					</p>
					<p class="mt-1 text-slate-400">{t('analytics.chart.dayTotal', { n: formatInt(hoverPoint.requests) })}</p>
				{:else}
					<p class="text-slate-300">
						{t('analytics.kpi.successRate')}:
						<span class="font-semibold">
							{formatPct(
								hoverPoint.ok + hoverPoint.empty + hoverPoint.fail > 0
									? (hoverPoint.ok / (hoverPoint.ok + hoverPoint.empty + hoverPoint.fail)) * 100
									: 0
							)}
						</span>
					</p>
					<p class="mt-1 text-slate-400">{t('analytics.chart.dayTotal', { n: formatInt(hoverPoint.requests) })}</p>
				{/if}
			</div>
		{/if}
	{/if}
</div>
