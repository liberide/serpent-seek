<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { get } from '$lib/api';
	import { dateTime, dayLabel, formatCost, formatInt, formatPct, ms } from '$lib/format';
	import { subscribeSSE } from '$lib/sse';
	import { t } from '$lib/i18n.svelte';
	import { ANALYTICS_PRESETS, presetRange } from '$lib/analyticsRange';
	import AnalyticsReportView from '$lib/components/analytics/AnalyticsReportView.svelte';
	import {
		ANONYMOUS_USER_ID,
		type AnalyticsResponse,
		type QuotaStatus,
		type UserStatRow,
		type UsersAnalyticsResponse
	} from '$lib/components/analytics/types';

	const initial = presetRange('thisMonth');
	let from = $state(initial.from);
	let to = $state(initial.to);
	let preset = $state<string>('thisMonth');
	let live = $state(true);

	let rows = $state<UserStatRow[]>([]);
	let totals = $state<UsersAnalyticsResponse['totals'] | null>(null);
	let range = $state<{ from: string; to: string }>(initial);
	let loading = $state(true);
	let error = $state('');
	let updatedAt = $state('');

	// The selected owner: a real user row or the anonymous bucket. The id is
	// mirrored in the URL (?user=…) so a per-user view is linkable.
	let selectedId = $state(page.url.searchParams.get('user') ?? '');
	let selected = $state<UserStatRow | null>(null);
	let detail = $state<AnalyticsResponse | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');

	let unsubscribe: (() => void) | undefined;
	let refreshTimer: ReturnType<typeof setTimeout> | undefined;
	let loadSeq = 0;
	let detailSeq = 0;

	async function load() {
		const seq = ++loadSeq;
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ from, to });
			const data = await get<UsersAnalyticsResponse>(`/api/stats/users?${params}`);
			if (seq !== loadSeq) return;
			rows = data.rows ?? [];
			totals = data.totals;
			range = data.range;
			// Keep the selected row in sync (quota usage, last activity).
			if (selectedId) {
				selected = rows.find((row) => row.user_id === selectedId) ?? selected;
			}
			updatedAt = new Date().toISOString();
		} catch (err) {
			if (seq === loadSeq) error = (err as Error).message;
		} finally {
			if (seq === loadSeq) loading = false;
		}
	}

	async function loadDetail() {
		if (!selectedId) return;
		const seq = ++detailSeq;
		detailLoading = true;
		detailError = '';
		try {
			const params = new URLSearchParams({ from, to, user_id: selectedId });
			const data = await get<AnalyticsResponse>(`/api/stats/analytics?${params}`);
			if (seq !== detailSeq) return;
			detail = data;
		} catch (err) {
			if (seq === detailSeq) detailError = (err as Error).message;
		} finally {
			if (seq === detailSeq) detailLoading = false;
		}
	}

	function scheduleRefresh() {
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(() => {
			load();
			if (selectedId) loadDetail();
		}, 1500);
	}

	function applyPreset(key: string) {
		const next = presetRange(key);
		from = next.from;
		to = next.to;
		preset = key;
		load();
		if (selectedId) loadDetail();
	}

	function applyCustomRange() {
		preset = 'custom';
		load();
		if (selectedId) loadDetail();
	}

	// select opens the per-user view; the id lands in the URL so the view can
	// be shared and survives a reload.
	function select(row: UserStatRow) {
		selectedId = row.user_id;
		selected = row;
		detail = null;
		goto(`/analytics/users?user=${encodeURIComponent(row.user_id)}`, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
		loadDetail();
	}

	function back() {
		selectedId = '';
		selected = null;
		detail = null;
		goto('/analytics/users', { replaceState: true, noScroll: true, keepFocus: true });
	}

	onMount(() => {
		load();
		if (selectedId) {
			// Deep link (?user=…): the row data arrives with the leaderboard.
			detailLoading = true;
			loadDetail();
		}
		unsubscribe = subscribeSSE('/api/events', (event) => {
			if (live && event === 'request_done') scheduleRefresh();
		});
		return () => {
			unsubscribe?.();
			clearTimeout(refreshTimer);
		};
	});

	// --- display helpers ---

	function displayName(row: UserStatRow): string {
		if (row.user_id === ANONYMOUS_USER_ID) return t('analytics.users.anonymous');
		if (row.name) return row.name;
		if (!row.exists) return t('analytics.users.deleted');
		return row.user_id;
	}

	function rowTitle(row: UserStatRow): string {
		return row.user_id === ANONYMOUS_USER_ID
			? t('analytics.users.anonymousHint')
			: row.name
				? row.user_id
				: row.user_id;
	}

	function successRate(row: UserStatRow): number {
		return row.requests > 0 ? (row.ok / row.requests) * 100 : 0;
	}

	function share(row: UserStatRow): number {
		const total = totals?.requests ?? 0;
		return total > 0 ? (row.requests / total) * 100 : 0;
	}

	function periodLabel(p: string): string {
		switch (p) {
			case 'daily':
				return t('users.quotaPeriodDaily');
			case 'weekly':
				return t('users.quotaPeriodWeekly');
			case 'monthly':
				return t('users.quotaPeriodMonthly');
			case 'rolling':
				return t('users.quotaPeriodRolling');
			default:
				return p;
		}
	}

	function periodShort(p: string): string {
		switch (p) {
			case 'daily':
				return t('users.quotaShortDaily');
			case 'weekly':
				return t('users.quotaShortWeekly');
			case 'monthly':
				return t('users.quotaShortMonthly');
			case 'rolling':
				return t('users.quotaShortRolling');
			default:
				return p;
		}
	}

	// quotaLabel renders the allowance badge for the leaderboard table.
	function quotaLabel(quota: QuotaStatus | null | undefined): string {
		if (!quota || !quota.requests) return t('users.quotaUnlimited');
		return `${quota.remaining}/${quota.requests} · ${periodShort(quota.period)}`;
	}

	// quotaTone colours the usage bar: green normally, amber past 80%, red
	// when the allowance is exhausted.
	function quotaTone(quota: QuotaStatus): string {
		const usedPct = (quota.used / quota.requests) * 100;
		if (quota.remaining <= 0) return 'var(--status-fail)';
		if (usedPct >= 80) return 'var(--status-empty)';
		return 'var(--status-ok)';
	}
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-end gap-2">
		{#if updatedAt}
			<span class="hidden text-xs text-slate-500 sm:inline">
				{t('analytics.updatedAt', { time: dateTime(updatedAt) })}
			</span>
		{/if}
		{#if live}
			<span class="flex items-center gap-1 text-xs text-emerald-300">
				<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400"></span>
				{t('analytics.live')}
			</span>
		{/if}
		<button class="btn" onclick={() => { load(); if (selectedId) loadDetail(); }} disabled={loading}>
			{#if loading}
				<span
					class="h-3 w-3 animate-spin rounded-full border-2 border-slate-500 border-t-transparent"
				></span>
			{/if}
			{t('common.refresh')}
		</button>
	</div>

	<!-- Filters -->
	<div class="card space-y-3">
		<div class="flex flex-wrap gap-1.5">
			{#each ANALYTICS_PRESETS as item (item.key)}
				<button
					class="rounded-full border px-3 py-1 text-xs font-medium transition {preset === item.key
						? 'border-emerald-500/60 bg-emerald-500/15 text-emerald-200'
						: 'border-slate-700 bg-slate-800/60 text-slate-300 hover:bg-slate-700'}"
					onclick={() => applyPreset(item.key)}
				>
					{t(item.label)}
				</button>
			{/each}
		</div>
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div class="min-w-0">
				<span class="label">{t('analytics.from')}</span>
				<input type="date" class="input" bind:value={from} max={to} onchange={applyCustomRange} />
			</div>
			<div class="min-w-0">
				<span class="label">{t('analytics.to')}</span>
				<input type="date" class="input" bind:value={to} min={from} onchange={applyCustomRange} />
			</div>
		</div>
		<label class="flex w-fit cursor-pointer items-center gap-2 text-xs text-slate-400">
			<input type="checkbox" class="accent-emerald-500" bind:checked={live} />
			{t('analytics.liveHint')}
		</label>
	</div>

	{#if error}
		<div class="card border-rose-800/70 bg-rose-950/30 text-sm text-rose-200">{error}</div>
	{/if}

	{#if selected}
		<!-- Per-user detail -->
		<div class="space-y-4">
			<div class="card">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div class="flex min-w-0 items-center gap-3">
						<button class="btn px-2.5 py-1 text-xs" onclick={back} title={t('analytics.users.back')}>
							←
						</button>
						<span class="text-3xl">{selected.user_id === ANONYMOUS_USER_ID ? '🕵' : '👤'}</span>
						<div class="min-w-0">
							<h2 class="truncate text-lg font-semibold">{displayName(selected)}</h2>
							<p class="text-xs text-slate-500" title={rowTitle(selected)}>
								{#if selected.user_id === ANONYMOUS_USER_ID}
									{t('analytics.users.anonymousHint')}
								{:else}
									{#if selected.role}<span class="badge mr-1 bg-slate-800 text-slate-300">{selected.role}</span>{/if}
									{#if selected.disabled}
										<span class="badge mr-1 bg-rose-500/15 text-rose-300">{t('users.disabled')}</span>
									{/if}
									{#if !selected.exists}
										<span class="badge mr-1 bg-rose-500/15 text-rose-300">{t('analytics.users.deleted')}</span>
									{/if}
									{#if selected.created_at}
										{t('analytics.users.registered')} {dateTime(selected.created_at)}
									{/if}
								{/if}
							</p>
						</div>
					</div>
					<div class="flex items-center gap-4 text-xs text-slate-400">
						{#if selected.user_id !== ANONYMOUS_USER_ID}
							<span title={t('analytics.users.keys')}>
								🔑 {formatInt(selected.keys_count)}
							</span>
						{/if}
						<span title={t('analytics.users.lastActive')}>
							{selected.last_active ? dateTime(selected.last_active) : t('analytics.users.never')}
						</span>
					</div>
				</div>

				{#if selected.quota && selected.quota.requests}
					{@const quota = selected.quota}
					<div class="mt-4 rounded-lg border border-slate-800 bg-slate-950/60 p-3">
						<div class="mb-2 flex flex-wrap items-center justify-between gap-2">
							<span class="text-xs font-semibold uppercase tracking-wide text-slate-400">
								🚦 {t('users.quota')} · {periodLabel(quota.period)}
								{#if quota.period === 'rolling'}({quota.window_hours}h){/if}
							</span>
							<span class="text-xs text-slate-400">
								{t('users.quotaUsed')} {formatInt(quota.used)} ·
								{t('users.quotaRemaining')} {formatInt(quota.remaining)} ·
								{t('users.quotaLimit')} {formatInt(quota.requests)}
								{#if quota.reset_at}
									· {t('users.quotaResetAt')}: {dateTime(quota.reset_at)}
								{/if}
							</span>
						</div>
						<div class="h-2 overflow-hidden rounded-full bg-slate-800">
							<div
								class="h-full rounded-full transition-all"
								style="width:{Math.min(100, (quota.used / quota.requests) * 100)}%; background:{quotaTone(quota)}"
							></div>
						</div>
					</div>
				{/if}
			</div>

			{#if detailError}
				<div class="card border-rose-800/70 bg-rose-950/30 text-sm text-rose-200">{detailError}</div>
			{/if}

			{#if detailLoading && !detail}
				<p class="py-8 text-center text-sm text-slate-500">{t('common.loading')}</p>
			{:else if detail}
				<p class="text-xs text-slate-500">
					{t('analytics.users.detailHint', {
						name: displayName(selected),
						from: dayLabel(detail.range.from),
						to: dayLabel(detail.range.to)
					})}
				</p>
				<AnalyticsReportView
					report={detail.report}
					providers={detail.providers ?? []}
					range={detail.range}
					providerHint={t('analytics.users.providerHint')}
				/>
			{/if}
		</div>
	{:else}
		<!-- Leaderboard -->
		<div class="grid grid-cols-2 gap-3 md:grid-cols-3">
			<div class="card">
				<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.total')}</p>
				<p class="mt-1 text-2xl font-semibold tabular-nums">{formatInt(totals?.requests)}</p>
				<p class="text-[11px] text-slate-500">
					{t('analytics.summaryPeriod', { from: dayLabel(range.from), to: dayLabel(range.to) })}
				</p>
			</div>
			<div class="card">
				<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.users.activeUsers')}</p>
				<p class="mt-1 text-2xl font-semibold tabular-nums text-emerald-300">
					{formatInt(totals?.active_users)}
				</p>
				<p class="text-[11px] text-slate-500">
					{t('analytics.users.activeUsersHint', { n: formatInt(totals?.requests) })}
				</p>
			</div>
			{#if totals?.cost_configured}
				<div class="card">
					<p class="text-xs uppercase tracking-wide text-slate-400">{t('analytics.kpi.cost')}</p>
					<p class="mt-1 text-2xl font-semibold tabular-nums text-teal-300">
						{formatCost(totals?.cost ?? 0)}
					</p>
					<p class="text-[11px] text-slate-500">{t('analytics.kpi.costHint')}</p>
				</div>
			{/if}
		</div>

		<div class="card overflow-x-auto">
			<table class="table">
				<thead>
					<tr>
						<th>{t('common.name')}</th>
						<th>{t('analytics.kpi.total')}</th>
						<th>{t('analytics.kpi.successRate')}</th>
						<th>{t('analytics.kpi.avgMs')}</th>
						<th>{t('analytics.kpi.fail')}</th>
						<th>{t('analytics.users.lastActive')}</th>
						<th>{t('analytics.users.keys')}</th>
						<th>{t('users.quota')}</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.user_id)}
						<tr
							class="cursor-pointer transition hover:bg-slate-800/40"
							title={t('analytics.users.selectHint')}
							onclick={() => select(row)}
						>
							<td>
								<span class="flex min-w-0 items-center gap-2">
									{#if row.user_id === ANONYMOUS_USER_ID}
										<span>🕵</span>
									{:else}
										<span>👤</span>
									{/if}
									<span class="min-w-0">
										<span class="block truncate font-medium text-slate-200" title={rowTitle(row)}>
											{displayName(row)}
										</span>
										{#if row.role}
											<span class="text-[11px] text-slate-500">
												{row.role}{#if row.disabled} · {t('users.disabled')}{/if}{#if !row.exists} · {t('analytics.users.deleted')}{/if}
											</span>
										{/if}
									</span>
								</span>
							</td>
							<td class="min-w-32">
								<span class="flex items-center gap-2">
									<span class="tabular-nums">{formatInt(row.requests)}</span>
									<span class="h-1.5 w-16 overflow-hidden rounded-full bg-slate-800" title={t('analytics.users.share')}>
										<span
											class="block h-full rounded-full"
											style="width:{share(row)}%; background:linear-gradient(90deg, var(--color-emerald-500), var(--color-emerald-300))"
										></span>
									</span>
								</span>
							</td>
							<td class="tabular-nums text-emerald-300">{formatPct(successRate(row))}</td>
							<td class="tabular-nums">{ms(row.avg_ms)}</td>
							<td class="tabular-nums {row.fail > 0 ? 'text-rose-300' : 'text-slate-500'}">
								{formatInt(row.fail)}
							</td>
							<td class="text-xs">{row.last_active ? dateTime(row.last_active) : t('analytics.users.never')}</td>
							<td class="tabular-nums text-xs">
								{row.user_id === ANONYMOUS_USER_ID ? '—' : formatInt(row.keys_count)}
							</td>
							<td>
								{#if row.user_id === ANONYMOUS_USER_ID}
									<span class="text-slate-500">—</span>
								{:else if row.quota && row.quota.requests}
									<span
										class="badge {row.quota.remaining <= 0
											? 'bg-rose-500/15 text-rose-300'
											: 'bg-emerald-500/15 text-emerald-300'}"
									>
										🚦 {quotaLabel(row.quota)}
									</span>
								{:else}
									<span class="text-slate-500">∞ {t('users.quotaUnlimited')}</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
			{#if !loading && rows.length === 0}
				<p class="py-8 text-center text-sm text-slate-500">{t('analytics.chart.noData')}</p>
			{/if}
		</div>
	{/if}
</div>