<script lang="ts">
	import { formatInt, formatPct } from '$lib/format';

	type Segment = { key: string; label: string; value: number; color: string };

	let {
		segments = [],
		total = 0,
		centerValue = '',
		centerLabel = ''
	}: {
		segments?: Segment[];
		total?: number;
		centerValue?: string;
		centerLabel?: string;
	} = $props();

	const radius = 70;
	const circumference = 2 * Math.PI * radius;
	const active = $derived(segments.filter((s) => s.value > 0));
	const sum = $derived(active.reduce((acc, s) => acc + s.value, 0));

	let offset = 0;
	const arcs = $derived.by(() => {
		offset = 0;
		return active.map((segment) => {
			const fraction = sum > 0 ? segment.value / sum : 0;
			const dash = fraction * circumference;
			const arc = { ...segment, dash, gap: circumference - dash, offset };
			offset += dash;
			return arc;
		});
	});
</script>

<div class="flex w-full flex-col items-center gap-4">
	<div class="relative h-44 w-44 shrink-0">
		<svg viewBox="0 0 180 180" class="h-full w-full -rotate-90" role="img">
			<circle
				cx="90"
				cy="90"
				r={radius}
				fill="none"
				stroke="var(--color-slate-800)"
				stroke-width="18"
			/>
			{#each arcs as arc (arc.key)}
				<circle
					cx="90"
					cy="90"
					r={radius}
					fill="none"
					stroke={arc.color}
					stroke-width="18"
					stroke-linecap="butt"
					stroke-dasharray="{arc.dash} {arc.gap}"
					stroke-dashoffset={-arc.offset}
					style="transition: stroke-dasharray 0.6s ease, stroke-dashoffset 0.6s ease;"
				>
					<title>{arc.label}: {formatInt(arc.value)}</title>
				</circle>
			{/each}
		</svg>
		<div class="absolute inset-0 flex flex-col items-center justify-center">
			<span class="text-2xl font-semibold text-slate-100">{centerValue}</span>
			<span class="text-[11px] uppercase tracking-wide text-slate-500">{centerLabel}</span>
		</div>
	</div>
	<ul class="w-full space-y-2">
		{#each segments as segment (segment.key)}
			<li class="flex items-center justify-between gap-3 text-sm">
				<span class="flex min-w-0 items-center gap-2 text-slate-300">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{segment.color}"></span>
					<span class="min-w-0 break-words">{segment.label}</span>
				</span>
				<span class="shrink-0 whitespace-nowrap tabular-nums text-slate-400">
					{formatInt(segment.value)}
					<span class="ml-1 text-xs text-slate-500">
						({formatPct(total > 0 ? (segment.value / total) * 100 : 0)})
					</span>
				</span>
			</li>
		{/each}
	</ul>
</div>
