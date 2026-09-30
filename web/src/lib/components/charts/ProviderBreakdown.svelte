<script lang="ts">
	import { formatInt, formatPct, ms } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type ProviderStat = {
		provider: string;
		code: string;
		enabled: boolean;
		configured: boolean;
		total: number;
		ok: number;
		empty: number;
		fail: number;
		skip: number;
		avg_ms: number;
		success_rate: number;
	};

	let { providers = [] }: { providers?: ProviderStat[] } = $props();

	const maxTotal = $derived(Math.max(1, ...providers.map((p) => p.total)));

	function scale(p: ProviderStat): number {
		return Math.max(p.total / maxTotal, p.total > 0 ? 0.02 : 0);
	}
	function pctOf(p: ProviderStat, value: number): string {
		return p.total > 0 ? `${(value / p.total) * 100}%` : '0%';
	}
</script>

{#if providers.length === 0}
	<p class="py-8 text-center text-sm text-slate-500">{t('analytics.provider.noData')}</p>
{:else}
	<div class="mb-3 flex flex-wrap items-center gap-4 text-xs text-slate-400">
		<span class="flex items-center gap-1.5">
			<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-ok)"></span>
			{t('analytics.status.ok')}
		</span>
		<span class="flex items-center gap-1.5">
			<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-empty)"></span>
			{t('analytics.status.empty')}
		</span>
		<span class="flex items-center gap-1.5">
			<span class="h-2.5 w-2.5 rounded-full" style="background:var(--status-fail)"></span>
			{t('analytics.status.fail')}
		</span>
		<span class="flex items-center gap-1.5">
			<span class="h-2.5 w-2.5 rounded-full" style="background:var(--color-slate-600)"></span>
			{t('analytics.provider.skip')}
		</span>
	</div>
	<div class="space-y-3">
		{#each providers as provider (provider.provider)}
			<div
				class="grid grid-cols-1 gap-2 rounded-lg px-2 py-2 transition hover:bg-slate-800/30 md:grid-cols-[minmax(140px,1.2fr)_minmax(100px,1.6fr)_minmax(0,1.6fr)] md:items-center md:gap-4"
			>
				<div class="flex min-w-0 items-center gap-2">
					<span
						class="h-2 w-2 shrink-0 rounded-full"
						style="background:{provider.enabled ? 'var(--status-ok)' : 'var(--color-slate-600)'}"
						title={provider.enabled ? t('common.enabled') : t('common.disabled')}
					></span>
					<div class="min-w-0">
						<p class="truncate text-sm font-medium text-slate-200" title={provider.provider}>
							{provider.provider}
						</p>
						{#if provider.code}
							<p class="text-[11px] text-slate-500">
								{provider.code}{#if !provider.configured}
									<span class="ml-1 text-amber-400">· {t('analytics.provider.legacy')}</span>
								{/if}
							</p>
						{/if}
					</div>
				</div>

				<div class="h-3 w-full overflow-hidden rounded-full bg-slate-800/70">
					{#if provider.total > 0}
						<div class="flex h-full items-stretch" style="width:{scale(provider) * 100}%">
							<div
								style="width:{pctOf(provider, provider.ok)}; background:var(--status-ok)"
								title="{t('analytics.provider.ok')}: {formatInt(provider.ok)}"
							></div>
							<div
								style="width:{pctOf(provider, provider.empty)}; background:var(--status-empty)"
								title="{t('analytics.provider.empty')}: {formatInt(provider.empty)}"
							></div>
							<div
								style="width:{pctOf(provider, provider.fail)}; background:var(--status-fail)"
								title="{t('analytics.provider.fail')}: {formatInt(provider.fail)}"
							></div>
							<div
								style="width:{pctOf(provider, provider.skip)}; background:var(--color-slate-600)"
								title="{t('analytics.provider.skip')}: {formatInt(provider.skip)}"
							></div>
						</div>
					{:else}
						<div
							class="h-full w-full opacity-40"
							style="background:repeating-linear-gradient(45deg, transparent, transparent 6px, var(--color-slate-700) 6px, var(--color-slate-700) 7px)"
						></div>
					{/if}
				</div>

				<div class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs tabular-nums">
					<span class="text-slate-300" title={t('analytics.provider.total')}>
						<span class="text-slate-500">{t('analytics.provider.total')}:</span> {formatInt(provider.total)}
					</span>
					<span class="text-emerald-300" title={t('analytics.provider.ok')}>
						✓ {formatInt(provider.ok)}
					</span>
					<span class="text-amber-300" title={t('analytics.provider.empty')}>
						◐ {formatInt(provider.empty)}
					</span>
					<span class="text-rose-300" title={t('analytics.provider.fail')}>
						✕ {formatInt(provider.fail)}
					</span>
					<span class="text-slate-500" title={t('analytics.provider.skip')}>
						⤼ {formatInt(provider.skip)}
					</span>
					<span class="text-slate-300" title={t('analytics.provider.success')}>
						{formatPct(provider.success_rate)}
					</span>
					<span class="text-slate-500" title={t('analytics.provider.avg')}>
						{ms(provider.avg_ms)}
					</span>
				</div>
			</div>
		{/each}
	</div>
{/if}
