<script lang="ts">
	import { Handle, Position } from '@xyflow/svelte';
	import { providerReason } from '$lib/reason';
	import { t } from '$lib/i18n.svelte';

	// NodeCard — chain block card. Three kinds share the same shell:
	//   start  ▶ logical entry point, fans out to several providers
	//   join   ⧉ merges incoming branches and removes duplicate URLs
	//   provider (default) runs one configured provider instance
	// In the editor it hosts clickable timeout presets; in history/playground it
	// only paints the run state recorded in data.status.
	let {
		data = {},
		selected = false
	}: {
		data?: {
			key?: string;
			kind?: string;
			label?: string;
			provider?: string;
			timeout_ms?: number;
			retries?: number;
			retry_delay_ms?: number;
			status?: string; // gray | running | ok | empty | fail | skip
			took_ms?: number;
			added_links?: number;
			added_final?: boolean;
			error?: string;
			is_start?: boolean;
			is_answer?: boolean;
			start_taken?: boolean;
			start_owner?: string;
			invalid?: boolean;
			finish?: boolean;
			mode?: string;
			branches?: number;
			ignore_count?: boolean;
			node?: { kind?: string; timeout_ms?: number; retries?: number; retry_delay_ms?: number };
			interactive?: boolean;
			onCycleTimeout?: (key: string) => void;
		};
		selected?: boolean;
	} = $props();

	const kind = $derived(data.kind ?? data.node?.kind ?? 'provider');
	const isStart = $derived(kind === 'start');
	const isJoin = $derived(kind === 'join');
	const isProvider = $derived(!isStart && !isJoin);

	// Run-state styling: base border/background per status; the start block
	// always keeps its yellow contour underneath. Logical blocks use their own
	// accent (amber start, violet join) when idle.
	const stateClass = $derived.by(() => {
		switch (data.status) {
			case 'ok':
				return 'border-emerald-500 bg-emerald-500/15';
			case 'empty':
				return 'border-amber-500 bg-amber-500/10';
			case 'fail':
				return 'border-rose-500 bg-rose-500/15';
			case 'skip':
				return 'border-slate-600 opacity-60';
			case 'running':
				return 'border-amber-300 node-running';
			default:
				return 'border-slate-700';
		}
	});

	const timeoutMs = $derived(data.node?.timeout_ms ?? data.timeout_ms ?? 20000);
	const timeoutSec = $derived(Math.round(timeoutMs / 1000));
	const retriesNum = $derived(data.node?.retries ?? data.retries ?? 0);
	const retryDelayMs = $derived(data.node?.retry_delay_ms ?? data.retry_delay_ms ?? 700);

	function cycleTimeout(event: MouseEvent) {
		event.stopPropagation();
		data.onCycleTimeout?.(data.key ?? '');
	}

	const handleColors: Record<string, string> = {
		ok: 'var(--status-ok)',
		empty: 'var(--status-empty)',
		fail: 'var(--status-fail)',
		next: 'var(--status-next)'
	};
</script>

<div
	class="relative rounded-xl border-2 px-3 py-2 text-xs shadow-lg backdrop-blur-md transition-transform {stateClass}"
	class:opacity-70={data.status === 'skip'}
	style:width="220px"
	style:min-height="120px"
	style:background={data.status === 'ok'
		? 'var(--node-ok-bg)'
		: data.status === 'fail'
			? 'var(--node-fail-bg)'
			: data.status === 'empty'
				? 'var(--node-empty-bg)'
				: 'var(--node-bg)'}
	style:border-color={data.status && data.status !== 'gray'
		? undefined
		: isStart
			? 'var(--node-start)'
			: isJoin
				? '#a78bfa'
				: data.is_start
					? 'var(--node-start)'
					: undefined}
	style:box-shadow={isStart || data.is_start
		? '0 0 0 4px var(--node-start-bg)'
		: selected
			? '0 0 0 2px var(--color-emerald-400)'
			: undefined}
	style:transform={selected ? 'translateY(-2px)' : undefined}
	title={data.status === 'fail' && data.error ? providerReason(data.error) : undefined}
>
	{#if !isStart}
		<Handle type="target" position={Position.Left} />
	{/if}

	<!-- header: icon+name, kind/provider chip -->
	<div class="flex items-center gap-1.5">
		<span class="text-slate-400">{isStart ? '▶' : isJoin ? '⧉' : '▣'}</span>
		<span class="min-w-0 flex-1 truncate font-semibold text-slate-100">
			{data.label ?? (isStart ? t('flow.start') : isJoin ? t('flow.join') : (data.provider ?? t('flow.node')))}
		</span>
		{#if isStart}
			<span class="rounded bg-amber-400/20 px-1 py-px text-[9px] font-bold text-amber-300">START</span>
		{:else if isJoin}
			<span class="rounded bg-violet-500/25 px-1 py-px text-[9px] font-bold text-violet-200">MERGE</span>
		{:else if data.provider}
			<span class="rounded bg-slate-700/60 px-1 py-px font-mono text-[9px] text-slate-300">
				⚙{data.provider}
			</span>
		{/if}
		{#if data.is_answer}
			<span class="rounded bg-violet-500/25 px-1 py-px text-[9px] font-bold text-violet-200" title={t('flow.modeAnswer')}>
				💬
			</span>
		{/if}
		{#if data.is_start && !isStart}
			<span class="badge bg-amber-400/20 text-amber-300">▶ {t('flow.start')}</span>
		{/if}
	</div>

	{#if isStart}
		<p class="mt-2 text-[10px] leading-snug text-slate-400">{t('flow.startHint')}</p>
	{:else if isJoin}
		<p class="mt-2 text-[10px] leading-snug text-slate-400">{t('flow.joinHint')}</p>
		<p class="mt-1 font-mono text-[10px] text-violet-300">
			⇥ {data.branches ?? 0} {t('flow.branchesShort')} · ⧉ {t('flow.dedupeShort')}
		</p>
		{#if data.ignore_count}
			<p class="mt-1">
				<span
					class="rounded bg-violet-500/30 px-1.5 py-px text-[9px] font-bold text-violet-100"
					title={t('flow.ignoreCountHint')}>Σ {t('flow.allResultsShort')}</span
				>
			</p>
		{/if}
		{#if data.took_ms}
			<p class="mt-1 font-mono text-[10px] text-violet-200">⏱ {data.took_ms}ms</p>
		{/if}
	{:else}
		<!-- settings line: timeout + retries/delay -->
		<div class="mt-1.5 flex items-center gap-2 font-mono text-[10px] text-slate-400">
			{#if data.interactive}
				<button
					type="button"
					class="rounded px-1 py-px text-slate-300 transition hover:bg-slate-800 hover:text-amber-200"
					title={t('flow.timeoutCycleHint')}
					onclick={cycleTimeout}
				>
					⏱ {timeoutSec}s
				</button>
			{:else}
				<span>⏱ {timeoutSec}s</span>
			{/if}
			<span>
				⟳ ×{retriesNum}<span class="text-slate-600"> · {retryDelayMs}ms</span>
			</span>
		</div>
	{/if}

	<!-- status footer: response-time badge / +n links / finish / validation marker -->
	<div class="mt-1.5 flex h-4 items-center gap-1.5">
		{#if data.finish}
			<span class="rounded bg-emerald-500/25 px-1 text-[9px] font-bold text-emerald-200">
				🏁 {t('flow.finishBadge')}
			</span>
		{/if}
		{#if data.invalid}
			<span class="rounded border border-amber-400/70 bg-amber-400/15 px-1 text-[9px] font-bold text-amber-300">
				⚠ {t('flow.invalidBadge')}
			</span>
		{/if}
		{#if data.mode === 'full_chain' && (isStart || isJoin)}
			<span class="rounded bg-violet-500/20 px-1 text-[9px] font-bold text-violet-300">🧩</span>
		{/if}
		{#if data.status === 'ok' && data.took_ms}
			<span class="rounded bg-emerald-500/20 px-1 font-mono text-[9px] text-emerald-300">{data.took_ms}ms</span>
		{/if}
		{#if (data.added_links ?? 0) > 0}
			<span
				class="rounded bg-emerald-500/20 px-1 font-mono text-[9px] text-emerald-300"
				title={t('flow.addedLinksHint')}
			>
				+{data.added_links} {t('flow.linksShort')}
			</span>
		{:else if data.added_final && data.status === 'ok'}
			<span class="rounded bg-slate-600/40 px-1 font-mono text-[9px] text-slate-400" title={t('flow.addedLinksHint')}>
				+0 {t('flow.linksShort')}
			</span>
		{/if}
		{#if data.status === 'running'}
			<span class="ml-auto font-mono text-[9px] text-amber-300">…</span>
		{/if}
	</div>

	<!-- outcome handles on the right -->
	{#if isStart || isJoin}
		<Handle
			type="source"
			id="next"
			position={Position.Right}
			style="top: 50%; background: {isJoin ? '#a78bfa' : 'var(--node-start)'}; width: 10px; height: 10px;"
			title="next"
		/>
	{:else}
		{#each ['ok', 'empty', 'fail', 'next'] as kind, i (kind)}
			<Handle
				type="source"
				id={kind}
				position={Position.Right}
				style="top: {34 + i * 18}px; background: {handleColors[kind]}; width: 8px; height: 8px;"
				title={kind}
			/>
		{/each}
	{/if}
</div>

<style>
	.node-running {
		animation: node-pulse 1.2s ease-in-out infinite;
	}
	@keyframes node-pulse {
		0%,
		100% {
			box-shadow: 0 0 0 2px color-mix(in oklab, var(--status-running) 35%, transparent);
		}
		50% {
			box-shadow: 0 0 0 6px color-mix(in oklab, var(--status-running) 12%, transparent);
		}
	}
</style>
