<script lang="ts">
	import { t } from '$lib/i18n.svelte';

	type Step = { status?: string; kind?: string; node_key?: string; provider?: string };
	let {
		steps = [],
		done = false,
		unique = null
	}: { steps?: Step[]; done?: boolean; unique?: number | null } = $props();

	const segments = $derived(
		steps.map((step) => {
			switch (step.status) {
				case 'ok':
					return 'bg-emerald-500';
				case 'empty':
					return 'bg-amber-500';
				case 'fail':
					return 'bg-rose-500';
				case 'skip':
					return 'bg-slate-600';
				default:
					return 'bg-amber-400';
			}
		})
	);
	const total = $derived(Math.max(steps.length, 1));
	const label = $derived.by(() => {
		if (!done) return t('progress.running');
		if (steps.some((s) => s.status === 'ok')) return t('progress.ok');
		if (steps.some((s) => s.status === 'fail')) return t('progress.fail');
		return t('progress.empty');
	});
	const tone = $derived(
		steps.some((s) => s.status === 'ok')
			? 'text-emerald-300'
			: steps.some((s) => s.status === 'fail')
				? 'text-rose-300'
				: steps.some((s) => s.status === 'empty')
					? 'text-amber-300'
					: 'text-slate-300'
	);
</script>

<div class="card">
	<div class="mb-2 flex items-center justify-between text-xs">
		<span class="font-semibold uppercase tracking-wide text-slate-400">{t('progress.title')}</span>
		<span class={tone}>{label}</span>
	</div>
	<div class="flex h-3 w-full gap-0.5 overflow-hidden rounded-full bg-slate-800">
		{#each steps as _step, i (i)}
			<div class="h-full flex-1 {segments[i] ?? 'bg-slate-700'}"></div>
		{/each}
		{#if steps.length === 0}
			<div class="h-full w-full bg-slate-800"></div>
		{/if}
	</div>
	<p class="mt-1 text-xs text-slate-500">{t('progress.steps', { n: total })}</p>
	{#if unique !== null}
		<p class="mt-1 font-mono text-xs text-violet-300">🧩 {t('progress.unique', { n: unique })}</p>
	{/if}
</div>
