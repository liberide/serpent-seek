<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import type { DeveloperEndpoint } from '$lib/developer';

	// Reusable multi-select of API endpoints, grouped by OpenAPI tag. Used both
	// when creating a restricted token and when editing its permissions.
	let {
		endpoints = [],
		selected = $bindable<string[]>([])
	}: {
		endpoints: DeveloperEndpoint[];
		selected?: string[];
	} = $props();

	let query = $state('');

	const selectedSet = $derived(new Set(selected));

	const groups = $derived.by(() => {
		const map: Record<string, DeveloperEndpoint[]> = {};
		for (const ep of endpoints) {
			(map[ep.tag || 'other'] ??= []).push(ep);
		}
		return Object.entries(map).sort((a, b) => a[0].localeCompare(b[0]));
	});

	// Groups filtered by the search box (matches method, path and summary).
	const visible = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return groups;
		return groups
			.map(
				([tag, list]) =>
					[tag, list.filter((ep) => `${ep.id} ${ep.summary ?? ''}`.toLowerCase().includes(q))] as const
			)
			.filter(([, list]) => list.length > 0);
	});

	function toggle(id: string) {
		selected = selectedSet.has(id) ? selected.filter((item) => item !== id) : [...selected, id];
	}

	function toggleGroup(list: DeveloperEndpoint[]) {
		const ids = list.map((ep) => ep.id);
		const allSelected = ids.every((id) => selectedSet.has(id));
		selected = allSelected
			? selected.filter((id) => !ids.includes(id))
			: [...new Set([...selected, ...ids])];
	}

	// Colour-coded HTTP method badge, matching the Swagger UI palette.
	function methodClass(method: string): string {
		switch (method) {
			case 'GET':
				return 'bg-emerald-500/15 text-emerald-300';
			case 'POST':
				return 'bg-sky-500/15 text-sky-300';
			case 'PUT':
				return 'bg-amber-500/15 text-amber-300';
			case 'PATCH':
				return 'bg-violet-500/15 text-violet-300';
			case 'DELETE':
				return 'bg-rose-500/15 text-rose-300';
			default:
				return 'bg-slate-600/30 text-slate-300';
		}
	}
</script>

<div class="space-y-2">
	<div class="flex flex-wrap items-center gap-2">
		<input class="input min-w-40 flex-1" bind:value={query} placeholder={t('developer.permissionsSearch')} />
		<button class="btn text-xs" onclick={() => (selected = endpoints.map((ep) => ep.id))}>
			{t('developer.permissionsSelectAll')}
		</button>
		<button class="btn text-xs" onclick={() => (selected = [])}>{t('developer.permissionsClear')}</button>
	</div>
	<p class="text-xs text-slate-500">
		{t('developer.permissionsSelected', { n: selected.length, total: endpoints.length })}
	</p>
	<div class="max-h-72 space-y-3 overflow-y-auto rounded-lg border border-slate-800 bg-slate-950/40 p-2">
		{#each visible as [tag, list] (tag)}
			<div>
				<div class="mb-1 flex items-center justify-between gap-2">
					<p class="text-[11px] font-semibold uppercase tracking-wide text-slate-500">{tag}</p>
					<button
						class="text-[11px] text-emerald-300 transition hover:text-emerald-200"
						onclick={() => toggleGroup(list)}
					>
						{list.every((ep) => selectedSet.has(ep.id))
							? t('developer.permissionsClear')
							: t('developer.permissionsSelectAll')}
					</button>
				</div>
				<ul class="space-y-0.5">
					{#each list as ep (ep.id)}
						<li>
							<label class="flex items-start gap-2 rounded px-1.5 py-1 text-xs transition hover:bg-slate-800/50">
								<input
									type="checkbox"
									class="mt-0.5"
									checked={selectedSet.has(ep.id)}
									onchange={() => toggle(ep.id)}
								/>
								<span class="min-w-0">
									<span class="mr-1 rounded px-1 py-0.5 font-mono text-[10px] font-semibold {methodClass(ep.method)}">
										{ep.method}
									</span>
									<span class="font-mono text-slate-300">{ep.path}</span>
									{#if ep.summary}<span class="block text-slate-500">{ep.summary}</span>{/if}
								</span>
							</label>
						</li>
					{/each}
				</ul>
			</div>
		{/each}
	</div>
</div>
