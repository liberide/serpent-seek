<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from '$lib/api';
	import { dateTime, ms } from '$lib/format';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { subscribeSSE } from '$lib/sse';
	import { t } from '$lib/i18n.svelte';

	type Summary = {
		requests_today: number;
		success_rate: number;
		avg_ms: number;
		errors_today: number;
		total_requests: number;
		daily: { date: string; requests: number; ok: number; empty: number; fail: number }[];
		recent: any[];
	};
	type Provider = { id: string; code: string; name: string; enabled: boolean; credentials_set: Record<string, boolean> };

	let summary = $state<Summary | null>(null);
	let providers = $state<Provider[]>([]);
	let loading = $state(true);
	let unsubscribe: (() => void) | undefined;

	async function load() {
		try {
			const data = await get<any>('/api/stats/summary');
			summary = data.summary;
			providers = data.providers;
		} catch (error) {
			console.error('failed to load dashboard summary', error);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		unsubscribe = subscribeSSE('/api/events', (event) => {
			if (event === 'request_done') load();
		});
		return () => unsubscribe?.();
	});

	const maxDaily = $derived(Math.max(1, ...(summary?.daily ?? []).map((d) => d.requests)));
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold">{t('dashboard.title')}</h1>
		<button class="btn" onclick={load}>{t('common.refresh')}</button>
	</div>

	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		<div class="card">
			<p class="text-xs uppercase tracking-wide text-slate-400">{t('dashboard.requestsToday')}</p>
			<p class="mt-1 text-2xl font-semibold">{summary?.requests_today ?? '—'}</p>
		</div>
		<div class="card">
			<p class="text-xs uppercase tracking-wide text-slate-400">{t('dashboard.success')}</p>
			<p class="mt-1 text-2xl font-semibold text-emerald-300">
				{summary ? summary.success_rate.toFixed(1) + '%' : '—'}
			</p>
		</div>
		<div class="card">
			<p class="text-xs uppercase tracking-wide text-slate-400">{t('dashboard.avgTime')}</p>
			<p class="mt-1 text-2xl font-semibold">{summary ? ms(summary.avg_ms) : '—'}</p>
		</div>
		<div class="card">
			<p class="text-xs uppercase tracking-wide text-slate-400">{t('dashboard.errorsToday')}</p>
			<p class="mt-1 text-2xl font-semibold text-rose-300">{summary?.errors_today ?? '—'}</p>
		</div>
	</div>

	<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
		<div class="card lg:col-span-2">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">
				{t('dashboard.recent')}
			</h2>
			{#if loading}
				<p class="text-sm text-slate-500">{t('common.loading')}</p>
			{:else}
				<table class="table">
					<thead>
						<tr><th>{t('common.time')}</th><th>{t('dashboard.query')}</th><th>{t('common.status')}</th><th>{t('common.provider')}</th><th>{t('common.time')}</th></tr>
					</thead>
					<tbody>
						{#each summary?.recent ?? [] as request (request.id)}
							<tr>
								<td class="whitespace-nowrap text-xs text-slate-400">{dateTime(request.created_at)}</td>
								<td class="max-w-[240px] truncate">
									<a class="text-emerald-300 hover:underline" href="/history/{request.id}">{request.query}</a>
								</td>
								<td><StatusBadge status={request.status} /></td>
								<td class="text-xs">{request.used_provider || '—'}</td>
								<td class="text-xs">{ms(request.total_ms)}</td>
							</tr>
						{/each}
						{#if (summary?.recent ?? []).length === 0}
							<tr><td colspan="5" class="py-4 text-center text-slate-500">{t('dashboard.noRequests')}</td></tr>
						{/if}
					</tbody>
				</table>
			{/if}
		</div>

		<div class="space-y-4">
			<div class="card">
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">{t('dashboard.days7')}</h2>
				<div class="flex h-24 items-end gap-2">
					{#each summary?.daily ?? [] as day (day.date)}
						<div class="flex flex-1 flex-col items-center gap-1">
							<div
								class="w-full rounded-t bg-emerald-600/70"
								style="height: {Math.round((day.requests / maxDaily) * 80)}px"
								title={t('dashboard.requestsTitle', { n: day.requests })}
							></div>
							<span class="text-[10px] text-slate-500">{day.date.slice(5)}</span>
						</div>
					{/each}
				</div>
			</div>
			<div class="card">
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">{t('dashboard.providers')}</h2>
				<ul class="space-y-1 text-sm">
					{#each providers as provider (provider.id)}
						<li class="flex items-center justify-between">
							<span>{provider.name} <span class="text-xs text-slate-500">{provider.code}</span></span>
							<span class="text-xs {provider.enabled ? 'text-emerald-300' : 'text-slate-500'}">
								{provider.enabled ? t('common.enabled') : t('common.disabled')}
							</span>
						</li>
					{/each}
				</ul>
			</div>
		</div>
	</div>
</div>
