<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import { supportOpen } from '$lib/stores';
</script>

<button
	type="button"
	class="support-link mt-1 inline-block text-left text-xs text-slate-500"
	onclick={() => supportOpen.set(true)}
>
	{t('support.link')}
</button>
