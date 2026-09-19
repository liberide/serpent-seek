<script lang="ts">
	import { untrack } from 'svelte';
	import { useSvelteFlow } from '@xyflow/svelte';

	type FitOptions = {
		padding?: number;
		minZoom?: number;
		maxZoom?: number;
		duration?: number;
	};

	// FlowFitter lives inside <SvelteFlow> to get the flow instance and hands
	// its fitView to the parent: used to pull the camera back after a block
	// is appended to the chain.
	let { register }: { register: (fit: (options?: FitOptions) => void) => void } = $props();

	const { fitView } = useSvelteFlow();
	// Register once at setup: the parent holds the callback, props never change.
	untrack(() => register((options) => void fitView(options)));
</script>
