<script lang="ts">
	import { toast } from '$lib/stores';
	const tone: Record<string, string> = {
		info: 'border-slate-600 bg-slate-800 text-slate-100',
		success: 'border-emerald-600 bg-emerald-900/80 text-emerald-100',
		error: 'border-rose-600 bg-rose-900/80 text-rose-100'
	};
</script>

{#if $toast}
	<div class="fixed bottom-4 right-4 z-50 max-w-sm whitespace-pre-line rounded-lg border px-4 py-3 text-sm shadow-lg {tone[$toast.type]}">
		{$toast.message}
	</div>
{/if}
