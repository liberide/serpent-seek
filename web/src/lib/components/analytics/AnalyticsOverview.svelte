<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from '$lib/api';
	import { dateTime } from '$lib/format';
	import { subscribeSSE } from '$lib/sse';
	import { t } from '$lib/i18n.svelte';
	import { ANALYTICS_PRESETS, presetRange } from '$lib/analyticsRange';
	import AnalyticsReportView from '$lib/components/analytics/AnalyticsReportView.svelte';
	import type {
		AnalyticsProviderStat,
		AnalyticsRange,
		AnalyticsReport,
		AnalyticsResponse
	} from '$lib/components/analytics/types';

	const initial = presetRange('thisMonth');
	let from = $state(initial.from);
	let to = $state(initial.to);
	let preset = $state<string>('thisMonth');
	let provider = $state('');
	let status = $state('');
	let live = $state(true);

	let report = $state<AnalyticsReport | null>(null);
	let providers = $state<AnalyticsProviderStat[]>([]);
	let range = $state<AnalyticsRange>({ from: initial.from, to: initial.to });
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
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-end gap-2">
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

	<!-- Filters -->
	<div class="card space-y-3">
		<div class="flex flex-wrap gap-1.5">
			{#each ANALYTICS_PRESETS as item (item.key)}
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

	{#if loading && !report}
		<p class="py-8 text-center text-sm text-slate-500">{t('common.loading')}</p>
	{:else if report}
		<AnalyticsReportView
			{report}
			{providers}
			{range}
			providerHint={t('analytics.provider.hint')}
		/>
	{/if}
</div>