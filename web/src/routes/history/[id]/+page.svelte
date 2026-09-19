<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { get } from '$lib/api';
	import { dateTime, ms } from '$lib/format';
	import ChainGraph from '$lib/components/ChainGraph.svelte';
	import StepTimeline from '$lib/components/StepTimeline.svelte';
	import ProgressCube from '$lib/components/ProgressCube.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { subscribeSSE } from '$lib/sse';
	import { providerReason } from '$lib/reason';
	import { t } from '$lib/i18n.svelte';

	type Step = {
		idx: number;
		attempt_no: number;
		node_key: string;
		provider: string;
		status: string;
		kind: string;
		stage?: string;
		http_status: number;
		error: string;
		permanent: boolean;
		results_count: number;
		took_ms: number;
		started_at: string;
	};
	type Request = {
		id: string;
		rid: string;
		query: string;
		count: number;
		status: string;
		used_provider: string;
		results_count: number;
		total_ms: number;
		client: string;
		error: string;
		answer?: string;
		created_at: string;
		chain_id?: string;
		chain_snapshot?: { name?: string; version?: number; mode?: string; nodes?: any[]; edges?: any[] };
		merge?: { collected_total: number; unique_links: number; duplicates_removed: number };
		results?: { link: string; title?: string; snippet?: string; sources?: string[] }[];
		steps?: Step[];
	};

	let request = $state<Request | null>(null);
	let steps = $state<Step[]>([]);
	let loading = $state(true);
	let liveUnique = $state<number | null>(null);
	let unsubscribe: (() => void) | undefined;
	let reloadTimer: ReturnType<typeof setTimeout> | undefined;
	const id = $derived(page.params.id);

	async function load() {
		loading = true;
		try {
			const data = await get<Request>(`/api/requests/${id}`);
			request = data;
			steps = data.steps ?? [];
		} finally {
			loading = false;
		}
	}

	function scheduleReload() {
		clearTimeout(reloadTimer);
		reloadTimer = setTimeout(load, 300);
	}

	onMount(() => {
		load();
		return () => {
			unsubscribe?.();
			clearTimeout(reloadTimer);
		};
	});

	$effect(() => {
		unsubscribe?.();
		unsubscribe = undefined;
		if (request?.status === 'running') {
			unsubscribe = subscribeSSE(`/api/requests/${id}/events`, (event, data) => {
				if (event === 'step_finished' || event === 'provider_dead') scheduleReload();
				if (event === 'merge_progress') liveUnique = (data as any)?.unique ?? liveUnique;
				if (event === 'merge_done' || event === 'request_done') load();
			});
		}
	});

	const nodeStates = $derived.by(() => {
		const states: Record<string, string> = {};
		const rank: Record<string, number> = { skip: 1, empty: 2, ok: 3, fail: 4 };
		for (const step of steps) {
			if (!step.node_key) continue;
			const current = states[step.node_key];
			if (!current || (rank[step.status] ?? 0) >= (rank[current] ?? 0)) {
				states[step.node_key] = step.status;
			}
		}
		return states;
	});

	const running = $derived(request?.status === 'running');
	const fullChain = $derived(request?.chain_snapshot?.mode === 'full_chain');

	// The block where the answer finalized = the node of the last recorded step
	// (skip steps included: the walk ended there). Only shown for finished requests.
	const lastStepNode = $derived.by(() => {
		if (!request || request.status === 'running') return null;
		for (let i = steps.length - 1; i >= 0; i--) {
			if (steps[i].node_key) return steps[i].node_key;
		}
		return null;
	});
	const finishLabel = $derived.by(() => {
		if (!lastStepNode) return null;
		const node = request?.chain_snapshot?.nodes?.find((n) => n.key === lastStepNode);
		return node?.label ?? lastStepNode;
	});

	// Per-node stats for the read-only graph: response-time badge, "+n links"
	// and the tooltip error of the last failed step. Live, +n counts the rows a
	// block collected; once finished, it switches to the block's contribution to
	// the final merged answer (links whose sources[0] is this block's provider).
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
	<div class="flex items-center justify-between">
		<a class="btn text-xs" href="/history">{t('request.backHistory')}</a>
		<button class="btn text-xs" onclick={load}>{t('common.refresh')}</button>
	</div>

	{#if loading && !request}
		<p class="text-sm text-slate-500">{t('common.loading')}</p>
	{:else if request}
		<div class="card">
			<div class="flex flex-wrap items-center gap-3">
				<h1 class="text-lg font-semibold">{request.query}</h1>
				<StatusBadge status={request.status} />
				{#if fullChain}<span class="badge bg-violet-500/15 text-violet-300">{t('request.fullChainBadge')}</span>{/if}
				<span class="font-mono text-xs text-slate-400">{request.rid}</span>
			</div>
			<div class="mt-2 flex flex-wrap gap-4 text-xs text-slate-400">
				<span>{t('request.provider')} {request.used_provider || '—'}</span>
				<span>{t('request.results')} {request.results_count}</span>
				<span>{t('request.time')} {ms(request.total_ms)}</span>
				<span>{t('request.client')} {request.client || '—'}</span>
				<span>{dateTime(request.created_at)}</span>
			</div>
			{#if request.chain_snapshot}
				<div class="mt-1 flex flex-wrap gap-4 text-xs text-slate-500">
					<span>
						{t('request.chain')}
						{#if request.chain_id}
							<a class="text-slate-300 underline decoration-slate-600 hover:text-emerald-300" href="/chains/{request.chain_id}">{request.chain_snapshot.name}</a>
						{:else}
							{request.chain_snapshot.name}
						{/if}
						<span class="font-mono"> · v{request.chain_snapshot.version ?? 0}</span>
					</span>
					{#if finishLabel}
						<span>🏁 {t('request.finishedIn')} <span class="font-mono text-slate-300">{finishLabel}</span></span>
					{/if}
				</div>
			{/if}
			{#if request.merge}
				<p class="mt-2 font-mono text-xs text-violet-300">
					🧩 {t('request.mergeSummary', {
						collected: request.merge.collected_total,
						dups: request.merge.duplicates_removed,
						unique: request.merge.unique_links
					})}
					{#if request.merge.collected_total - request.merge.duplicates_removed > request.merge.unique_links}
						<span class="text-slate-400">
							· {t('request.mergeCapped', {
								before: request.merge.collected_total - request.merge.duplicates_removed,
								count: request.count
							})}
						</span>
					{/if}
				</p>
			{/if}
			{#if request.error}
				<p class="mt-2 break-all rounded bg-rose-950/40 p-2 font-mono text-xs text-rose-200">{providerReason(request.error)}</p>
			{/if}
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
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">
					{t('request.chainSnapshot')}
				</h2>
				<ChainGraph chain={request.chain_snapshot} states={nodeStates} stats={nodeStats} finish={lastStepNode} />
			</div>
		{/if}

		{#if (request.results?.length ?? 0) > 0}
			<div class="card">
				<h2 class="mb-2 text-sm font-semibold uppercase tracking-wide text-slate-400">
					{t('request.resultsTitle')}
				</h2>
				<ol class="space-y-2 text-sm">
					{#each request.results ?? [] as row (row.link)}
						<li>
							<a class="text-emerald-300 hover:underline" href={row.link} target="_blank" rel="noreferrer">{row.title || row.link}</a>
							{#if (row.sources?.length ?? 0) > 1}
								<span class="badge ml-1 bg-violet-500/15 font-mono text-violet-300">
									{t('request.multiSource', { n: row.sources?.length ?? 0, names: (row.sources ?? []).join(', ') })}
								</span>
							{:else if (row.sources?.length ?? 0) === 1}
								<span class="badge ml-1 bg-slate-700/40 font-mono text-slate-400">{row.sources?.[0]}</span>
							{/if}
							{#if row.snippet}<p class="text-xs text-slate-400">{row.snippet}</p>{/if}
						</li>
					{/each}
				</ol>
			</div>
		{/if}

		<StepTimeline {steps} {running} />
	{:else}
		<p class="text-sm text-slate-500">{t('request.notFound')}</p>
	{/if}
</div>
