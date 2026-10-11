<script lang="ts">
	import { onMount } from 'svelte';
	import { get, post } from '$lib/api';
	import { notify } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { t } from '$lib/i18n.svelte';

	type LogEntry = { id: number; ts: string; level: string; rid: string; api: string; message: string };

	let items = $state<LogEntry[]>([]);
	let total = $state(0);
	let limit = 100;
	let level = $state('');
	let query = $state('');
	let confirmClear = $state(false);
	let cursors = $state<string[]>(['']);
	let nextCursor = $state<string | null>(null);

	const page = $derived(cursors.length);
	const currentCursor = $derived(cursors[cursors.length - 1]);

	async function load(cursor: string = currentCursor) {
		const params = new URLSearchParams({ limit: String(limit) });
		if (cursor) params.set('cursor', cursor);
		if (level) params.set('level', level);
		if (query) params.set('q', query);
		const data = await get<any>(`/api/logs?${params}`);
		items = data.items;
		total = data.total;
		nextCursor = data.next_cursor ?? null;
	}

	function applyFilters() {
		cursors = [''];
		load('');
	}

	function nextPage() {
		if (!nextCursor) return;
		cursors = [...cursors, nextCursor];
		load(nextCursor);
	}

	function prevPage() {
		if (cursors.length <= 1) return;
		cursors = cursors.slice(0, -1);
		load(cursors[cursors.length - 1]);
	}

	onMount(() => load());

	async function clearLogs() {
		try {
			const result = await post<any>('/api/maintenance/clear-logs');
			notify(t('logs.deleted', { n: result.deleted }), 'success');
			confirmClear = false;
			cursors = [''];
			await load('');
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
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('logs.title')}</h1>
		<div class="flex gap-2">
			<button class="btn" onclick={() => load()}>{t('common.refresh')}</button>
			<button class="btn btn-danger" onclick={() => (confirmClear = true)}>{t('logs.clear')}</button>
		</div>
	</div>

	<div class="card grid grid-cols-1 gap-3 md:grid-cols-3">
		<div>
			<span class="label">{t('logs.level')}</span>
			<select class="input" bind:value={level} onchange={applyFilters}>
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
			<button class="btn btn-primary w-full" onclick={applyFilters}>{t('common.apply')}</button>
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
				<button class="btn px-2 py-0.5" disabled={page <= 1} onclick={prevPage}>←</button>
				<span>{page}</span>
				<button class="btn px-2 py-0.5" disabled={!nextCursor} onclick={nextPage}>→</button>
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
