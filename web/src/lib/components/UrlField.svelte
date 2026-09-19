<script lang="ts">
	let {
		value = $bindable(''),
		placeholder = '',
		label = '',
		hint = '',
		disabled = false
	}: {
		value?: string;
		placeholder?: string;
		label?: string;
		hint?: string;
		disabled?: boolean;
	} = $props();
</script>

<div>
	{#if label}<span class="label">{label}</span>{/if}
	<input class="input font-mono text-xs" bind:value {placeholder} {disabled} />
	{#if hint}<p class="mt-1 text-xs text-slate-500">{hint}</p>{/if}
</div>
