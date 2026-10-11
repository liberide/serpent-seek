<script lang="ts">
	import Modal from './Modal.svelte';
	import { t } from '$lib/i18n.svelte';
	import { dateTime } from '$lib/format';
	import { TZ_OFFSETS, formatTzOffset, type TzOffset } from '$lib/tzOffsets';

	export type QuotaStatus = {
		requests: number;
		period: string;
		window_hours: number;
		tz_offset: number;
		used: number;
		remaining: number;
		reset_at?: string;
		updated_at?: string;
	};

	export type QuotaPayload = {
		requests: number;
		period: string;
		window_hours: number;
		tz_offset: number;
	};

	let {
		open = false,
		userName = '',
		status = null,
		onsave,
		onremove,
		onclose
	}: {
		open?: boolean;
		userName?: string;
		status?: QuotaStatus | null;
		onsave?: (payload: QuotaPayload) => Promise<void>;
		onremove?: () => Promise<void>;
		onclose?: () => void;
	} = $props();

	let requests = $state(0);
	let period = $state('daily');
	let windowHours = $state(24);
	let tzOffset = $state(0);
	// "Unlimited" checkbox: when checked the limit fields collapse and the
	// save payload sends requests: 0 only — any previously entered limit is
	// never sent to the server while the box is ticked.
	let unlimited = $state(false);
	let saving = $state(false);
	let removing = $state(false);
	let error = $state('');

	const hasQuota = $derived(!!status && status.requests > 0);

	// Guard against the toggle race: an unlimited save always wins over any
	// stale field state, because save() reads `unlimited` at send time.
	const effectiveRequests = $derived(unlimited ? 0 : Math.max(0, Number(requests) || 0));

	// The stored offset is normally one of TZ_OFFSETS. When an admin sets a
	// custom value through the API, append it so the select can display it
	// instead of silently resetting it to UTC on save.
	const tzOptions = $derived.by<TzOffset[]>(() => {
		const known = new Set(TZ_OFFSETS.map((o) => o.offset));
		if (!known.has(tzOffset)) {
			return [...TZ_OFFSETS, { offset: tzOffset, label: formatTzOffset(tzOffset) }].sort(
				(a, b) => a.offset - b.offset
			);
		}
		return TZ_OFFSETS;
	});

	// Reset the form from the server status every time the dialog opens.
	// `unlimited` is derived from the status alone: requests > 0 means the
	// checkbox starts unchecked, 0/absent means checked.
	$effect(() => {
		if (open) {
			requests = status?.requests ?? 0;
			period = status?.period || 'daily';
			windowHours = status?.window_hours || 24;
			tzOffset = status?.tz_offset ?? 0;
			unlimited = !status || !status.requests;
			error = '';
		}
	});

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

	async function save() {
		if (saving) return;
		saving = true;
		error = '';
		try {
			if (unlimited) {
				// Send requests: 0 and nothing else meaningful: the server
				// treats 0 as unlimited, so any limit the admin typed before
				// ticking the box must not leak into the stored quota.
				await onsave?.({ requests: 0, period, window_hours: 24, tz_offset: 0 });
			} else {
				await onsave?.({
					requests: Math.max(1, Number(requests) || 1),
					period,
					window_hours: Number(windowHours) || 24,
					tz_offset: Number(tzOffset) || 0
				});
			}
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
		}
	}

	async function remove() {
		if (removing || !hasQuota) return;
		removing = true;
		error = '';
		try {
			await onremove?.();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			removing = false;
		}
	}

	function close() {
		if (saving || removing) return;
		onclose?.();
	}
</script>

<Modal {open} title={t('users.quotaTitle')} {onclose}>
	{#if open}
		<p class="mb-3 text-xs text-slate-400">
			{t('users.quotaFor')} <span class="font-medium text-slate-200">{userName}</span>
		</p>

		{#if hasQuota}
			<div class="mb-4 grid grid-cols-3 gap-2 rounded-lg border border-slate-800 bg-slate-950/60 p-3 text-center">
				<div>
					<div class="text-lg font-semibold">{status!.used}</div>
					<div class="text-[10px] uppercase tracking-wide text-slate-500">{t('users.quotaUsed')}</div>
				</div>
				<div>
					<div class="text-lg font-semibold">{status!.remaining}</div>
					<div class="text-[10px] uppercase tracking-wide text-slate-500">{t('users.quotaRemaining')}</div>
				</div>
				<div>
					<div class="text-lg font-semibold">{status!.requests}</div>
					<div class="text-[10px] uppercase tracking-wide text-slate-500">{t('users.quotaLimit')}</div>
				</div>
			</div>
			{#if status!.reset_at}
				<p class="mb-4 text-xs text-slate-500">
					{t('users.quotaResetAt')}: <span class="text-slate-300">{dateTime(status!.reset_at)}</span>
				</p>
			{/if}
		{/if}

		<div class="space-y-3">
			<!-- Toggle styled like the switches in Settings. The switch mirrors
			     the label: ON (emerald) = "unlimited", which is also the default
			     state for a user without a quota. Turning it OFF reveals the
			     limit fields below. -->
			<div class="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2">
				<span class="text-sm">{t('users.quotaUnlimitedLabel')}</span>
				<button
					type="button"
					role="switch"
					aria-checked={unlimited}
					aria-label={t('users.quotaUnlimitedLabel')}
					class="relative h-6 w-11 shrink-0 rounded-full transition {unlimited
						? 'bg-emerald-600'
						: 'bg-slate-700'}"
					onclick={() => (unlimited = !unlimited)}
				>
					<span
						class="absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all {unlimited
							? 'left-[22px]'
							: 'left-0.5'}"
					></span>
				</button>
			</div>

			<!-- grid-rows 0fr->1fr animates the height so the limit fields
			     collapse/expand smoothly instead of popping in and out. -->
			<div
				class="grid transition-[grid-template-rows] duration-200 ease-in-out {unlimited
					? 'grid-rows-[0fr]'
					: 'grid-rows-[1fr]'}"
			>
				<div class="overflow-hidden">
					<div class="space-y-3 pt-1 {unlimited ? 'pointer-events-none opacity-40' : ''}">
						<div>
							<span class="label">{t('users.quotaRequests')}</span>
							<input
								class="input"
								type="number"
								min="1"
								step="1"
								bind:value={requests}
								disabled={unlimited}
							/>
							<p class="mt-1 text-xs text-slate-500">{t('users.quotaRequestsHint')}</p>
						</div>
						<div>
							<span class="label">{t('users.quotaPeriod')}</span>
							<select class="input" bind:value={period} disabled={unlimited}>
								<option value="daily">{t('users.quotaPeriodDaily')}</option>
								<option value="weekly">{t('users.quotaPeriodWeekly')}</option>
								<option value="monthly">{t('users.quotaPeriodMonthly')}</option>
								<option value="rolling">{t('users.quotaPeriodRolling')}</option>
							</select>
						</div>
						{#if period === 'rolling'}
							<div>
								<span class="label">{t('users.quotaWindowHours')}</span>
								<input
									class="input"
									type="number"
									min="1"
									max="720"
									step="1"
									bind:value={windowHours}
									disabled={unlimited}
								/>
							</div>
						{/if}
						{#if period !== 'rolling'}
							<div>
								<span class="label">{t('users.quotaTzOffset')}</span>
								<select class="input" bind:value={tzOffset} disabled={unlimited}>
									{#each tzOptions as opt (opt.offset)}
										<option value={opt.offset}>{opt.label}</option>
									{/each}
								</select>
								<p class="mt-1 text-xs text-slate-500">{t('users.quotaTzOffsetHint')}</p>
							</div>
						{/if}
					</div>
				</div>
			</div>

			<p class="text-xs text-slate-500">{t('users.quotaHint')}</p>
			{#if error}<p class="text-xs text-rose-300">{error}</p>{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-between gap-2">
			<div>
				{#if hasQuota}
					<button class="btn btn-danger" onclick={remove} disabled={removing || saving}>
						{removing ? t('common.saving') : t('users.quotaRemove')}
					</button>
				{/if}
			</div>
			<div class="flex gap-2">
				<button class="btn" onclick={close} disabled={saving || removing}>{t('common.cancel')}</button>
				<button class="btn btn-primary" onclick={save} disabled={saving || removing}>
					{saving ? t('common.saving') : t('common.save')}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>
