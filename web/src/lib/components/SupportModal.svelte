<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import { notify, supportOpen } from '$lib/stores';
	import { THEMES, applyTheme, theme, type ThemeId } from '$lib/theme';
	import { wallets, type Wallet } from '$lib/wallets';
	import { renderSVG } from 'uqr';
	import Modal from './Modal.svelte';

	let copiedId = $state<string | null>(null);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	// Wallet whose QR code is currently shown (null = no QR overlay).
	let qrWallet = $state<Wallet | null>(null);

	// Theme selection is pending until the user presses "Apply".
	let pending = $state<ThemeId>($theme);
	const currentTheme = $derived(THEMES.find((item) => item.id === $theme) ?? THEMES[0]);
	const pendingTheme = $derived(THEMES.find((item) => item.id === pending) ?? THEMES[0]);

	// Every time the dialog opens, restart from the currently active theme.
	// Closing the support dialog also dismisses any open QR code.
	$effect(() => {
		if ($supportOpen) {
			pending = $theme;
		} else {
			qrWallet = null;
		}
	});

	async function copy(address: string, id: string) {
		try {
			if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
			await navigator.clipboard.writeText(address);
		} catch {
			// Fallback for non-secure contexts (plain HTTP / older browsers).
			const area = document.createElement('textarea');
			area.value = address;
			area.setAttribute('readonly', '');
			area.style.position = 'fixed';
			area.style.opacity = '0';
			document.body.appendChild(area);
			area.select();
			try {
				document.execCommand('copy');
			} finally {
				area.remove();
			}
		}
		copiedId = id;
		notify(t('support.copied'), 'success');
		clearTimeout(copyTimer);
		copyTimer = setTimeout(() => (copiedId = null), 1500);
	}

	function apply() {
		if (pending === $theme) return;
		applyTheme(pending);
		notify(t('theme.applied'), 'success');
	}

	/** QR is always rendered dark-on-white so any camera can scan it. */
	function qrSvg(address: string): string {
		return renderSVG(address, {
			ecc: 'M',
			border: 2,
			pixelSize: 6,
			blackColor: '#000000',
			whiteColor: '#ffffff'
		});
	}
</script>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && qrWallet) {
			event.stopPropagation();
			qrWallet = null;
		}
	}}
/>

<!--
  The backdrop is transparent and does not capture clicks, so the sidebar/menu
  stays visible and usable behind the dialog. Only the dialog panel itself is
  opaque, with pointer events enabled.
-->
<Modal
	open={$supportOpen}
	title={t('support.title')}
	onclose={() => supportOpen.set(false)}
	backdropClass="pointer-events-none bg-transparent"
	panelClass="pointer-events-auto bg-slate-900"
>
	<p class="mb-3 text-xs text-slate-400">{t('support.subtitle')}</p>
	<div class="space-y-2">
		{#each wallets as wallet (wallet.id)}
			<div class="flex items-center gap-3 rounded-lg border border-slate-700 bg-slate-800 p-2">
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2">
						<span class="text-xs font-semibold text-slate-200">{wallet.name}</span>
						<span class="badge bg-slate-700 text-slate-300">{wallet.symbol}</span>
					</div>
					<code class="mt-0.5 block truncate font-mono text-xs text-slate-400" title={wallet.address}
						>{wallet.address}</code
					>
				</div>
				<div class="flex shrink-0 items-center gap-1">
					<button
						type="button"
						class="btn p-1.5"
						title={t('support.qr')}
						aria-label="{t('support.qr')} — {wallet.name}"
						onclick={() => (qrWallet = wallet)}
					>
						<svg viewBox="0 0 24 24" class="h-4 w-4" fill="currentColor" aria-hidden="true">
							<path
								d="M3 3v8h8V3H3zm6 6H5V5h4v4zM3 13v8h8v-8H3zm6 6H5v-4h4v4zM13 3v8h8V3h-8zm6 6h-4V5h4v4zM13 13h2v2h-2zM17 13h2v2h-2zM13 17h2v2h-2zM19 17h2v4h-4v-2h2zM15 19h2v2h-2zM19 13h2v2h-2z"
							/>
						</svg>
					</button>
					<button
						type="button"
						class="btn px-2 py-1"
						title={copiedId === wallet.id ? t('support.copied') : t('support.copy')}
						aria-label="{t('support.copy')} — {wallet.name}"
						onclick={() => copy(wallet.address, wallet.id)}
					>
						<span aria-hidden="true">{copiedId === wallet.id ? '✓' : '⧉'}</span>
					</button>
				</div>
			</div>
		{/each}
	</div>
	<p class="mt-4 text-center text-xs text-slate-400">{t('support.thanks')}</p>

	<!-- Interface theme picker -->
	<div class="mt-5 border-t border-slate-800 pt-4">
		<div class="mb-2 flex items-baseline justify-between gap-2">
			<h4 class="text-xs font-semibold uppercase tracking-wide text-slate-400">
				{t('theme.title')}
			</h4>
			<span class="truncate text-[10px] text-slate-500">
				{t('theme.current')}: {t(currentTheme.labelKey)}
			</span>
		</div>
		<div class="grid grid-cols-5 gap-2">
			{#each THEMES as item (item.id)}
				<button
					type="button"
					data-theme-preview={item.id}
					aria-pressed={pending === item.id}
					title={t(item.hintKey)}
					onclick={() => (pending = item.id)}
					class="group rounded-lg border-2 p-1.5 transition {pending === item.id
						? 'border-emerald-500 shadow-[var(--accent-glow)]'
						: 'border-slate-700 hover:border-slate-500'}"
				>
					<span class="block overflow-hidden rounded border border-slate-700/80">
						<span class="block h-1.5 bg-emerald-500"></span>
						<span class="block bg-slate-950 p-1">
							<span class="block h-1.5 w-4/5 rounded-sm bg-slate-700"></span>
							<span class="mt-1 block h-1.5 w-3/5 rounded-sm bg-slate-600"></span>
							<span class="mt-1 block h-1.5 w-2/3 rounded-sm bg-violet-500/70"></span>
						</span>
					</span>
					<span class="mt-1 block truncate text-center text-[10px] font-medium text-slate-300">
						{t(item.labelKey)}
					</span>
				</button>
			{/each}
		</div>
		<p class="mt-2 text-[11px] leading-snug text-slate-500">{t(pendingTheme.hintKey)}</p>
		<button
			type="button"
			class="btn btn-primary mt-3 w-full justify-center"
			disabled={pending === $theme}
			onclick={apply}
		>
			{pending === $theme ? t('theme.active') : t('theme.apply')}
		</button>
	</div>
</Modal>

<!-- QR code overlay: rendered outside the scrollable dialog panel. -->
{#if qrWallet}
	{@const wallet = qrWallet}
	<div class="fixed inset-0 z-[60] flex items-center justify-center p-4">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/70"
			aria-label={t('modal.close')}
			onclick={() => (qrWallet = null)}
		></button>
		<div
			class="relative w-full max-w-xs rounded-xl border border-slate-700 bg-slate-900 p-4 shadow-2xl"
			role="dialog"
			aria-modal="true"
			tabindex="-1"
			aria-label="{t('support.qrTitle')} — {wallet.name}"
		>
			<div class="mb-3 flex items-center justify-between gap-2">
				<div class="flex min-w-0 items-center gap-2">
					<span class="truncate text-sm font-semibold text-slate-100">{wallet.name}</span>
					<span class="badge shrink-0 bg-slate-700 text-slate-300">{wallet.symbol}</span>
				</div>
				<button
					type="button"
					class="btn px-2 py-0.5"
					onclick={() => (qrWallet = null)}
					aria-label={t('modal.close')}
				>
					✕
				</button>
			</div>
			<div
				class="mx-auto w-56 max-w-full rounded-lg bg-white p-3 [&_svg]:block [&_svg]:h-auto [&_svg]:w-full"
				role="img"
				aria-label="{t('support.qrTitle')}: {wallet.address}"
			>
				{@html qrSvg(wallet.address)}
			</div>
			<p class="mt-3 text-center text-[11px] text-slate-400">{t('support.qrHint')}</p>
			<code class="mt-2 block break-all rounded bg-slate-800 p-2 font-mono text-[11px] text-slate-300"
				>{wallet.address}</code
			>
			<button
				type="button"
				class="btn mt-2 w-full justify-center text-xs"
				onclick={() => copy(wallet.address, wallet.id)}
			>
				{copiedId === wallet.id ? t('support.copied') : t('support.copy')}
			</button>
		</div>
	</div>
{/if}
