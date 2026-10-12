<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import { supportOpen } from '$lib/stores';

	// `compact` renders the icon-only variant used by the collapsed sidebar
	// rail, where there is no room for the "Support the project" label.
	let { compact = false }: { compact?: boolean } = $props();
</script>

<button
	type="button"
	class={compact
		? 'rounded-md p-1.5 transition hover:bg-slate-800'
		: 'support-link mt-1 inline-block text-left text-xs text-slate-500'}
	title={t('support.link')}
	aria-label={t('support.link')}
	onclick={() => supportOpen.set(true)}
>
	{#if compact}
		<!-- Slowly shimmering, slightly dimmed heart (masked gradient). -->
		<span class="support-heart"></span>
	{:else}
		{t('support.link')}
	{/if}
</button>
