<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from '$lib/api';
	import { dateTime, ms } from '$lib/format';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { subscribeSSE } from '$lib/sse';
	import { t } from '$lib/i18n.svelte';

	type Request = {
		id: string;
		rid: string;
		query: string;
		status: string;
		used_provider: string;
		results_count: number;
		total_ms: number;
		client: string;
		created_at: string;
		chain_snapshot?: { mode?: string };
	};

	let items = $state<Request[]>([]);
	let total = $state(0);
	let page = $state(1);
	let limit = 50;
	let status = $state('');
	let provider = $state('');
	let query = $state('');
	let loading = $state(false);
	let unsubscribe: (() => void) | undefined;
	let refreshTimer: ReturnType<typeof setTimeout> | undefined;

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({ page: String(page), limit: String(limit) });
			if (status) params.set('status', status);
			if (provider) params.set('provider', provider);
			if (query) params.set('q', query);
			const data = await get<any>(`/api/requests?${params}`);
			items = data.items;
			total = data.total;
		} finally {
			loading = false;
		}
	}

	function scheduleRefresh() {
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(load, 500);
	}

	onMount(() => {
		load();
		unsubscribe = subscribeSSE('/api/events', (event) => {
			if (event === 'request_created' || event === 'request_done') scheduleRefresh();
		});
		return () => {
			unsubscribe?.();
			clearTimeout(refreshTimer);
		};
	});

	const pages = $derived(Math.max(1, Math.ceil(total / limit)));
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('history.title')}</h1>
		<button class="btn" onclick={load}>{t('common.refresh')}</button>
	</div>

	<div class="card grid grid-cols-1 gap-3 md:grid-cols-4">
		<div>
			<span class="label">{t('common.status')}</span>
			<select class="input" bind:value={status} onchange={() => { page = 1; load(); }}>
				<option value="">{t('common.all')}</option>
				<option value="ok">ok</option>
				<option value="empty">empty</option>
				<option value="fail">fail</option>
				<option value="running">running</option>
			</select>
		</div>
		<div>
			<span class="label">{t('common.provider')}</span>
			<input class="input" bind:value={provider} placeholder={t('history.providerPlaceholder')} />
		</div>
		<div>
			<span class="label">{t('history.queryText')}</span>
			<input class="input" bind:value={query} onkeydown={(e) => e.key === 'Enter' && load()} />
		</div>
		<div class="flex items-end">
			<button class="btn btn-primary w-full" onclick={() => { page = 1; load(); }}>{t('common.apply')}</button>
		</div>
	</div>

	<div class="card">
		<table class="table">
			<thead>
				<tr>
					<th>{t('history.date')}</th><th>{t('history.rid')}</th><th>{t('dashboard.query')}</th><th>{t('common.status')}</th><th>{t('common.provider')}</th>
					<th>{t('history.results')}</th><th>{t('common.time')}</th><th>{t('history.client')}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as request (request.id)}
					<tr>
						<td class="whitespace-nowrap text-xs text-slate-400">{dateTime(request.created_at)}</td>
						<td class="font-mono text-xs">{request.rid}</td>
						<td class="max-w-[260px] truncate">
							<a class="text-emerald-300 hover:underline" href="/history/{request.id}">{request.query}</a>
							{#if request.chain_snapshot?.mode === 'full_chain'}
								<span class="badge ml-1 bg-violet-500/15 text-violet-300" title={t('request.fullChainBadge')}>🧩</span>
							{/if}
						</td>
						<td><StatusBadge status={request.status} /></td>
						<td class="text-xs">{request.used_provider || '—'}</td>
						<td class="text-xs">{request.results_count}</td>
						<td class="text-xs">{ms(request.total_ms)}</td>
						<td class="text-xs">{request.client || '—'}</td>
					</tr>
				{/each}
				{#if items.length === 0 && !loading}
					<tr><td colspan="8" class="py-6 text-center text-slate-500">{t('history.nothingFound')}</td></tr>
				{/if}
			</tbody>
		</table>
		<div class="mt-3 flex items-center justify-between text-xs text-slate-400">
			<span>{t('common.total', { n: total })}</span>
			<div class="flex items-center gap-2">
				<button class="btn px-2 py-0.5" disabled={page <= 1} onclick={() => { page--; load(); }}>←</button>
				<span>{page} / {pages}</span>
				<button class="btn px-2 py-0.5" disabled={page >= pages} onclick={() => { page++; load(); }}>→</button>
			</div>
		</div>
	</div>
</div>
