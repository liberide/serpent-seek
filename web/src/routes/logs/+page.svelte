<script lang="ts">
	import { onMount } from 'svelte';
	import { get, post } from '$lib/api';
	import { notify } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { t } from '$lib/i18n.svelte';

	type LogEntry = { id: number; ts: string; level: string; rid: string; api: string; message: string };

	let items = $state<LogEntry[]>([]);
	let total = $state(0);
	let page = $state(1);
	let limit = 100;
	let level = $state('');
	let query = $state('');
	let confirmClear = $state(false);

	async function load() {
		const params = new URLSearchParams({ page: String(page), limit: String(limit) });
		if (level) params.set('level', level);
		if (query) params.set('q', query);
		const data = await get<any>(`/api/logs?${params}`);
		items = data.items;
		total = data.total;
	}

	onMount(load);

	async function clearLogs() {
		try {
			const result = await post<any>('/api/maintenance/clear-logs');
			notify(t('logs.deleted', { n: result.deleted }), 'success');
			confirmClear = false;
			page = 1;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	const levelTone: Record<string, string> = {
		info: 'bg-slate-600/30 text-slate-300',
		warn: 'bg-amber-500/15 text-amber-300',
		error: 'bg-rose-500/15 text-rose-300',
		debug: 'bg-slate-700/30 text-slate-400'
	};
	const pages = $derived(Math.max(1, Math.ceil(total / limit)));
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('logs.title')}</h1>
		<div class="flex gap-2">
			<button class="btn" onclick={load}>{t('common.refresh')}</button>
			<button class="btn btn-danger" onclick={() => (confirmClear = true)}>{t('logs.clear')}</button>
		</div>
	</div>

	<div class="card grid grid-cols-1 gap-3 md:grid-cols-3">
		<div>
			<span class="label">{t('logs.level')}</span>
			<select class="input" bind:value={level} onchange={() => { page = 1; load(); }}>
				<option value="">{t('common.all')}</option>
				<option value="debug">debug</option>
				<option value="info">info</option>
				<option value="warn">warn</option>
				<option value="error">error</option>
			</select>
		</div>
		<div>
			<span class="label">{t('logs.search')}</span>
			<input class="input" bind:value={query} onkeydown={(e) => e.key === 'Enter' && load()} />
		</div>
		<div class="flex items-end">
			<button class="btn btn-primary w-full" onclick={() => { page = 1; load(); }}>{t('common.apply')}</button>
		</div>
	</div>

	<div class="card">
		<table class="table">
			<thead><tr><th>{t('common.time')}</th><th>{t('logs.level')}</th><th>{t('history.rid')}</th><th>API</th><th>{t('logs.message')}</th></tr></thead>
			<tbody>
				{#each items as entry (entry.id)}
					<tr>
						<td class="whitespace-nowrap font-mono text-xs text-slate-400">{entry.ts}</td>
						<td><span class="badge {levelTone[entry.level] ?? ''}">{entry.level}</span></td>
						<td class="font-mono text-xs">{entry.rid}</td>
						<td class="text-xs">{entry.api || '—'}</td>
						<td class="max-w-[520px] whitespace-pre-wrap break-all text-xs">{entry.message}</td>
					</tr>
				{/each}
				{#if items.length === 0}
					<tr><td colspan="5" class="py-6 text-center text-slate-500">{t('logs.empty')}</td></tr>
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

<Confirm
	open={confirmClear}
	message={t('logs.clearConfirm')}
	confirmLabel={t('common.delete')}
	onconfirm={clearLogs}
	oncancel={() => (confirmClear = false)}
/>
