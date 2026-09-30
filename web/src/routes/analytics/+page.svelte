<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from '$lib/api';
	import { dateTime, dayLabel, formatInt, formatPct, ms } from '$lib/format';
	import { subscribeSSE } from '$lib/sse';
	import { t } from '$lib/i18n.svelte';
	import TrendChart from '$lib/components/charts/TrendChart.svelte';
	import DonutChart from '$lib/components/charts/DonutChart.svelte';
	import HourHistogram from '$lib/components/charts/HourHistogram.svelte';
	import ProviderBreakdown from '$lib/components/charts/ProviderBreakdown.svelte';

	type DailyStat = {
		date: string;
		requests: number;
		ok: number;
		empty: number;
		fail: number;
		avg_ms: number;
	};
	type HourStat = { hour: number; requests: number; ok: number; empty: number; fail: number };
	type TopQuery = { query: string; count: number; ok: number; fail: number };
	type ProviderStat = {
		provider: string;
		code: string;
		enabled: boolean;
		configured: boolean;
		total: number;
		ok: number;
		empty: number;
		fail: number;
		skip: number;
		avg_ms: number;
		success_rate: number;
	};
	type AnalyticsReport = {
		total_requests: number;
		ok: number;
		empty: number;
		fail: number;
		success_rate: number;
		avg_ms: number;
		p95_ms: number;
		active_days: number;
		busiest_date: string;
		busiest_count: number;
		daily: DailyStat[];
		hourly: HourStat[];
		top_queries: TopQuery[];
	};
	type AnalyticsResponse = {
		report: AnalyticsReport;
		providers: ProviderStat[];
		range: { from: string; to: string };
	};

	type TrendMode = 'volume' | 'latency' | 'success';

	const presets = [
		{ key: 'today', label: 'analytics.presets.today' },
		{ key: 'yesterday', label: 'analytics.presets.yesterday' },
		{ key: 'last7', label: 'analytics.presets.last7' },
		{ key: 'last30', label: 'analytics.presets.last30' },
		{ key: 'thisMonth', label: 'analytics.presets.thisMonth' },
		{ key: 'lastMonth', label: 'analytics.presets.lastMonth' },
		{ key: 'thisYear', label: 'analytics.presets.thisYear' }
	] as const;

	function iso(date: Date): string {
		return date.toISOString().slice(0, 10);
	}
	function addDays(base: Date, days: number): Date {
		const next = new Date(base);
		next.setUTCDate(next.getUTCDate() + days);
		return next;
	}
	function todayUTC(): Date {
		const now = new Date();
		return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
	}
	function presetRange(key: string): { from: string; to: string } {
		const today = todayUTC();
		const year = today.getUTCFullYear();
		const month = today.getUTCMonth();
		switch (key) {
			case 'today':
				return { from: iso(today), to: iso(today) };
			case 'yesterday': {
				const y = addDays(today, -1);
				return { from: iso(y), to: iso(y) };
			}
			case 'last7':
				return { from: iso(addDays(today, -6)), to: iso(today) };
			case 'last30':
				return { from: iso(addDays(today, -29)), to: iso(today) };
			case 'lastMonth': {
				const start = new Date(Date.UTC(year, month - 1, 1));
				const end = new Date(Date.UTC(year, month, 0));
				return { from: iso(start), to: iso(end) };
			}
			case 'thisYear':
				return { from: iso(new Date(Date.UTC(year, 0, 1))), to: iso(today) };
			case 'thisMonth':
			default:
				return { from: iso(new Date(Date.UTC(year, month, 1))), to: iso(today) };
		}
	}

	const initial = presetRange('thisMonth');
	let from = $state(initial.from);
	let to = $state(initial.to);
	let preset = $state<string>('thisMonth');
	let provider = $state('');
	let status = $state('');
	let mode = $state<TrendMode>('volume');
	let live = $state(true);

	let report = $state<AnalyticsReport | null>(null);
	let providers = $state<ProviderStat[]>([]);
	let range = $state<{ from: string; to: string }>({ from: initial.from, to: initial.to });
	let loading = $state(true);
	let error = $state('');
	let updatedAt = $state('');
	let unsubscribe: (() => void) | undefined;
	let refreshTimer: ReturnType<typeof setTimeout> | undefined;
	let loadSeq = 0;

	async function load() {
		const seq = ++loadSeq;
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ from, to });
			if (provider) params.set('provider', provider);
			if (status) params.set('status', status);
			const data = await get<AnalyticsResponse>(`/api/stats/analytics?${params}`);
			if (seq !== loadSeq) return;
			report = data.report;
			providers = data.providers ?? [];
			range = data.range;
			updatedAt = new Date().toISOString();
		} catch (err) {
			if (seq === loadSeq) error = (err as Error).message;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	function scheduleRefresh() {
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(load, 1500);
	}

	function applyPreset(key: string) {
		const next = presetRange(key);
		from = next.from;
		to = next.to;
		preset = key;
		load();
	}

	function reset() {
		provider = '';
		status = '';
		mode = 'volume';
		applyPreset('thisMonth');
	}

	onMount(() => {
		load();
		unsubscribe = subscribeSSE('/api/events', (event) => {
			if (live && event === 'request_done') scheduleRefresh();
		});
		return () => {
			unsubscribe?.();
			clearTimeout(refreshTimer);
		};
	});

	const providerOptions = $derived(
		[...new Set(providers.map((p) => p.provider))].filter(Boolean).sort()
	);
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

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div>
			<h1 class="text-xl font-semibold">{t('analytics.title')}</h1>
			<p class="text-xs text-slate-500">{t('analytics.subtitle')}</p>
		</div>
		<div class="flex items-center gap-2">
			{#if updatedAt}
				<span class="hidden text-xs text-slate-500 sm:inline">
					{t('analytics.updatedAt', { time: dateTime(updatedAt) })}
				</span>
			{/if}
			{#if live}
				<span class="flex items-center gap-1 text-xs text-emerald-300">
					<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400"></span>
					{t('analytics.live')}
				</span>
			{/if}
			<button class="btn" onclick={load} disabled={loading}>
				{#if loading}
					<span
						class="h-3 w-3 animate-spin rounded-full border-2 border-slate-500 border-t-transparent"
					></span>
				{/if}
				{t('common.refresh')}
			</button>
		</div>
	</div>

	<!-- Filters -->
	<div class="card space-y-3">
		<div class="flex flex-wrap gap-1.5">
			{#each presets as item (item.key)}
				<button
					class="rounded-full border px-3 py-1 text-xs font-medium transition {preset === item.key
						? 'border-emerald-500/60 bg-emerald-500/15 text-emerald-200'
						: 'border-slate-700 bg-slate-800/60 text-slate-300 hover:bg-slate-700'}"
					onclick={() => applyPreset(item.key)}
				>
					{t(item.label)}
				</button>
			{/each}
		</div>
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
			<div class="min-w-0">
				<span class="label">{t('analytics.from')}</span>
				<input
					type="date"
					class="input"
					bind:value={from}
					max={to}
					onchange={() => {
						preset = 'custom';
						load();
					}}
				/>
			</div>
			<div class="min-w-0">
				<span class="label">{t('analytics.to')}</span>
				<input
					type="date"
					class="input"
					bind:value={to}
					min={from}
					onchange={() => {
						preset = 'custom';
						load();
					}}
				/>
			</div>
			<div class="min-w-0">
				<span class="label">{t('analytics.provider.title')}</span>
				<select class="input" bind:value={provider} onchange={load}>
					<option value="">{t('analytics.allProviders')}</option>
					{#each providerOptions as name (name)}
						<option value={name}>{name}</option>
					{/each}
				</select>
			</div>
			<div class="min-w-0">
				<span class="label">{t('common.status')}</span>
				<select class="input" bind:value={status} onchange={load}>
					<option value="">{t('analytics.allStatuses')}</option>
					<option value="ok">{t('analytics.status.ok')}</option>
					<option value="empty">{t('analytics.status.empty')}</option>
					<option value="fail">{t('analytics.status.fail')}</option>
					<option value="running">{t('analytics.status.running')}</option>
				</select>
			</div>
			<div class="flex min-w-0 flex-wrap items-end justify-start gap-2 sm:col-span-2 xl:col-span-1">
				<button class="btn btn-primary" onclick={load}>{t('common.apply')}</button>
				<button class="btn" onclick={reset}>{t('analytics.reset')}</button>
			</div>
		</div>
		<label class="flex w-fit cursor-pointer items-center gap-2 text-xs text-slate-400">
			<input type="checkbox" class="accent-emerald-500" bind:checked={live} />
			{t('analytics.liveHint')}
		</label>
	</div>

	{#if error}
		<div class="card border-rose-800/70 bg-rose-950/30 text-sm text-rose-200">{error}</div>
	{/if}

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
			<p class="text-[11px] text-slate-500">{t('analytics.provider.hint')}</p>
		</div>
		{#if loading && !report}
			<p class="py-8 text-center text-sm text-slate-500">{t('common.loading')}</p>
		{:else}
			<ProviderBreakdown {providers} />
		{/if}
	</div>
</div>
