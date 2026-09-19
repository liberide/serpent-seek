<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { del, get, post } from '$lib/api';
	import { notify } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { t } from '$lib/i18n.svelte';

	type Chain = {
		id: string;
		name: string;
		active: boolean;
		mode?: string;
		version: number;
		nodes: any[];
		edges: any[];
	};
	let chains = $state<Chain[]>([]);
	let loading = $state(true);
	let deleteId = $state<string | null>(null);
	let newChainMode = $state<string>('first_success');

	async function load() {
		loading = true;
		try {
			chains = await get<Chain[]>('/api/chains');
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function createChain() {
		try {
			const providerList = await get<any[]>('/api/providers');
			const first = providerList.find((p) => p.enabled) ?? providerList[0];
			const chain = await post<Chain>('/api/chains', {
				name: t('chains.newName'),
				mode: newChainMode,
				nodes: first
					? [
							{
								key: 'start',
								provider_id: first.id,
								label: first.name ?? first.code,
								params: {},
								timeout_ms: 20000,
								retries: 0,
								retry_delay_ms: 700,
								delay_policy: 'linear',
								on_success: 'stop',
								on_empty: 'next',
								on_fail: 'next',
								is_start: true
							}
						]
					: [],
				edges: []
			});
			await goto(`/chains/${chain.id}`);
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function activate(id: string) {
		try {
			await post(`/api/chains/${id}/activate`);
			notify(t('chains.activated'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function duplicate(id: string) {
		try {
			await post(`/api/chains/${id}/duplicate`);
			notify(t('chains.copied'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function remove() {
		if (!deleteId) return;
		try {
			await del(`/api/chains/${deleteId}`);
			notify(t('chains.deleted'), 'success');
			deleteId = null;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('chains.title')}</h1>
		<div class="flex items-center gap-2">
			<span class="text-[10px] font-semibold uppercase tracking-wide text-slate-500">{t('flow.mode')}</span>
			<div class="inline-flex overflow-hidden rounded-lg border border-slate-700">
				<button
					class="px-3 py-1 text-xs transition {newChainMode === 'first_success'
						? 'bg-emerald-600 text-white'
						: 'bg-slate-800 text-slate-300 hover:bg-slate-700'}"
					onclick={() => (newChainMode = 'first_success')}>⚡ {t('flow.modeFirst')}</button
				>
				<button
					class="px-3 py-1 text-xs transition {newChainMode === 'full_chain'
						? 'bg-violet-600 text-white'
						: 'bg-slate-800 text-slate-300 hover:bg-slate-700'}"
					onclick={() => (newChainMode = 'full_chain')}>🧩 {t('flow.modeFull')}</button
				>
			</div>
			<button class="btn btn-primary" onclick={createChain}>{t('chains.create')}</button>
		</div>
	</div>

	<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
		{#each chains as chain (chain.id)}
			<div class="card">
				<div class="flex items-center justify-between">
					<div>
						<h2 class="font-semibold">
							{chain.name}
							{#if chain.mode === 'full_chain'}
								<span class="badge ml-1 bg-violet-500/15 text-violet-300">🧩 {t('flow.modeFull')}</span>
							{:else}
								<span class="badge ml-1 bg-slate-700/40 text-slate-400">⚡ {t('flow.modeFirst')}</span>
							{/if}
						</h2>
						<p class="text-xs text-slate-500">
							{t('chains.summary', { nodes: chain.nodes?.length ?? 0, edges: chain.edges?.length ?? 0, version: chain.version })}
						</p>
					</div>
					{#if chain.active}<span class="badge bg-emerald-500/15 text-emerald-300">{t('common.active')}</span>{/if}
				</div>
				<div class="mt-3 flex flex-wrap gap-2">
					<a class="btn text-xs" href="/chains/{chain.id}">{t('common.edit')}</a>
					{#if !chain.active}<button class="btn text-xs" onclick={() => activate(chain.id)}>{t('chains.makeActive')}</button>{/if}
					<button class="btn text-xs" onclick={() => duplicate(chain.id)}>{t('chains.duplicate')}</button>
					<button class="btn btn-danger text-xs" onclick={() => (deleteId = chain.id)}>{t('common.delete')}</button>
				</div>
			</div>
		{/each}
		{#if !loading && chains.length === 0}
			<div class="card text-sm text-slate-500">{t('chains.empty')}</div>
		{/if}
	</div>
</div>

<Confirm
	open={deleteId !== null}
	message={t('chains.deleteConfirm')}
	confirmLabel={t('common.delete')}
	onconfirm={remove}
	oncancel={() => (deleteId = null)}
/>
