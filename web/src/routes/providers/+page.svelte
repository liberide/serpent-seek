<script lang="ts">
	import { onMount } from 'svelte';
	import { del, get, patch, post, put } from '$lib/api';
	import { notify } from '$lib/stores';
	import Modal from '$lib/components/Modal.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import SecretField from '$lib/components/SecretField.svelte';
	import UrlField from '$lib/components/UrlField.svelte';
	import { t } from '$lib/i18n.svelte';

	type Provider = {
		id: string;
		code: string;
		name: string;
		enabled: boolean;
		base_url: string;
		params: Record<string, string>;
		credentials_set: Record<string, boolean>;
	};

	type ParamOption = { label: string; value: string };
	type ParamField = {
		key: string;
		label: string;
		type: 'text' | 'number' | 'select' | 'boolean' | 'textarea';
		default?: string;
		options?: ParamOption[];
		hint?: string;
		required?: boolean;
	};
	type CredentialField = { key: string; multiline?: boolean };
	type DriverSchema = {
		code: string;
		name: string;
		deprecated: boolean;
		default_base_url?: string;
		credentials: CredentialField[];
		params: ParamField[];
		hints?: string[];
	};

	let providers = $state<Provider[]>([]);
	let driverSchemas = $state<DriverSchema[]>([]);
	let loading = $state(true);
	let loadingDrivers = $state(true);

	let editing = $state<Provider | null>(null);
	let creating = $state(false);
	let form = $state({ code: '', name: '', enabled: true, base_url: '' });
	let credentials = $state<Record<string, string>>({});
	let paramValues = $state<Record<string, string>>({});
	let paramsText = $state('{}');
	let rawJsonMode = $state(false);

	let deleteTarget = $state<Provider | null>(null);
	let deleteConflict = $state<string[]>([]);
	let testResults = $state<Record<string, any>>({});
	let testing = $state<Record<string, boolean>>({});

	const selectedSchema = $derived<DriverSchema | undefined>(
		driverSchemas.find((d) => d.code === form.code)
	);

	const baseUrlPlaceholder = $derived(selectedSchema?.default_base_url ?? '');
	const baseUrlHint = $derived(
		selectedSchema?.default_base_url
			? t('providers.baseUrlOverrideHint')
			: t('providers.baseUrlRequiredHint')
	);

	const sortedDrivers = $derived(
		driverSchemas
			.slice()
			.sort((a, b) => a.name.localeCompare(b.name))
			.map((d) => ({ code: d.code, name: d.name, deprecated: d.deprecated }))
	);

	async function load() {
		loading = true;
		try {
			providers = await get<Provider[]>('/api/providers');
		} finally {
			loading = false;
		}
	}

	async function loadDrivers() {
		loadingDrivers = true;
		try {
			driverSchemas = await get<DriverSchema[]>('/api/providers/drivers');
			if (driverSchemas.length > 0 && form.code === '') {
				form.code = driverSchemas[0].code;
				resetParams();
			}
		} finally {
			loadingDrivers = false;
		}
	}

	onMount(() => {
		load();
		loadDrivers();
	});

	function emptyCredentials(): Record<string, string> {
		const out: Record<string, string> = {};
		for (const d of driverSchemas) {
			for (const c of d.credentials) {
				out[c.key] = '';
			}
		}
		return out;
	}

	function defaultParams(schema?: DriverSchema): Record<string, string> {
		const out: Record<string, string> = {};
		if (!schema) return out;
		for (const p of schema.params) {
			if (p.default !== undefined && p.default !== '') {
				out[p.key] = p.default;
			}
		}
		return out;
	}

	function syncParamsText() {
		paramsText = JSON.stringify(paramValues, null, 2);
	}

	function openCreate() {
		creating = true;
		editing = null;
		form = { code: sortedDrivers[0]?.code ?? '', name: '', enabled: true, base_url: '' };
		credentials = emptyCredentials();
		paramValues = defaultParams(selectedSchema);
		rawJsonMode = false;
		syncParamsText();
	}

	function openEdit(provider: Provider) {
		creating = false;
		editing = $state.snapshot(provider) as Provider;
		form = { code: provider.code, name: provider.name, enabled: provider.enabled, base_url: provider.base_url };
		credentials = emptyCredentials();
		paramValues = { ...defaultParams(selectedSchema), ...(provider.params ?? {}) };
		rawJsonMode = false;
		syncParamsText();
	}

	function resetParams() {
		paramValues = defaultParams(selectedSchema);
		syncParamsText();
	}

	function onParamChange(key: string, value: string) {
		paramValues = { ...paramValues, [key]: value };
		syncParamsText();
	}

	function onFormCodeChange(code: string) {
		form = { ...form, code };
		paramValues = defaultParams(selectedSchema);
		syncParamsText();
	}

	function parseParamsText(): Record<string, string> | null {
		try {
			return JSON.parse(paramsText || '{}');
		} catch {
			return null;
		}
	}

	async function save() {
		let params = parseParamsText();
		if (params === null) {
			notify(t('providers.badParams'), 'error');
			return;
		}
		const missing = (selectedSchema?.params ?? []).filter(
			(p) => p.required && !String(params?.[p.key] ?? '').trim()
		);
		if (missing.length > 0) {
			notify(t('providers.missingParams', { fields: missing.map((m) => m.label).join(', ') }), 'error');
			return;
		}
		try {
			if (creating) {
				await post('/api/providers', {
					code: form.code,
					name: form.name,
					enabled: form.enabled,
					base_url: form.base_url,
					params,
					credentials
				});
				notify(t('providers.added'), 'success');
			} else if (editing) {
				await put(`/api/providers/${editing.id}`, {
					name: form.name,
					enabled: form.enabled,
					base_url: form.base_url,
					params,
					credentials
				});
				notify(t('providers.saved'), 'success');
			}
			creating = false;
			editing = null;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function toggle(provider: Provider) {
		try {
			await patch(`/api/providers/${provider.id}`, { enabled: !provider.enabled });
			provider.enabled = !provider.enabled;
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function testConnection(provider: Provider) {
		testing = { ...testing, [provider.id]: true };
		try {
			testResults = { ...testResults, [provider.id]: await post(`/api/providers/${provider.id}/test`) };
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			testing = { ...testing, [provider.id]: false };
		}
	}

	async function remove() {
		if (!deleteTarget) return;
		try {
			await del(`/api/providers/${deleteTarget.id}`);
			notify(t('providers.deleted'), 'success');
			deleteTarget = null;
			deleteConflict = [];
			await load();
		} catch (error) {
			const message = (error as Error).message;
			deleteConflict = message ? [message] : [];
			notify(message, 'error');
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold">{t('providers.title')}</h1>
		<div class="flex gap-2">
			<button class="btn" onclick={load}>{t('common.refresh')}</button>
			<button class="btn btn-primary" onclick={openCreate}>{t('providers.add')}</button>
		</div>
	</div>
	<p class="text-xs text-slate-500">
		{t('providers.hint')}
	</p>

	{#if loading}
		<p class="text-sm text-slate-500">{t('common.loading')}</p>
	{:else}
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
			{#each providers as provider (provider.id)}
				<div class="card" class:opacity-60={!provider.enabled}>
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								<h2 class="truncate font-semibold">{provider.name}</h2>
								<span class="badge bg-slate-700/40 text-slate-300">{provider.code}</span>
								{#if driverSchemas.find((d) => d.code === provider.code)?.deprecated}
									<span class="badge bg-amber-500/15 text-amber-300">{t('providers.deprecated')}</span>
								{/if}
							</div>
							<p class="mt-1 truncate font-mono text-xs text-slate-400">{provider.base_url || '—'}</p>
						</div>
						<button
							class="relative h-6 w-11 shrink-0 rounded-full transition {provider.enabled ? 'bg-emerald-600' : 'bg-slate-700'}"
							role="switch"
							aria-checked={provider.enabled}
							aria-label={provider.enabled ? t('providers.disable') : t('providers.enable')}
							onclick={() => toggle(provider)}
						>
							<span
								class="absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all {provider.enabled
									? 'left-[22px]'
									: 'left-0.5'}"
							></span>
						</button>
					</div>

					{#if provider.code === 'searxng'}
						<p class="mt-1 text-xs text-slate-500">
							{t('providers.searxngHint')}
						</p>
					{/if}
					{#if provider.code === 'apiserpent'}
						<p class="mt-1 text-xs text-slate-500">
							{t('providers.enginePerInstance')}
						</p>
					{/if}

					<div class="mt-3 flex flex-wrap gap-2">
						<button class="btn text-xs" onclick={() => openEdit(provider)}>{t('providers.configure')}</button>
						<button
							class="btn text-xs"
							onclick={() => testConnection(provider)}
							disabled={testing[provider.id]}
						>
							{testing[provider.id] ? t('providers.testing') : t('providers.test')}
						</button>
						<button
							class="btn btn-danger text-xs"
							onclick={() => {
								deleteTarget = provider;
								deleteConflict = [];
							}}>{t('common.delete')}</button
						>
					</div>

					{#if testResults[provider.id]}
						<div class="mt-2 rounded-lg border border-slate-800 bg-slate-950/60 p-2 text-xs">
							<span class="text-slate-300">
								kind={testResults[provider.id].kind} · http={testResults[provider.id].http} · {testResults[
									provider.id
								].took_ms}ms
							</span>
							{#if testResults[provider.id].error}
								<p class="mt-1 break-all text-rose-300">{testResults[provider.id].error}</p>
							{/if}
							{#if testResults[provider.id].answer}
								<p class="mt-1 whitespace-pre-wrap break-words rounded bg-slate-800/60 p-2 text-slate-300">
									<span class="font-semibold">{t('providers.answer')}:</span> {testResults[provider.id].answer}
								</p>
							{/if}
							<ul class="mt-1 space-y-0.5 text-slate-400">
								{#each testResults[provider.id].results ?? [] as row (row.link)}
									<li class="truncate">• {row.title} — {row.link}</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{/each}
			{#if providers.length === 0}
				<div class="card text-sm text-slate-500">
					{t('providers.empty')}
				</div>
			{/if}
		</div>
	{/if}
</div>

<Modal
	open={creating || editing !== null}
	title={creating ? t('providers.newTitle') : t('providers.editTitle', { name: editing?.name ?? '' })}
	onclose={() => {
		creating = false;
		editing = null;
	}}
>
	{#if loadingDrivers}
		<p class="text-sm text-slate-500">{t('common.loading')}</p>
	{:else}
		<div class="space-y-3">
			{#if creating}
				<div>
					<span class="label">{t('providers.driverType')}</span>
					<select class="input" value={form.code} onchange={(e) => onFormCodeChange((e.target as HTMLSelectElement).value)}>
						{#each sortedDrivers as driver (driver.code)}
							<option value={driver.code}>
								{driver.name}{#if driver.deprecated} ({t('providers.deprecated')}){/if}
							</option>
						{/each}
					</select>
				</div>
			{:else}
				<p class="text-xs text-slate-500">
					{t('providers.driver')} <span class="font-mono">{form.code}</span> {t('providers.driverFixed')}
				</p>
			{/if}
			<div>
				<span class="label">{t('providers.uniqueName')}</span>
				<input class="input" bind:value={form.name} placeholder={t('providers.namePlaceholder')} />
			</div>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={form.enabled} /> {t('providers.enabledLabel')}
			</label>
			<UrlField
				label={t('providers.baseUrl')}
				bind:value={form.base_url}
				placeholder={baseUrlPlaceholder}
				hint={baseUrlHint}
			/>

			{#each selectedSchema?.credentials ?? [] as field (field.key)}
				<SecretField
					label={field.key}
					multiline={field.multiline ?? false}
					alreadySet={!!(editing?.credentials_set?.[field.key])}
					bind:value={credentials[field.key]}
					placeholder={editing?.credentials_set?.[field.key] ? t('providers.secretSaved') : t('providers.secretEmpty')}
				/>
			{/each}

			<div>
				<div class="mb-1 flex items-center justify-between">
					<span class="label mb-0">{t('providers.params')}</span>
					<div class="flex gap-2">
						<button class="btn px-2 py-0.5 text-xs" onclick={() => (rawJsonMode = !rawJsonMode)}>
							{rawJsonMode ? 'Form' : 'JSON'}
						</button>
						<button class="btn px-2 py-0.5 text-xs" onclick={resetParams}>{t('providers.resetJson')}</button>
					</div>
				</div>

				{#if rawJsonMode}
					<textarea class="input h-28 font-mono text-xs" bind:value={paramsText}></textarea>
				{:else}
					{#each selectedSchema?.params ?? [] as field (field.key)}
						{@const current = paramValues[field.key] ?? ''}
						<div class="mb-2">
							<span class="label">{field.label}{#if field.required}<span class="text-rose-400"> *</span>{/if}</span>
							{#if field.type === 'select'}
								{@const known = (field.options ?? []).some((o) => o.value === current)}
								<select
									class="input"
									value={current}
									required={field.required ?? false}
									onchange={(e) => onParamChange(field.key, (e.currentTarget as HTMLSelectElement).value)}
								>
									{#if !known}
										<option value={current} disabled>
											{current
												? t('providers.customValue', { value: current })
												: t('providers.selectPlaceholder')}
										</option>
									{/if}
									{#each field.options ?? [] as opt (opt.value)}
										<option value={opt.value}>{opt.label}</option>
									{/each}
								</select>
							{:else if field.type === 'boolean'}
								<label class="flex items-center gap-2 text-sm">
									<input
										type="checkbox"
										checked={current === 'true'}
										onchange={(e) =>
											onParamChange(field.key, String((e.currentTarget as HTMLInputElement).checked))}
									/>
									{current === 'true' ? 'On' : 'Off'}
								</label>
							{:else if field.type === 'textarea'}
								<textarea
									class="input h-20 font-mono text-xs"
									value={current}
									required={field.required ?? false}
									placeholder={field.hint ?? ''}
									oninput={(e) => onParamChange(field.key, (e.currentTarget as HTMLTextAreaElement).value)}
								></textarea>
							{:else}
								<input
									type={field.type === 'number' ? 'number' : 'text'}
									class="input"
									value={current}
									required={field.required ?? false}
									placeholder={field.hint ?? ''}
									oninput={(e) => onParamChange(field.key, (e.currentTarget as HTMLInputElement).value)}
								/>
							{/if}
							{#if field.hint}
								<p class="text-[11px] text-slate-500">{field.hint}</p>
							{/if}
						</div>
					{/each}
				{/if}

				{#if selectedSchema?.hints?.length}
					<div class="mt-2 rounded-lg border border-slate-800 bg-slate-950/60 p-2 text-[11px] text-slate-400">
						{#each selectedSchema.hints as hint (hint)}
							<p>• {hint}</p>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}
	{#snippet footer()}
		{#if !loadingDrivers}
			<div class="flex justify-end gap-2">
				<button
					class="btn"
					onclick={() => {
						creating = false;
						editing = null;
					}}>{t('common.cancel')}</button
				>
				<button class="btn btn-primary" onclick={save}>{t('common.save')}</button>
			</div>
		{/if}
	{/snippet}
</Modal>

<Confirm
	open={deleteTarget !== null}
	title={t('providers.deleteTitle')}
	message={deleteConflict.length
		? t('providers.inUse', { chains: deleteConflict.join('; ') })
		: t('providers.deleteConfirm', { name: deleteTarget?.name ?? '' })}
	confirmLabel={deleteConflict.length ? t('providers.understood') : t('common.delete')}
	onconfirm={() => {
		if (deleteConflict.length) {
			deleteTarget = null;
			deleteConflict = [];
			return;
		}
		remove();
	}}
	oncancel={() => {
		deleteTarget = null;
		deleteConflict = [];
	}}
/>
