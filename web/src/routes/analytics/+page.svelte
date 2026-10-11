<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import AnalyticsOverview from '$lib/components/analytics/AnalyticsOverview.svelte';
</script>

<div class="space-y-4">
	<div>
		<h1 class="text-xl font-semibold">{t('analytics.title')}</h1>
		<p class="text-xs text-slate-500">{t('analytics.subtitle')}</p>
	</div>
	<AnalyticsOverview />
</div>