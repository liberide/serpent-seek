<script lang="ts">
	import { t } from '$lib/i18n.svelte';

	let {
		value = $bindable(''),
		placeholder = '',
		label = '',
		alreadySet = false,
		hint = '',
		multiline = false
	}: {
		value?: string;
		placeholder?: string;
		label?: string;
		alreadySet?: boolean;
		hint?: string;
		multiline?: boolean;
	} = $props();
</script>

<div>
	{#if label}<span class="label">{label}</span>{/if}
	{#if multiline}
		<textarea
			autocomplete="off"
			spellcheck="false"
			class="input h-32 font-mono text-xs"
			bind:value
			{placeholder}
		></textarea>
	{:else}
		<input
			type="password"
			autocomplete="new-password"
			class="input font-mono text-xs"
			bind:value
			{placeholder}
		/>
	{/if}
	<p class="mt-1 text-xs text-slate-500">
		{#if alreadySet}<span class="text-emerald-400">{t('providers.secretSet')}</span> {t('providers.secretKeep')} {/if}
		{hint}
	</p>
</div>
