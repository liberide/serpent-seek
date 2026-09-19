<script lang="ts">
	import { statusTone, toneClasses } from '$lib/format';
	import { t } from '$lib/i18n.svelte';
	let { status, label }: { status?: string; label?: string } = $props();
</script>

<span class="badge {toneClasses[statusTone(status)]}">{label ?? status ?? t('common.unknown')}</span>
