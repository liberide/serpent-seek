<script lang="ts">
	import StatusBadge from './StatusBadge.svelte';
	import { dateTime, ms } from '$lib/format';
	import { providerReason } from '$lib/reason';
	import { t } from '$lib/i18n.svelte';

	type Step = {
		idx: number;
		attempt_no?: number;
		node_key?: string;
		provider: string;
		engine?: string;
		status: string;
		kind?: string;
		stage?: string;
		http_status?: number;
		error?: string;
		permanent?: boolean;
		results_count?: number;
		took_ms?: number;
		started_at?: string;
		finished_at?: string;
	};

	let { steps = [], running = false }: { steps?: Step[]; running?: boolean } = $props();

	let now = $state(Date.now());
	$effect(() => {
		if (!running) return;
		const timer = setInterval(() => (now = Date.now()), 100);
		return () => clearInterval(timer);
	});

	function elapsed(step: Step): number {
		if (step.status !== 'running' || !step.started_at) return step.took_ms ?? 0;
		const start = new Date(step.started_at).getTime();
		return Math.max(0, now - start);
	}
</script>

<div class="card">
	<h3 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">{t('timeline.title')}</h3>
	{#if steps.length === 0}
		<p class="text-sm text-slate-500">{t('timeline.empty')}</p>
	{/if}
	<ol class="space-y-2">
		{#each steps as step, i (i)}
			<li class="rounded-lg border border-slate-800 bg-slate-900/40 p-3 text-sm">
				<div class="flex flex-wrap items-center gap-2">
					<span class="font-mono text-xs text-slate-500">#{step.idx}</span>
					<span class="font-semibold">{step.provider}</span>
					{#if step.node_key}<span class="text-xs text-slate-500">({step.node_key})</span>{/if}
					<StatusBadge status={step.status} />
					{#if step.kind}<span class="badge bg-slate-700/40 text-slate-300">{step.kind}</span>{/if}
					{#if step.stage}<span class="badge bg-violet-500/15 text-violet-300">{step.stage}</span>{/if}
					{#if step.http_status}<span class="text-xs text-slate-400">HTTP {step.http_status}</span>{/if}
					{#if step.status === 'ok' && (step.results_count ?? 0) > 0}
						<span class="badge bg-emerald-500/15 font-mono text-emerald-300">
							{t('timeline.addedLinks', { n: step.results_count ?? 0 })}
						</span>
					{/if}
					<span class="ml-auto font-mono text-xs text-slate-300">{ms(elapsed(step))}</span>
				</div>
				<div class="mt-1 flex flex-wrap gap-3 text-xs text-slate-500">
					{#if step.attempt_no}<span>{t('timeline.attempt', { n: step.attempt_no })}</span>{/if}
					<span>{t('timeline.results', { n: step.results_count ?? 0 })}</span>
					{#if step.permanent}<span class="text-rose-400">permanent</span>{/if}
					{#if step.started_at}<span>{dateTime(step.started_at)}</span>{/if}
				</div>
				{#if step.error}
					<p class="mt-1 break-all rounded bg-rose-950/40 p-2 font-mono text-xs text-rose-200">{providerReason(step.error)}</p>
				{/if}
			</li>
		{/each}
	</ol>
</div>
