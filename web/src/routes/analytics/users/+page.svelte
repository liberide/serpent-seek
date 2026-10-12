<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session } from '$lib/stores';
	import { t } from '$lib/i18n.svelte';
	import AnalyticsUsers from '$lib/components/analytics/AnalyticsUsers.svelte';

	// The per-user section is admin-only (the leaderboard endpoint enforces it
	// too); everyone else lands back on the overview.
	const isAdmin = $derived(!!$session?.admin);

	onMount(() => {
		if (!$session?.admin) {
			goto('/analytics', { replaceState: true });
		}
	});
</script>

{#if isAdmin}
	<div class="space-y-4">
		<div>
			<h1 class="text-xl font-semibold">{t('analytics.title')}</h1>
			<p class="text-xs text-slate-500">{t('analytics.users.subtitle')}</p>
		</div>
		<AnalyticsUsers />
	</div>
{/if}