<script lang="ts">
	import { t } from '$lib/i18n.svelte';

	let {
		text = '',
		title
	}: {
		text?: string;
		title?: string;
	} = $props();

	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function copy() {
		if (!text) return;
		try {
			await navigator.clipboard.writeText(text);
		} catch {
			// Fallback for non-secure contexts / older browsers.
			const ta = document.createElement('textarea');
			ta.value = text;
			ta.setAttribute('readonly', '');
			ta.style.position = 'fixed';
			ta.style.opacity = '0';
			document.body.appendChild(ta);
			ta.select();
			try {
				document.execCommand('copy');
			} catch {
				/* ignore */
			}
			document.body.removeChild(ta);
		}
		copied = true;
		clearTimeout(timer);
		timer = setTimeout(() => (copied = false), 1500);
	}
</script>

<button
	type="button"
	class="btn p-1.5"
	title={copied ? t('common.copied') : (title ?? t('common.copy'))}
	aria-label={copied ? t('common.copied') : (title ?? t('common.copy'))}
	onclick={copy}
	disabled={!text}
>
	{#if copied}
		<svg
			viewBox="0 0 24 24"
			class="h-4 w-4 text-emerald-300"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<path d="M20 6 9 17l-5-5" />
		</svg>
	{:else}
		<svg
			viewBox="0 0 24 24"
			class="h-4 w-4"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			<rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
			<path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
		</svg>
	{/if}
</button>
