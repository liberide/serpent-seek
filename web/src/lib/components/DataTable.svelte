<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n.svelte';

	type Column = { key: string; label: string; class?: string };

	let {
		columns = [],
		rows = [],
		empty = t('datatable.empty'),
		cell
	}: {
		columns?: Column[];
		rows?: Record<string, unknown>[];
		empty?: string;
		cell?: Snippet<[Record<string, unknown>]>;
	} = $props();
</script>

<div class="overflow-x-auto">
	<table class="table">
		<thead>
			<tr>
				{#each columns as column (column.key)}
					<th class={column.class}>{column.label}</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#if rows.length === 0}
				<tr>
					<td colspan={columns.length} class="py-6 text-center text-slate-500">{empty}</td>
				</tr>
			{/if}
			{#each rows as row, i (i)}
				<tr>
					{#if cell}
						{@render cell(row)}
					{:else}
						{#each columns as column (column.key)}
							<td>{row[column.key]}</td>
						{/each}
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>
</div>
