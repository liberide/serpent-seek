<script lang="ts">
	import DeveloperPanel from '$lib/components/DeveloperPanel.svelte';
	import { t } from '$lib/i18n.svelte';
</script>

<div class="space-y-4">
	<div>
		<h1 class="text-xl font-semibold">{t('nav.developer')}</h1>
		<p class="text-sm text-slate-400">{t('developer.subtitle')}</p>
	</div>
	<DeveloperPanel />
</div>
