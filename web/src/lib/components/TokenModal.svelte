<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import EndpointPicker from '$lib/components/EndpointPicker.svelte';
	import { t } from '$lib/i18n.svelte';
	import type { DeveloperEndpoint, DeveloperToken } from '$lib/developer';

	// Create/edit dialog for a developer token: name plus a clear choice between
	// full access and a custom endpoint permission list.
	let {
		open = false,
		token = null,
		endpoints = [],
		saving = false,
		onsave,
		onclose
	}: {
		open?: boolean;
		token?: DeveloperToken | null;
		endpoints: DeveloperEndpoint[];
		saving?: boolean;
		onsave: (payload: { name: string; fullAccess: boolean; permissions: string[] }) => void;
		onclose: () => void;
	} = $props();

	let name = $state('');
	let fullAccess = $state(true);
	let selected = $state<string[]>([]);

	const isEdit = $derived(token !== null);

	// Seed the form whenever the dialog opens or the target token changes.
	$effect(() => {
		if (!open) return;
		name = token?.name ?? '';
		fullAccess = token ? token.all_endpoints : true;
		selected = token ? [...token.permissions] : [];
	});
</script>

<Modal
	{open}
	title={isEdit ? t('developer.editTokenTitle') : t('developer.createTokenTitle')}
	{onclose}
	wide
>
	<div class="space-y-4">
		<div>
			<span class="label">{t('developer.tokenName')}</span>
			<input class="input" bind:value={name} placeholder={t('developer.tokenNamePlaceholder')} />
		</div>

		<div>
			<span class="label">{t('developer.permissions')}</span>
			<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
				<button
					type="button"
					class="rounded-lg border p-3 text-left transition {fullAccess
						? 'border-emerald-600 bg-emerald-950/30'
						: 'border-slate-700 bg-slate-900 hover:border-slate-500'}"
					aria-pressed={fullAccess}
					onclick={() => (fullAccess = true)}
				>
					<div class="flex items-center gap-2 text-sm font-semibold">
						<span aria-hidden="true">🔓</span>{t('developer.permissionsFull')}
						{#if fullAccess}<span class="ml-auto text-emerald-400">✓</span>{/if}
					</div>
					<p class="mt-1 text-xs text-slate-400">{t('developer.permissionsFullHint')}</p>
				</button>
				<button
					type="button"
					class="rounded-lg border p-3 text-left transition {!fullAccess
						? 'border-emerald-600 bg-emerald-950/30'
						: 'border-slate-700 bg-slate-900 hover:border-slate-500'}"
					aria-pressed={!fullAccess}
					onclick={() => (fullAccess = false)}
				>
					<div class="flex items-center gap-2 text-sm font-semibold">
						<span aria-hidden="true">🔐</span>{t('developer.permissionsRestricted')}
						{#if !fullAccess}<span class="ml-auto text-emerald-400">✓</span>{/if}
					</div>
					<p class="mt-1 text-xs text-slate-400">{t('developer.permissionsRestrictedHint')}</p>
				</button>
			</div>
		</div>

		{#if !fullAccess}
			<EndpointPicker {endpoints} bind:selected />
		{/if}
	</div>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button class="btn" onclick={onclose} disabled={saving}>{t('common.cancel')}</button>
			<button
				class="btn btn-primary"
				onclick={() => onsave({ name, fullAccess, permissions: selected })}
				disabled={saving}
			>
				{saving ? t('common.saving') : isEdit ? t('common.save') : t('developer.generate')}
			</button>
		</div>
	{/snippet}
</Modal>
