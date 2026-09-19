<script lang="ts">
	import { SvelteFlow, Background, Controls } from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import GraphNode from './GraphNode.svelte';
	import { t } from '$lib/i18n.svelte';

	type ChainNode = {
		key: string;
		provider_id: string;
		provider?: string;
		label?: string;
		mode?: string;
		timeout_ms?: number;
		retries?: number;
		retry_delay_ms?: number;
		is_start?: boolean;
		pos_x?: number;
		pos_y?: number;
	};
	type ChainEdge = { from_key: string; to_key: string; condition?: string };
	type Chain = { name?: string; version?: number; mode?: string; nodes?: ChainNode[]; edges?: ChainEdge[] };
	type NodeStats = { took_ms?: number; added?: number; added_final?: boolean; error?: string };

	let {
		chain = { nodes: [], edges: [] },
		states = {},
		stats = {},
		finish = null
	}: {
		chain?: Chain;
		states?: Record<string, string>;
		stats?: Record<string, NodeStats>;
		finish?: string | null;
	} = $props();

	const nodeTypes = { graph: GraphNode };
	const fullChain = $derived(chain.mode === 'full_chain');

	// The last failed step decides the tooltip error; ok steps give the timing
	// badge and (full-chain) the "+n links" contribution.
	const nodes = $derived(
		(chain.nodes ?? []).map((node, index) => ({
			id: node.key,
			type: 'graph',
			position: {
				x: node.pos_x ?? index * 250,
				y: node.pos_y ?? 80
			},
			data: {
				key: node.key,
				label: node.label ?? node.provider_id,
				provider: node.provider ?? '',
				is_answer: node.mode === 'answer',
				timeout_ms: node.timeout_ms,
				retries: node.retries,
				retry_delay_ms: node.retry_delay_ms,
				is_start: node.is_start ?? false,
				finish: finish === node.key,
				mode: chain.mode ?? 'first_success',
				status: states[node.key] ?? 'gray',
				took_ms: stats[node.key]?.took_ms,
				added_links: stats[node.key]?.added ?? 0,
				added_final: stats[node.key]?.added_final ?? false,
				error: stats[node.key]?.error
			}
		}))
	);

	function edgeColor(condition: string): string {
		switch (condition) {
			case 'success':
			case 'ok':
				return 'var(--status-ok)';
			case 'empty':
				return 'var(--status-empty)';
			case 'fail':
				return 'var(--status-fail)';
			default:
				return 'var(--status-next)';
		}
	}

	function conditionToHandle(condition: string): string | undefined {
		switch (condition) {
			case 'success':
				return 'ok';
			case 'empty':
				return 'empty';
			case 'fail':
				return 'fail';
			case 'next':
			case 'any':
				return 'next';
			default:
				return undefined;
		}
	}

	const edges = $derived(
		(chain.edges ?? []).map((edge, index) => {
			const condition = edge.condition ?? 'next';
			const dimmed = fullChain && !['next', 'any', ''].includes(condition);
			return {
				id: 'e-' + index,
				source: edge.from_key,
				target: edge.to_key,
				sourceHandle: conditionToHandle(condition),
				label: condition,
				animated: states[edge.from_key] === 'running' || (fullChain && !dimmed),
				style: `stroke: ${edgeColor(condition)}; stroke-width: 2;` + (dimmed ? ' stroke-opacity: .35; stroke-dasharray: 5 4;' : '')
			};
		})
	);
</script>

<div class="relative h-[420px] w-full overflow-hidden rounded-xl border border-slate-800 bg-slate-950">
	<SvelteFlow
		{nodes}
		{edges}
		{nodeTypes}
		fitView
		colorMode="dark"
		nodesDraggable={false}
		nodesConnectable={false}
		elementsSelectable={false}
		zoomOnScroll={false}
		panOnScroll
	>
		<Background />
		<Controls />
	</SvelteFlow>
	{#if chain.name || chain.version !== undefined}
		<div class="pointer-events-none absolute left-2 top-2 z-10">
			<span class="badge bg-slate-800/80 font-mono text-slate-400">
				{chain.name ?? ''}{#if chain.version !== undefined} · v{chain.version}{/if}
			</span>
		</div>
	{/if}
	{#if fullChain}
		<div class="pointer-events-none absolute inset-x-0 top-2 flex justify-center">
			<span class="badge bg-violet-500/25 text-violet-200 shadow-lg">{t('request.fullChainBadge')}</span>
		</div>
	{/if}
</div>
