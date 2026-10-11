<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from '$lib/api';
	import { dateTime } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type AuditEntry = {
		id: string;
		ts: string;
		user_id: string;
		user: string;
		via: string;
		ip: string;
		action: string;
		target: string;
		details?: Record<string, string>;
	};

	let items = $state<AuditEntry[]>([]);
	let total = $state(0);
	let page = $state(1);
	const limit = 100;
	let action = $state('');
	let query = $state('');

	async function load() {
		const params = new URLSearchParams({ page: String(page), limit: String(limit) });
		if (action) params.set('action', action);
		if (query) params.set('q', query);
		const data = await get<any>(`/api/audit?${params}`);
		items = data.items;
		total = data.total;
	}

	onMount(load);

	function details(entry: AuditEntry): string {
		return Object.entries(entry.details ?? {})
			.map(([key, value]) => `${key}=${value}`)
			.join(', ');
	}

	const pages = $derived(Math.max(1, Math.ceil(total / limit)));
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('audit.title')}</h1>
		<button class="btn" onclick={load}>{t('common.refresh')}</button>
	</div>

	<div class="card grid grid-cols-1 gap-3 md:grid-cols-3">
		<div>
			<span class="label">{t('audit.action')}</span>
			<input class="input" bind:value={action} placeholder="chain.update" onkeydown={(e) => e.key === 'Enter' && load()} />
		</div>
		<div>
			<span class="label">{t('audit.search')}</span>
			<input class="input" bind:value={query} onkeydown={(e) => e.key === 'Enter' && load()} />
		</div>
		<div class="flex items-end">
			<button class="btn btn-primary w-full" onclick={() => { page = 1; load(); }}>{t('common.apply')}</button>
		</div>
	</div>

	<div class="card overflow-x-auto">
		<table class="table">
			<thead>
				<tr>
					<th>{t('common.time')}</th>
					<th>{t('audit.actor')}</th>
					<th>{t('audit.via')}</th>
					<th>{t('audit.action')}</th>
					<th>{t('audit.target')}</th>
					<th>{t('audit.details')}</th>
					<th>{t('audit.ip')}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as entry (entry.id)}
					<tr>
						<td class="whitespace-nowrap font-mono text-xs text-slate-400">{dateTime(entry.ts)}</td>
						<td class="text-xs">{entry.user || '—'}</td>
						<td class="text-xs text-slate-400">{entry.via || '—'}</td>
						<td class="font-mono text-xs">{entry.action}</td>
						<td class="max-w-[220px] truncate text-xs" title={entry.target}>{entry.target || '—'}</td>
						<td class="max-w-[320px] break-words text-xs text-slate-400">{details(entry) || '—'}</td>
						<td class="font-mono text-xs text-slate-500">{entry.ip || '—'}</td>
					</tr>
				{/each}
				{#if items.length === 0}
					<tr><td colspan="7" class="py-6 text-center text-slate-500">{t('audit.empty')}</td></tr>
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
