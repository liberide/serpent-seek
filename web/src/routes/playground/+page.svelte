<script lang="ts">
	import { get, post } from '$lib/api';
	import { notify } from '$lib/stores';
	import ChainGraph from '$lib/components/ChainGraph.svelte';
	import StepTimeline from '$lib/components/StepTimeline.svelte';
	import ProgressCube from '$lib/components/ProgressCube.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { subscribeSSE } from '$lib/sse';
	import { ms } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	let query = $state('');
	let count = $state(5);
	let requestId = $state('');
	let request = $state<any>(null);
	let steps = $state<any[]>([]);
	let running = $state(false);
	let results = $state<any[]>([]);
	let liveUnique = $state<number | null>(null);
	let tip = $state<{ x: number; y: number } | null>(null);
	let unsubscribe: (() => void) | undefined;
	let reloadTimer: ReturnType<typeof setTimeout> | undefined;

	async function refresh() {
		if (!requestId) return;
		const data = await get<any>(`/api/requests/${requestId}`);
		request = data;
		steps = data.steps ?? [];
		running = data.status === 'running';
		if (!running) results = [];
	}

	function scheduleRefresh() {
		clearTimeout(reloadTimer);
		reloadTimer = setTimeout(refresh, 300);
	}

	async function run() {
		if (!query.trim()) return;
		unsubscribe?.();
		running = true;
		liveUnique = null;
		try {
			const started = await post<any>('/api/search/ui', { query, count });
			requestId = started.id;
			await refresh();
			unsubscribe = subscribeSSE(`/api/requests/${requestId}/events`, (event, data) => {
				if (event === 'merge_progress') {
					liveUnique = (data as any)?.unique ?? liveUnique;
				} else if (event === 'request_done') {
					running = false;
					scheduleRefresh();
				} else if (event === 'step_finished' || event === 'provider_dead' || event === 'merge_done') {
					scheduleRefresh();
				}
			});
		} catch (error) {
			running = false;
			notify((error as Error).message, 'error');
		}
	}

	const fullChain = $derived(request?.chain_snapshot?.mode === 'full_chain');

	// The block where the answer finalized = the node of the last recorded step.
	const lastStepNode = $derived.by(() => {
		if (!request || request.status === 'running') return null;
		for (let i = steps.length - 1; i >= 0; i--) {
			if (steps[i].node_key) return steps[i].node_key;
		}
		return null;
	});

	const nodeStates = $derived.by(() => {
		const states: Record<string, string> = {};
		const rank: Record<string, number> = { skip: 1, empty: 2, ok: 3, fail: 4 };
		for (const step of steps) {
			if (!step.node_key) continue;
			if (!states[step.node_key] || (rank[step.status] ?? 0) >= (rank[states[step.node_key]] ?? 0)) {
				states[step.node_key] = step.status;
			}
		}
		return states;
	});

	// Per-node stats for the live graph: response-time badge and the "+n links"
	// contribution of each ok block in full-chain mode (raw while running,
	// final-answer contribution once merged).
	const nodeStats = $derived.by(() => {
		const out: Record<string, { took_ms?: number; added?: number; added_final?: boolean; error?: string }> = {};
		for (const step of steps) {
			if (!step.node_key) continue;
			const cur = (out[step.node_key] ??= {});
			if (step.status === 'ok') {
				cur.took_ms = step.took_ms;
				if (fullChain) cur.added = (cur.added ?? 0) + (step.results_count ?? 0);
			}
			if (step.status === 'fail' && step.error) cur.error = step.error;
		}
		if (fullChain && !running && request?.results) {
			const providerByNode: Record<string, string> = {};
			for (const step of steps) {
				if (step.node_key && !providerByNode[step.node_key]) providerByNode[step.node_key] = step.provider;
			}
			const byProvider: Record<string, number> = {};
			for (const row of request.results) {
				const first = row.sources?.[0];
				if (first) byProvider[first] = (byProvider[first] ?? 0) + 1;
			}
			for (const [key, provider] of Object.entries(providerByNode)) {
				if (out[key]) {
					out[key].added = byProvider[provider] ?? 0;
					out[key].added_final = true;
				}
			}
		}
		return out;
	});
</script>

<div class="space-y-4">
	<h1 class="text-xl font-semibold">{t('playground.title')}</h1>

	<div class="card">
		<form
			class="flex flex-wrap items-end gap-2"
			onsubmit={(event) => {
				event.preventDefault();
				run();
			}}
		>
			<div class="min-w-[240px] flex-1">
				<span class="label">{t('playground.query')}</span>
				<input class="input" bind:value={query} placeholder={t('playground.queryPlaceholder')} />
			</div>
			<div class="w-28">
				<span class="label">{t('playground.count')}</span>
				<input
					type="number"
					min="0"
					max="20"
					class="input font-mono"
					bind:value={count}
					onmousemove={(event) => (tip = { x: event.clientX, y: event.clientY })}
					onmouseleave={() => (tip = null)}
				/>
			</div>
			<button class="btn btn-primary" type="submit" disabled={running}>{t('playground.find')}</button>
			{#if requestId}<a class="btn" href="/history/{requestId}">{t('playground.openHistory')}</a>{/if}
		</form>
	</div>

	{#if request}
		<div class="flex items-center gap-3">
			<StatusBadge status={request.status} />
			{#if fullChain}<span class="badge bg-violet-500/15 text-violet-300">{t('request.fullChainBadge')}</span>{/if}
			<span class="font-mono text-xs text-slate-400">{request.rid}</span>
			<span class="text-xs text-slate-400">{t('playground.provider')} {request.used_provider || '—'}</span>
			<span class="text-xs text-slate-400">{ms(request.total_ms)}</span>
		</div>
		{#if request.answer}
			<div class="card">
				<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-violet-300">
					💬 {t('request.answerTitle')}
				</h2>
				<p class="whitespace-pre-wrap break-words text-sm text-slate-200">{request.answer}</p>
			</div>
		{/if}
		<ProgressCube {steps} done={!running} unique={fullChain ? (liveUnique ?? request.merge?.unique_links ?? null) : null} />
		{#if request.chain_snapshot}
			<div class="card">
					<ChainGraph chain={request.chain_snapshot} states={nodeStates} stats={nodeStats} finish={lastStepNode} />
			</div>
		{/if}
		<StepTimeline {steps} {running} />
		{#if results.length > 0}
			<div class="card">
				<h2 class="mb-2 text-sm font-semibold">{t('playground.results')}</h2>
				<ol class="space-y-2 text-sm">
					{#each results as row (row.link)}
						<li>
							<a class="text-emerald-300 hover:underline" href={row.link} target="_blank" rel="noreferrer">{row.title || row.link}</a>
							<p class="text-xs text-slate-400">{row.snippet}</p>
						</li>
					{/each}
				</ol>
			</div>
		{/if}
	{/if}
</div>

{#if tip}
	<div
		class="pointer-events-none fixed z-50 max-w-[260px] rounded-md border border-slate-700 bg-slate-800 px-2 py-1 text-[11px] text-slate-200 shadow-lg"
		style="left: {tip.x + 14}px; top: {tip.y + 14}px;"
	>
		{t('playground.countAll')}
	</div>
{/if}
