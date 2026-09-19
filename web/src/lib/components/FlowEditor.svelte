<script lang="ts">
	import { tick } from 'svelte';
	import { SvelteFlow, Background, Controls } from '@xyflow/svelte';
	import '@xyflow/svelte/dist/style.css';
	import GraphNode from './GraphNode.svelte';
	import FlowFitter from './FlowFitter.svelte';
	import Confirm from './Confirm.svelte';
	import { notify } from '$lib/stores';
	import { t } from '$lib/i18n.svelte';

	type ChainNode = {
		id?: string;
		key: string;
		provider_id: string;
		label?: string;
		mode?: string;
		params?: Record<string, string>;
		timeout_ms: number;
		retries: number;
		retry_delay_ms: number;
		delay_policy: string;
		on_success: string;
		on_empty: string;
		on_fail: string;
		is_start?: boolean;
		pos_x?: number;
		pos_y?: number;
	};
	type ChainEdge = { id?: string; from_key: string; to_key: string; condition: string };
	type Chain = { id?: string; name: string; mode?: string; nodes: ChainNode[]; edges: ChainEdge[] };
	export type ProviderInstance = { id: string; code: string; name: string; enabled: boolean };

	let {
		chain,
		providers = [],
		errors = [],
		dirty = false,
		maxAttempts = 10,
		onchange
	}: {
		chain: Chain;
		providers?: ProviderInstance[];
		errors?: string[];
		dirty?: boolean;
		maxAttempts?: number;
		onchange?: (chain: Chain) => void;
	} = $props();

	const nodeTypes = { graph: GraphNode };
	const TIMEOUT_PRESETS = [5000, 10000, 20000];

	let flowNodes = $state<any[]>([]);
	let flowEdges = $state<any[]>([]);
	let selectedKey = $state<string | null>(null);
	let selectedEdgeId = $state<string | null>(null);
	let history = $state<string[]>([]);
	let loadedId: string | undefined = undefined;
	let mode = $state<string>('first_success');
	let pendingMode = $state<string | null>(null);
	let localErrors = $state<string[]>([]);
	let localWarnings = $state<string[]>([]);
	let menu = $state<{ x: number; y: number; key: string } | null>(null);
	let autoFit = $state<
		null | ((options?: { padding?: number; minZoom?: number; maxZoom?: number; duration?: number }) => void)
	>(null);

	$effect(() => {
		if (loadedId !== chain.id) {
			loadedId = chain.id;
			load(chain);
		}
	});

	const enabledProviders = $derived(providers.filter((p) => p.enabled));
	const selectedNode = $derived(flowNodes.find((n) => n.id === selectedKey));
	const selectedEdge = $derived(flowEdges.find((e) => e.id === selectedEdgeId));
	const selectedParams = $derived(
		selectedNode ? Object.entries(selectedNode.data.node.params ?? {}) : []
	);
	const startOwner = $derived(flowNodes.find((n) => n.data.node.is_start));
	const fullChain = $derived(mode === 'full_chain');

	function instance(id: string): ProviderInstance | undefined {
		return providers.find((p) => p.id === id);
	}

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

	function edgeCondition(edge: any): string {
		return typeof edge.label === 'string' && edge.label ? edge.label : 'next';
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

	function restyleEdge(edge: any): any {
		const condition = edgeCondition(edge);
		// In full_chain only neutral next-edges steer the walk: dim the rest.
		const dimmed = fullChain && !['next', 'any', ''].includes(condition);
		return {
			...edge,
			sourceHandle: edge.sourceHandle ?? conditionToHandle(condition),
			style:
				`stroke: ${edgeColor(condition)}; stroke-width: 2;` +
				(dimmed ? ' stroke-opacity: .35; stroke-dasharray: 5 4;' : ''),
			animated: fullChain && !dimmed
		};
	}

	function load(source: Chain) {
		mode = source.mode ?? 'first_success';
		flowNodes = (source.nodes ?? []).map((node, index) => {
			const inst = instance(node.provider_id);
			const label = node.label ?? inst?.name ?? node.provider_id;
			return {
				id: node.key,
				type: 'graph',
				position: { x: node.pos_x ?? index * 250, y: node.pos_y ?? 80 },
				data: {
					node: {
						key: node.key,
						provider_id: node.provider_id,
						label,
						mode: node.mode ?? 'search',
						params: { ...(node.params ?? {}) },
						timeout_ms: node.timeout_ms ?? 20000,
						retries: node.retries ?? 0,
						retry_delay_ms: node.retry_delay_ms ?? 700,
						delay_policy: node.delay_policy ?? 'linear',
						on_success: node.on_success ?? 'stop',
						on_empty: node.on_empty ?? 'next',
						on_fail: node.on_fail ?? 'next',
						is_start: node.is_start ?? false
					},
					key: node.key,
					label,
					provider: inst?.code ?? '',
					interactive: true,
					onToggleStart: toggleStart,
					onCycleTimeout: cycleTimeout
				}
			};
		});
		flowEdges = (source.edges ?? []).map((edge, index) => {
			const e = {
				id: 'e-' + index + '-' + edge.from_key,
				source: edge.from_key,
				target: edge.to_key,
				label: edge.condition ?? 'next'
			} as any;
			return restyleEdge(e);
		});
		selectedKey = null;
		selectedEdgeId = null;
		refreshFlags();
		computeIssues();
	}

	function serialized(): Chain {
		return {
			id: chain.id,
			name: chain.name,
			mode,
			nodes: flowNodes.map((n) => ({
				...n.data.node,
				key: n.id,
				pos_x: n.position.x,
				pos_y: n.position.y
			})),
			edges: flowEdges.map((e) => ({
				from_key: e.source,
				to_key: e.target,
				condition: edgeCondition(e)
			}))
		};
	}

	function snapshot() {
		history = [...history.slice(-29), JSON.stringify(serialized())];
	}

	function undo() {
		const previous = history.pop();
		if (!previous) return;
		load(JSON.parse(previous));
		sync();
	}

	// refreshFlags mirrors node.card display fields (start state, validation
	// marker, mode chip, settings line) from the source of truth in data.node.
	function refreshFlags() {
		const owner = flowNodes.find((n) => n.data.node.is_start);
		const ownerName = owner ? (owner.data.node.label ?? owner.id) : '';
		for (const n of flowNodes) {
			n.data.is_start = !!n.data.node.is_start;
			n.data.start_taken = !!owner;
			n.data.start_owner = ownerName;
			n.data.is_answer = n.data.node.mode === 'answer';
			n.data.mode = mode;
			n.data.timeout_ms = n.data.node.timeout_ms;
			n.data.retries = n.data.node.retries;
			n.data.retry_delay_ms = n.data.node.retry_delay_ms;
		}
	}

	function sync() {
		refreshFlags();
		computeIssues();
		onchange?.(serialized());
	}

	function reachable(fromKey: string, accept: ((condition: string) => boolean) | null): Set<string> {
		const adj = new Map<string, string[]>();
		for (const e of flowEdges) {
			if (accept && !accept(edgeCondition(e))) continue;
			adj.set(e.source, [...(adj.get(e.source) ?? []), e.target]);
		}
		const seen = new Set<string>();
		const queue = [fromKey];
		while (queue.length > 0) {
			const cur = queue.shift()!;
			if (seen.has(cur)) continue;
			seen.add(cur);
			queue.push(...(adj.get(cur) ?? []));
		}
		return seen;
	}

	// computeIssues runs the same rules as the server validator live on the
	// canvas: exactly one start block, attempt budget, full-chain reachability.
	function computeIssues() {
		const errs: string[] = [];
		const warns: string[] = [];
		for (const n of flowNodes) n.data.invalid = false;
		if (flowNodes.length === 0) {
			localErrors = [];
			localWarnings = [];
			return;
		}
		const starts = flowNodes.filter((n) => n.data.node.is_start);
		if (starts.length === 0) {
			errs.push(t('flow.startMissing'));
			for (const n of flowNodes) n.data.invalid = true;
		}
		if (starts.length > 1) {
			errs.push(t('flow.startMultiple', { keys: starts.map((s) => s.data.node.label ?? s.id).join(', ') }));
			for (const s of starts) s.data.invalid = true;
		}
		for (const s of starts) {
			if (s.data.node.mode === 'answer') {
				errs.push(t('flow.answerNotStart'));
				s.data.invalid = true;
			}
		}
		if (starts.length === 1) {
			const reach = reachable(starts[0].id, null);
			const potential = flowNodes
				.filter((n) => reach.has(n.id))
				.reduce((acc, n) => acc + (Number(n.data.node.retries) || 0) + 1, 0);
			if (maxAttempts > 0 && potential > maxAttempts) {
				warns.push(t('flow.budgetWarn', { x: potential, y: maxAttempts }));
			}
			if (fullChain) {
				const nextReach = reachable(starts[0].id, (c) => ['next', 'any', ''].includes(c));
				for (const n of flowNodes) {
					if (reach.has(n.id) && !nextReach.has(n.id)) {
						warns.push(t('flow.unreachableNext', { name: n.data.node.label ?? n.id }));
					}
				}
			}
		}
		localErrors = errs;
		localWarnings = warns;
	}

	function toggleStart(key: string, value: boolean) {
		const candidate = flowNodes.find((n) => n.id === key);
		if (value && candidate?.data.node.mode === 'answer') {
			notify(t('flow.answerNotStart'), 'error');
			return;
		}
		snapshot();
		for (const n of flowNodes) {
			if (value) n.data.node.is_start = n.id === key;
			else if (n.id === key) n.data.node.is_start = false;
		}
		sync();
	}

	function cycleTimeout(key: string) {
		const node = flowNodes.find((n) => n.id === key);
		if (!node) return;
		snapshot();
		const cur = Number(node.data.node.timeout_ms) || 20000;
		const idx = TIMEOUT_PRESETS.indexOf(cur);
		node.data.node.timeout_ms = TIMEOUT_PRESETS[(idx + 1) % TIMEOUT_PRESETS.length];
		sync();
	}

	// reassignStart picks a new start block after the previous one vanished:
	// the node with the fewest incoming edges, ties broken by the leftmost pos_x.
	function reassignStart() {
		if (flowNodes.length === 0 || flowNodes.some((n) => n.data.node.is_start)) return;
		const incoming = new Map<string, number>();
		for (const e of flowEdges) incoming.set(e.target, (incoming.get(e.target) ?? 0) + 1);
		const sorted = [...flowNodes]
			.filter((n) => n.data.node.mode !== 'answer')
			.sort(
				(a, b) => (incoming.get(a.id) ?? 0) - (incoming.get(b.id) ?? 0) || a.position.x - b.position.x
			);
		const next = sorted[0];
		if (!next) return;
		next.data.node.is_start = true;
		notify(t('flow.startReassigned', { name: next.data.node.label ?? next.id }), 'info');
	}

	function requestMode(next: string) {
		if (next === mode) return;
		if (dirty) {
			pendingMode = next;
			return;
		}
		applyMode(next);
	}

	function applyMode(next: string) {
		snapshot();
		mode = next;
		pendingMode = null;
		flowEdges = flowEdges.map((e) => restyleEdge({ ...e }));
		sync();
	}

	function addNode(provider: ProviderInstance) {
		snapshot();
		const key = 'blk-' + Math.random().toString(36).slice(2, 7);
		const count = flowNodes.length;
		const prev = flowNodes[flowNodes.length - 1];
		// Append right after the previous block, same row — the camera then
		// refits so the growing line stays fully visible.
		const position = prev ? { x: prev.position.x + 250, y: prev.position.y } : { x: 120, y: 80 };
		flowNodes = [
			...flowNodes,
			{
				id: key,
				type: 'graph',
				position,
				data: {
					node: {
						key,
						provider_id: provider.id,
						label: provider.name,
						mode: 'search',
						params: {},
						timeout_ms: 20000,
						retries: 0,
						retry_delay_ms: 700,
						delay_policy: 'linear',
						on_success: 'stop',
						on_empty: 'next',
						on_fail: 'next',
						// The first block added to an empty chain becomes the start.
						is_start: count === 0
					},
					key,
					label: provider.name,
					provider: provider.code,
					interactive: true,
					onToggleStart: toggleStart,
					onCycleTimeout: cycleTimeout
				}
			}
		];
		selectedKey = key;
		selectedEdgeId = null;
		sync();
		void refit();
	}

	// refit pulls the camera back (fit to content) once the new block is in.
	async function refit() {
		await tick();
		autoFit?.({ padding: 0.3, maxZoom: 1, duration: 300 });
	}

	function duplicateNode(key: string) {
		const src = flowNodes.find((n) => n.id === key);
		if (!src) return;
		snapshot();
		const newKey = 'blk-' + Math.random().toString(36).slice(2, 7);
		const nodeCopy = JSON.parse(JSON.stringify(src.data.node));
		nodeCopy.key = newKey;
		nodeCopy.is_start = false;
		nodeCopy.label = (nodeCopy.label ?? newKey) + ' (copy)';
		flowNodes = [
			...flowNodes,
			{
				id: newKey,
				type: 'graph',
				position: { x: src.position.x + 40, y: src.position.y + 40 },
				data: {
					node: nodeCopy,
					key: newKey,
					label: nodeCopy.label,
					provider: src.data.provider,
					interactive: true,
					onToggleStart: toggleStart,
					onCycleTimeout: cycleTimeout
				}
			}
		];
		selectedKey = newKey;
		sync();
	}

	function deleteNode(key: string) {
		snapshot();
		const wasStart = !!flowNodes.find((n) => n.id === key)?.data.node.is_start;
		flowNodes = flowNodes.filter((n) => n.id !== key);
		flowEdges = flowEdges.filter((e) => e.source !== key && e.target !== key);
		if (selectedKey === key) selectedKey = null;
		if (wasStart) reassignStart();
		sync();
	}

	function deleteSelected() {
		if (selectedEdgeId) {
			snapshot();
			flowEdges = flowEdges.filter((e) => e.id !== selectedEdgeId);
			selectedEdgeId = null;
			sync();
			return;
		}
		if (selectedKey) deleteNode(selectedKey);
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key !== 'Delete' && event.key !== 'Backspace') return;
		const target = event.target as HTMLElement | null;
		if (target && (target.closest('input, textarea, select, [contenteditable="true"]') || target.tagName === 'INPUT')) {
			return;
		}
		if (selectedKey || selectedEdgeId) {
			event.preventDefault();
			deleteSelected();
		}
	}

	function refresh(node: any) {
		const inst = instance(node.data.node.provider_id);
		node.data.provider = inst?.code ?? '';
		node.data.label = node.data.node.label;
		refreshFlags();
	}

	function onProviderChange(node: any) {
		const inst = instance(node.data.node.provider_id);
		if (inst) node.data.node.label = inst.name;
		refresh(node);
		sync();
	}

	function setNodeMode(next: string) {
		if (!selectedNode) return;
		if (next === 'answer' && selectedNode.data.node.is_start) {
			notify(t('flow.answerNotStart'), 'error');
			return;
		}
		snapshot();
		selectedNode.data.node.mode = next;
		sync();
	}

	function setTimeoutSeconds(node: any, seconds: number) {
		const s = Math.min(120, Math.max(1, Math.round(seconds || 20)));
		node.data.node.timeout_ms = s * 1000;
		sync();
	}

	function setRetries(node: any, retries: number) {
		node.data.node.retries = Math.min(10, Math.max(0, Math.round(retries || 0)));
		sync();
	}

	function setRetryDelay(node: any, delay: number) {
		node.data.node.retry_delay_ms = Math.min(30000, Math.max(0, Math.round(delay || 0)));
		sync();
	}

	const HANDLE_TO_CONDITION: Record<string, string> = { ok: 'success', empty: 'empty', fail: 'fail', next: 'next' };

	type FlowConnection = {
		source: string | null;
		target: string | null;
		sourceHandle?: string | null;
		targetHandle?: string | null;
	};

	// SvelteFlow adds the connected edge to the bound `flowEdges` itself
	// (see store.addEdge in the library). onbeforeconnect turns the pending
	// connection into the final edge so exactly ONE edge is created; adding
	// another edge in onconnect used to leave a phantom label-less "next"
	// next to every real edge.
	function beforeConnect(connection: FlowConnection): any {
		if (!connection.source || !connection.target) return false;
		snapshot();
		const condition =
			HANDLE_TO_CONDITION[connection.sourceHandle ?? ''] ?? (fullChain ? 'next' : 'fail');
		return restyleEdge({
			id: 'e-' + Math.random().toString(36).slice(2, 7),
			source: connection.source,
			target: connection.target,
			sourceHandle: connection.sourceHandle ?? conditionToHandle(condition),
			targetHandle: connection.targetHandle ?? undefined,
			label: condition
		});
	}

	async function connected() {
		// Wait for the edge added by SvelteFlow to land in the bound array.
		await tick();
		sync();
	}

	function onEdgeConditionChange(edge: any, condition: string) {
		flowEdges = flowEdges.map((e) =>
			e.id === edge.id
				? restyleEdge({ ...e, label: condition, sourceHandle: conditionToHandle(condition) })
				: e
		);
		sync();
	}

	function addParam() {
		if (!selectedNode) return;
		const key = prompt(t('flow.paramKey'));
		if (!key) return;
		selectedNode.data.node.params[key] = '';
		sync();
	}
</script>

<svelte:window onkeydown={onKeydown} onclick={() => (menu = null)} />

<div class="grid grid-cols-1 gap-4 lg:grid-cols-[180px_1fr_300px]">
	<!-- palette -->
	<aside class="card h-fit">
		<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">{t('flow.providers')}</h3>
		<div class="flex flex-col gap-2">
			{#each enabledProviders as provider (provider.id)}
				<button
					class="w-full cursor-pointer rounded-lg border border-slate-700 bg-slate-800 px-2.5 py-2 text-left transition hover:bg-slate-700"
					title={`${provider.name} · ${provider.code}`}
					onclick={() => addNode(provider)}
				>
					<span class="flex items-start gap-1.5">
						<span class="text-emerald-400">+</span>
						<span class="min-w-0 flex-1 break-words text-sm font-medium leading-tight text-slate-100"
							>{provider.name}</span
						>
					</span>
					<span class="mt-0.5 block break-all pl-4 font-mono text-[10px] text-slate-500">⚙ {provider.code}</span>
				</button>
			{/each}
			{#if enabledProviders.length === 0}
				<p class="text-xs text-slate-500">{t('flow.noEnabled')}</p>
			{/if}
		</div>
		{#if providers.length > enabledProviders.length}
			<p class="mt-2 text-[10px] text-slate-500">
				{t('flow.hidden', { n: providers.length - enabledProviders.length })}
			</p>
		{/if}
		<button class="btn mt-3 w-full" onclick={undo} disabled={history.length === 0}>{t('flow.undo')}</button>
		<button class="btn btn-danger mt-2 w-full" onclick={deleteSelected} disabled={!selectedKey && !selectedEdgeId}>
			{t('flow.deleteSelected')}
		</button>
		<p class="mt-2 text-[10px] text-slate-500">{t('flow.keyboardHint')}</p>
		{#if errors.length > 0 || localErrors.length > 0}
			<div class="mt-3 rounded-lg border border-rose-700 bg-rose-950/40 p-2 text-xs text-rose-200">
				{#each [...errors, ...localErrors] as error, i (i)}<p>• {error}</p>{/each}
			</div>
		{/if}
		{#if localWarnings.length > 0}
			<div class="mt-3 rounded-lg border border-amber-600 bg-amber-950/40 p-2 text-xs text-amber-200">
				{#each localWarnings as warning, i (i)}<p>⚠ {warning}</p>{/each}
			</div>
		{/if}
	</aside>

	<!-- canvas -->
	<div class="flex flex-col gap-2">
		<!-- mode selector (chain template) -->
		<div class="flex flex-wrap items-center gap-2">
			<span class="text-[10px] font-semibold uppercase tracking-wide text-slate-500">{t('flow.mode')}</span>
			<div class="inline-flex overflow-hidden rounded-lg border border-slate-700">
				<button
					class="px-3 py-1 text-xs transition {mode === 'first_success'
						? 'bg-emerald-600 text-white'
						: 'bg-slate-800 text-slate-300 hover:bg-slate-700'}"
					onclick={() => requestMode('first_success')}
				>
					⚡ {t('flow.modeFirst')}
				</button>
				<button
					class="px-3 py-1 text-xs transition {mode === 'full_chain'
						? 'bg-violet-600 text-white'
						: 'bg-slate-800 text-slate-300 hover:bg-slate-700'}"
					onclick={() => requestMode('full_chain')}
				>
					🧩 {t('flow.modeFull')}
				</button>
			</div>
			{#if startOwner}
				<span class="badge bg-amber-400/15 text-amber-300">
					▶ {startOwner.data.node.label ?? startOwner.id}
				</span>
			{/if}
		</div>

		<div class="relative h-[560px] overflow-hidden rounded-xl border border-slate-800 bg-slate-950">
			<div class:opacity-90={fullChain} class="h-full w-full">
				<SvelteFlow
					bind:nodes={flowNodes}
					bind:edges={flowEdges}
					{nodeTypes}
					fitView
					colorMode="dark"
					onbeforeconnect={beforeConnect}
					onconnect={connected}
					onnodeclick={({ node }) => {
						selectedKey = node.id;
						selectedEdgeId = null;
					}}
					onnodecontextmenu={({ node, event }) => {
						event.preventDefault();
						selectedKey = node.id;
						selectedEdgeId = null;
						menu = { x: event.clientX, y: event.clientY, key: node.id };
					}}
					onedgeclick={({ edge }) => {
						selectedEdgeId = edge.id;
						selectedKey = null;
					}}
					onnodedragstop={() => sync()}
				>
					<Background />
					<Controls />
					<FlowFitter register={(fit) => (autoFit = fit)} />
				</SvelteFlow>
			</div>
			{#if fullChain}
				<div class="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center">
					<span class="badge bg-violet-500/25 text-violet-200 shadow-lg">🧩 {t('flow.fullDimHint')}</span>
				</div>
			{/if}
			{#if menu}
				<div
					class="fixed z-50 min-w-[160px] rounded-lg border border-slate-700 bg-slate-900 p-1 shadow-xl"
					style="left: {menu.x}px; top: {menu.y}px"
				>
					<button
						class="block w-full rounded px-3 py-1.5 text-left text-xs text-slate-200 hover:bg-slate-800"
						onclick={() => {
							if (menu) duplicateNode(menu.key);
							menu = null;
						}}>{t('flow.duplicate')}</button
					>
					<button
						class="block w-full rounded px-3 py-1.5 text-left text-xs text-rose-300 hover:bg-rose-950/60"
						onclick={() => {
							if (menu) deleteNode(menu.key);
							menu = null;
						}}>{t('common.delete')}</button
					>
				</div>
			{/if}
		</div>
	</div>

	<!-- node config -->
	<aside class="card h-fit">
		{#if selectedEdge}
			<h3 class="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-400">{t('flow.edge')}</h3>
			<span class="label">{t('flow.condition')}</span>
			<select
				class="input"
				value={edgeCondition(selectedEdge)}
				onchange={(event) => onEdgeConditionChange(selectedEdge, (event.currentTarget as HTMLSelectElement).value)}
			>
				<option value="success">ok · success</option>
				<option value="empty">empty</option>
				<option value="fail">fail</option>
				<option value="next">next</option>
				<option value="any">any</option>
			</select>
			<p class="mt-2 text-xs text-slate-500">{selectedEdge.source} → {selectedEdge.target}</p>
			{#if fullChain && !['next', 'any'].includes(edgeCondition(selectedEdge))}
				<p class="mt-2 text-xs text-amber-300/80">🧩 {t('flow.edgeDimNote')}</p>
			{/if}
		{:else if selectedNode}
			<h3 class="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-400">{t('flow.node')}</h3>
			<div class="space-y-2">
				<label class="flex items-center justify-between rounded-lg border border-slate-700 px-2 py-1.5">
					<span class="text-xs font-semibold text-amber-300">▶ {t('flow.start')}</span>
					<input
						type="checkbox"
						class="accent-amber-400"
						checked={!!selectedNode.data.node.is_start}
						disabled={(!!startOwner && startOwner.id !== selectedNode.id) || selectedNode.data.node.mode === 'answer'}
						title={selectedNode.data.node.mode === 'answer'
							? t('flow.answerNotStart')
							: startOwner && startOwner.id !== selectedNode.id
								? t('flow.startTaken', { name: startOwner.data.node.label ?? startOwner.id })
								: ''}
						onchange={(event) => toggleStart(selectedNode.id, (event.currentTarget as HTMLInputElement).checked)}
					/>
				</label>
				<div>
					<span class="label">{t('common.provider')}</span>
					<select
						class="input"
						bind:value={selectedNode.data.node.provider_id}
						onchange={() => onProviderChange(selectedNode)}
					>
						{#each enabledProviders as provider (provider.id)}
							<option value={provider.id}>{provider.name} · {provider.code}</option>
						{/each}
					</select>
				</div>
				<div>
					<span class="label">{t('flow.nodeMode')}</span>
					<select
						class="input"
						value={selectedNode.data.node.mode ?? 'search'}
						onchange={(event) => setNodeMode((event.currentTarget as HTMLSelectElement).value)}
					>
						<option value="search">🔎 {t('flow.modeSearch')}</option>
						<option value="answer">💬 {t('flow.modeAnswer')}</option>
					</select>
				</div>
				<div>
					<span class="label">{t('common.name')}</span>
					<input class="input" bind:value={selectedNode.data.node.label} oninput={() => { refresh(selectedNode); sync(); }} />
				</div>
				<div class="grid grid-cols-2 gap-2">
					<div>
						<span class="label">{t('flow.timeoutSec')}</span>
						<div class="flex gap-1">
							{#each [5, 10, 20] as preset (preset)}
								<button
									class="btn min-w-0 flex-1 px-1 py-1 text-xs {Math.round(selectedNode.data.node.timeout_ms / 1000) === preset
										? 'border-emerald-500 text-emerald-300'
										: ''}"
									onclick={() => setTimeoutSeconds(selectedNode, preset)}>{preset}s</button
								>
							{/each}
						</div>
						<input
							type="number"
							min="1"
							max="120"
							class="input mt-1 font-mono"
							value={Math.round(selectedNode.data.node.timeout_ms / 1000)}
							onchange={(event) => setTimeoutSeconds(selectedNode, Number((event.currentTarget as HTMLInputElement).value))}
						/>
					</div>
					<div>
						<span class="label">{t('flow.retries')} (0–10)</span>
						<input
							type="number"
							min="0"
							max="10"
							class="input font-mono"
							value={selectedNode.data.node.retries}
							onchange={(event) => setRetries(selectedNode, Number((event.currentTarget as HTMLInputElement).value))}
						/>
					</div>
				</div>
				<div class="grid grid-cols-2 gap-2">
					<div>
						<span class="label">{t('flow.retryDelay')}</span>
						<input
							type="number"
							min="0"
							max="30000"
							class="input font-mono"
							value={selectedNode.data.node.retry_delay_ms}
							onchange={(event) => setRetryDelay(selectedNode, Number((event.currentTarget as HTMLInputElement).value))}
						/>
					</div>
					<div>
						<span class="label">{t('flow.delayPolicy')}</span>
						<select class="input" bind:value={selectedNode.data.node.delay_policy} onchange={() => sync()}>
							<option value="none">none</option>
							<option value="fixed">fixed</option>
							<option value="linear">linear</option>
						</select>
					</div>
				</div>
				<div class="grid grid-cols-3 gap-2">
					<div>
						<span class="label">{t('flow.onSuccess')}</span>
						<select class="input" bind:value={selectedNode.data.node.on_success} onchange={() => sync()}>
							<option value="stop">stop</option>
							<option value="edge">edge</option>
							<option value="next">next</option>
						</select>
					</div>
					<div>
						<span class="label">{t('flow.onEmpty')}</span>
						<select class="input" bind:value={selectedNode.data.node.on_empty} onchange={() => sync()}>
							<option value="next">next</option>
							<option value="stop">stop</option>
							<option value="edge">edge</option>
						</select>
					</div>
					<div>
						<span class="label">{t('flow.onFail')}</span>
						<select class="input" bind:value={selectedNode.data.node.on_fail} onchange={() => sync()}>
							<option value="next">next</option>
							<option value="stop">stop</option>
							<option value="edge">edge</option>
						</select>
					</div>
				</div>
				<div>
					<div class="mb-1 flex items-center justify-between">
						<span class="label mb-0">{t('flow.params')}</span>
						<button class="btn px-2 py-0.5 text-xs" onclick={addParam}>+</button>
					</div>
					{#each selectedParams as [key, value] (key)}
						<div class="mb-1 flex items-center gap-1">
							<input class="input flex-1" value={key} readonly />
							<input
								class="input flex-1"
								{value}
								oninput={(event) => {
									selectedNode.data.node.params[key] = (event.currentTarget as HTMLInputElement).value;
									sync();
								}}
							/>
							<button
								class="btn px-2 py-0.5"
								onclick={() => {
									delete selectedNode.data.node.params[key];
									sync();
								}}>✕</button
							>
						</div>
					{/each}
				</div>
			</div>
		{:else}
			<p class="text-sm text-slate-500">{t('flow.selectHint')}</p>
		{/if}
	</aside>
</div>

<Confirm
	open={pendingMode !== null}
	message={t('flow.modeConfirm')}
	confirmLabel={t('common.apply')}
	onconfirm={() => pendingMode && applyMode(pendingMode)}
	oncancel={() => (pendingMode = null)}
/>
