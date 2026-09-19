<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n.svelte';

	let {
		open = false,
		title = '',
		onclose,
		backdropClass = 'bg-black/60',
		panelClass = 'bg-slate-900',
		children,
		footer
	}: {
		open?: boolean;
		title?: string;
		onclose?: () => void;
		/** Extra classes for the full-screen backdrop (e.g. a transparent, click-through one). */
		backdropClass?: string;
		/** Extra classes for the dialog panel itself. */
		panelClass?: string;
		children?: Snippet;
		/** Action bar pinned to the bottom of the dialog (e.g. Cancel/Save buttons). */
		footer?: Snippet;
	} = $props();
</script>

{#if open}
	<div
		class="fixed inset-0 z-40 flex items-center justify-center p-4 {backdropClass}"
		role="presentation"
	>
		<div
			class="flex max-h-[calc(100dvh-2rem)] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-slate-700 shadow-2xl {panelClass}"
			role="dialog"
			aria-modal="true"
		>
			<div class="flex shrink-0 items-center justify-between border-b border-slate-800 px-4 py-3">
				<h3 class="text-sm font-semibold">{title}</h3>
				<button class="btn px-2 py-0.5" onclick={() => onclose?.()} aria-label={t('modal.close')}>✕</button>
			</div>
			<div class="min-h-0 flex-1 overflow-y-auto p-4">
				{@render children?.()}
			</div>
			{#if footer}
				<div class="shrink-0 border-t border-slate-800 px-4 py-3">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
