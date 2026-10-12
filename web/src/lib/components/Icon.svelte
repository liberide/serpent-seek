<script lang="ts">
	import { ICONS, type IconName } from './icons';

	let {
		name,
		class: className = 'h-5 w-5',
		stroke = 1.7
	}: { name: IconName; class?: string; stroke?: number } = $props();
</script>

<!--
  Renders one of the original sidebar glyphs. Shapes are stroked with the
  current text colour; `filled` shapes use it as their fill instead. The icon
  is decorative — the accessible name comes from the surrounding link/button.
-->
<svg
	class={className}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width={stroke}
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	{#each ICONS[name] as shape}
		{#if shape.t === 'path'}
			<path d={shape.d} fill={shape.filled ? 'currentColor' : 'none'} stroke={shape.filled ? 'none' : undefined}
			/>
		{:else if shape.t === 'circle'}
			<circle
				cx={shape.cx}
				cy={shape.cy}
				r={shape.r}
				fill={shape.filled ? 'currentColor' : 'none'}
				stroke={shape.filled ? 'none' : undefined}
			/>
		{:else}
			<rect x={shape.x} y={shape.y} width={shape.w} height={shape.h} rx={shape.rx ?? 0} />
		{/if}
	{/each}
</svg>
