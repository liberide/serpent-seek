<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { page } from '$app/state';
	import { get, post, put } from '$lib/api';
	import { notify } from '$lib/stores';
	import FlowEditor from '$lib/components/FlowEditor.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { t } from '$lib/i18n.svelte';

	type ChainNode = {
		key: string;
		provider_id: string;
		provider?: string;
		label?: string;
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
	type Chain = {
		id: string;
		name: string;
		active: boolean;
		mode?: string;
		version: number;
		nodes: ChainNode[];
		edges: any[];
	};

	const id = $derived(page.params.id);
	let chain = $state<Chain | null>(null);
	let draft = $state<Chain | null>(null);
	type ProviderInstance = { id: string; code: string; name: string; enabled: boolean };
	let providers = $state<ProviderInstance[]>([]);
	let errors = $state<string[]>([]);
	let saving = $state(false);
	let dirty = $state(false);
	let maxAttempts = $state(10);
	let autosaveTimer: ReturnType<typeof setTimeout> | undefined;

	onMount(async () => {
		try {
			chain = await get<Chain>(`/api/chains/${id}`);
			draft = $state.snapshot(chain) as Chain;
			const providerList = await get<any[]>('/api/providers');
			providers = providerList.map((p) => ({
				id: p.id,
				code: p.code,
				name: p.name,
				enabled: p.enabled
			}));
			const settings = await get<any>('/api/settings').catch(() => null);
			const attempts = Number(settings?.values?.max_attempts);
			if (attempts > 0) maxAttempts = attempts;
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	});

	onDestroy(() => clearTimeout(autosaveTimer));

	function onEditorChange(next: any) {
		draft = next;
		dirty = true;
		clearTimeout(autosaveTimer);
		autosaveTimer = setTimeout(save, 1200);
	}

	async function save() {
		if (!draft) return;
		clearTimeout(autosaveTimer);
		saving = true;
		try {
			const saved = await put<Chain>(`/api/chains/${id}`, draft);
			chain = saved;
			draft = { ...draft, version: saved.version };
			dirty = false;
			errors = [];
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			saving = false;
		}
	}

	async function validate() {
		if (!draft) return;
		try {
			const result = await post<any>('/api/chains/validate', draft);
			errors = result.errors ?? [];
			if (result.ok) notify(t('chain.valid'), 'success');
			else notify(t('chain.hasErrors'), 'error');
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function activate() {
		try {
			await post(`/api/chains/${id}/activate`);
			if (chain) chain = { ...chain, active: true };
			notify(t('chains.activated'), 'success');
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div class="flex items-center gap-2">
			<a class="btn text-xs" href="/chains">{t('chain.backChains')}</a>
			{#if draft}
				<input class="input max-w-xs" bind:value={draft.name} oninput={() => { dirty = true; }} />
			{/if}
			{#if chain?.active}<StatusBadge status="ok" label={t('common.active')} />{/if}
		</div>
		<div class="flex gap-2">
			<button class="btn" onclick={validate}>{t('chain.validate')}</button>
			<button class="btn btn-primary" onclick={save} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</button>
			{#if !chain?.active}<button class="btn" onclick={activate}>{t('chains.makeActive')}</button>{/if}
		</div>
	</div>
	{#if draft}
		<FlowEditor chain={draft} {providers} {errors} {dirty} {maxAttempts} onchange={onEditorChange} />
	{:else}
		<p class="text-sm text-slate-500">{t('common.loading')}</p>
	{/if}
	{#if dirty}<p class="text-xs text-amber-300">{t('chain.dirty')}</p>{/if}
</div>
