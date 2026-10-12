<script lang="ts">
	import { dayLabel, formatCost, formatInt, formatPct, ms } from '$lib/format';
	import { t } from '$lib/i18n.svelte';
	import TrendChart from '$lib/components/charts/TrendChart.svelte';
	import DonutChart from '$lib/components/charts/DonutChart.svelte';
	import HourHistogram from '$lib/components/charts/HourHistogram.svelte';
	import ProviderBreakdown from '$lib/components/charts/ProviderBreakdown.svelte';
	import type {
		AnalyticsProviderStat,
		AnalyticsRange,
		AnalyticsReport
	} from '$lib/components/analytics/types';

	type TrendMode = 'volume' | 'latency' | 'success';

	let {
		report = null,
		providers = [],
		range = { from: '', to: '' },
		providerHint = ''
	}: {
		report?: AnalyticsReport | null;
		providers?: AnalyticsProviderStat[];
		range?: AnalyticsRange;
		providerHint?: string;
	} = $props();

	let mode = $state<TrendMode>('volume');

	const maxTopCount = $derived(Math.max(1, ...(report?.top_queries ?? []).map((q) => q.count)));

	const statusSegments = $derived([
		{ key: 'ok', label: t('analytics.status.ok'), value: report?.ok ?? 0, color: 'var(--status-ok)' },
		{ key: 'empty', label: t('analytics.status.empty'), value: report?.empty ?? 0, color: 'var(--status-empty)' },
		{ key: 'fail', label: t('analytics.status.fail'), value: report?.fail ?? 0, color: 'var(--status-fail)' }
	]);

	const modeOptions = [
		{ key: 'volume' as const, label: 'analytics.chart.volume' },
		{ key: 'latency' as const, label: 'analytics.chart.latency' },
		{ key: 'success' as const, label: 'analytics.chart.success' }
	];

	const trendTitle = $derived(
		mode === 'volume'
			? t('analytics.chart.perDay')
			: mode === 'latency'
				? t('analytics.chart.latencyTrend')
				: t('analytics.chart.successTrend')
	);
</script>

<!-- KPIs -->
<div class="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.total')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums">{formatInt(report?.total_requests)}</p>
		<p class="text-[11px] text-slate-500">
			{t('analytics.summaryPeriod', { from: dayLabel(range.from), to: dayLabel(range.to) })}
		</p>
	</div>
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.successRate')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums text-emerald-300">
			{formatPct(report?.success_rate)}
		</p>
		<p class="text-[11px] text-slate-500">
			{t('analytics.kpi.okOf', { ok: formatInt(report?.ok), total: formatInt(report?.total_requests) })}
		</p>
	</div>
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.avgMs')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums">{ms(report?.avg_ms)}</p>
		<p class="text-[11px] text-slate-500">{t('analytics.kpi.avgHint')}</p>
	</div>
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.p95')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums">{ms(report?.p95_ms)}</p>
		<p class="text-[11px] text-slate-500">{t('analytics.kpi.p95Hint')}</p>
	</div>
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.fail')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums text-rose-300">{formatInt(report?.fail)}</p>
		<p class="text-[11px] text-slate-500">
			{t('analytics.kpi.emptyCount', { n: formatInt(report?.empty) })}
		</p>
	</div>
	<div class="card">
		<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.activeDays')}</p>
		<p class="mt-1 text-2xl font-semibold tabular-nums">{formatInt(report?.active_days)}</p>
		<p class="truncate text-[11px] text-slate-500">
			{#if report?.busiest_date}
				{t('analytics.kpi.busiest', {
					date: dayLabel(report.busiest_date),
					n: formatInt(report.busiest_count)
				})}
			{:else}
				{t('analytics.kpi.noBusiest')}
			{/if}
		</p>
	</div>
	{#if report?.cost_configured}
		<div class="card">
			<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.cost')}</p>
			<p class="mt-1 text-2xl font-semibold tabular-nums text-teal-300">
				{formatCost(report?.cost_total ?? 0)}
			</p>
			<p class="text-[11px] text-slate-500">{t('analytics.kpi.costHint')}</p>
		</div>
	{/if}
</div>

<!-- Main charts -->
<div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
	<div class="card xl:col-span-2">
		<div class="mb-3 flex flex-wrap items-center justify-between gap-2">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{trendTitle}</h2>
			<div class="flex rounded-lg border border-slate-700 bg-slate-900 p-0.5">
				{#each modeOptions as option (option.key)}
					<button
						class="rounded-md px-2.5 py-1 text-xs font-medium transition {mode === option.key
							? 'bg-slate-700 text-emerald-200'
							: 'text-slate-400 hover:text-slate-200'}"
						onclick={() => (mode = option.key)}
					>
						{t(option.label)}
					</button>
				{/each}
			</div>
		</div>
		<TrendChart data={report?.daily ?? []} {mode} height={320} />
		{#if mode === 'volume'}
			<div class="mt-2 flex flex-wrap items-center gap-4 text-xs text-slate-400">
				<span class="flex items-center gap-1.5">
					<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-ok)"></span>
					{t('analytics.status.ok')}
				</span>
				<span class="flex items-center gap-1.5">
					<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-empty)"></span>
					{t('analytics.status.empty')}
				</span>
				<span class="flex items-center gap-1.5">
					<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-fail)"></span>
					{t('analytics.status.fail')}
				</span>
				<span class="ml-auto text-[11px] text-slate-500">{t('analytics.chart.utcHint')}</span>
			</div>
		{:else if mode === 'latency'}
			<p class="mt-2 text-xs text-slate-500">{t('analytics.chart.latencyHint')} · {t('analytics.chart.utcHint')}</p>
		{:else}
			<p class="mt-2 text-xs text-slate-500">{t('analytics.chart.successHint')} · {t('analytics.chart.utcHint')}</p>
		{/if}
	</div>

	<div class="card">
		<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">
			{t('analytics.chart.statusDist')}
		</h2>
		<DonutChart
			segments={statusSegments}
			total={report?.total_requests ?? 0}
			centerValue={formatInt(report?.total_requests)}
			centerLabel={t('analytics.kpi.total')}
		/>
	</div>
</div>

<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
	<div class="card lg:col-span-2">
		<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">
			{t('analytics.chart.perHour')}
		</h2>
		<HourHistogram data={report?.hourly ?? []} />
		<p class="mt-1 text-right text-[11px] text-slate-500">{t('analytics.chart.utcHint')}</p>
	</div>

	<div class="card">
		<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">
			{t('analytics.top.title')}
		</h2>
		{#if (report?.top_queries ?? []).length === 0}
			<p class="py-6 text-center text-sm text-slate-500">{t('analytics.top.empty')}</p>
		{:else}
			<ol class="space-y-2">
				{#each report?.top_queries ?? [] as item, i (item.query)}
					<li class="text-sm">
						<div class="mb-1 flex items-center justify-between gap-3">
							<span class="flex min-w-0 items-center gap-2">
								<span class="w-4 shrink-0 text-right text-xs text-slate-500">{i + 1}</span>
								<span class="truncate text-slate-200" title={item.query}>{item.query}</span>
							</span>
							<span class="shrink-0 tabular-nums text-xs text-slate-400">
								{formatInt(item.count)}
							</span>
						</div>
						<div class="ml-6 h-1.5 overflow-hidden rounded-full bg-slate-800/70">
							<div
								class="h-full rounded-full"
								style="width:{(item.count / maxTopCount) * 100}%; background:linear-gradient(90deg, var(--color-emerald-500), var(--color-emerald-300))"
							></div>
						</div>
						<p class="ml-6 mt-0.5 text-[11px] text-slate-500">
							{t('analytics.top.okFail', { ok: formatInt(item.ok), fail: formatInt(item.fail) })}
						</p>
					</li>
				{/each}
			</ol>
		{/if}
	</div>
</div>

<!-- Provider comparison (always at the bottom) -->
<div class="card">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-2">
		<h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">
			{t('analytics.provider.heading')}
		</h2>
		<p class="text-[11px] text-slate-500">{providerHint}</p>
	</div>
	<ProviderBreakdown {providers} />
</div>