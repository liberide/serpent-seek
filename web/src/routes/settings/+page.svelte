<script lang="ts">
	import { onMount } from 'svelte';
	import { get, post, put } from '$lib/api';
	import { notify, authOffNoticeHidden } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { t } from '$lib/i18n.svelte';

	type Descriptor = {
		key: string;
		env: string;
		type: string;
		default: string;
		group: string;
		secret: boolean;
		env_only: boolean;
	};

	type Section = {
		id: string;
		labelKey: string;
		items: Descriptor[];
	};

	let descriptors = $state<Descriptor[]>([]);
	let values = $state<Record<string, string>>({});
	let env = $state<Record<string, boolean>>({});
	let secretSet = $state<Record<string, boolean>>({});
	let storage = $state<any>({});
	let jobs = $state<Record<string, any>>({});
	let secrets = $state<Record<string, string>>({});
	let saving = $state(false);
	let active = $state('general');
	let confirmAction = $state<'clear-logs' | 'clear-history' | null>(null);

	const groupLabels: Record<string, string> = {
		general: 'settings.groups.general',
		auth: 'settings.groups.auth',
		network: 'settings.groups.network',
		providers: 'settings.groups.providers',
		retention: 'settings.groups.retention',
		storage: 'settings.groups.storage'
	};

	// Fixed section order; unknown descriptor groups are inserted before the
	// static "maintenance"/"danger" sections.
	const fixed = ['general', 'auth', 'network', 'providers', 'retention', 'storage'];

	// Localized label/description for each setting, falling back to the raw key/env
	// so settings without a translation still render.
	function fieldLabel(d: Descriptor): string {
		const key = `settings.fields.${d.key}`;
		const label = t(key);
		return label === key ? d.key : label;
	}

	function fieldHint(d: Descriptor): string {
		const key = `settings.fieldHints.${d.key}`;
		const hint = t(key);
		return hint === key ? d.env : hint;
	}

	// A setting is read-only when the environment pins it. auth_enabled is
	// special only on the backend: AUTH_ENABLED=false is a default (not a pin),
	// so it is reported without the env marker and stays editable.
	function isLocked(descriptor: Descriptor): boolean {
		return env[descriptor.key] || descriptor.env_only;
	}

	async function load() {
		const data = await get<any>('/api/settings');
		descriptors = data.descriptors;
		values = data.values;
		env = data.env;
		secretSet = data.secret_set;
		storage = data.storage;
		const maintenance = await get<any>('/api/maintenance/status');
		jobs = maintenance.jobs;
	}

	onMount(async () => {
		await load();
		const hash = window.location.hash.replace('#', '');
		if (hash && sections.some((s) => s.id === hash)) active = hash;
	});

	const grouped = $derived.by(() => {
		const map: Record<string, Descriptor[]> = {};
		for (const descriptor of descriptors) {
			(map[descriptor.group] ??= []).push(descriptor);
		}
		return map;
	});

	// The settings menu: one section per descriptor group (in the fixed order),
	// then the static maintenance/danger sections. Only the active one renders.
	const sections = $derived.by(() => {
		const known = new Set(fixed);
		const extra = Object.keys(grouped).filter((g) => !known.has(g));
		const settingSections: Section[] = [...fixed, ...extra]
			.filter((g) => (grouped[g]?.length ?? 0) > 0)
			.map((g) => ({ id: g, labelKey: groupLabels[g] ?? g, items: grouped[g] }));
		return [
			...settingSections,
			{ id: 'maintenance', labelKey: 'settings.groups.maintenance', items: [] },
			{ id: 'danger', labelKey: 'settings.groups.danger', items: [] }
		];
	});

	function selectSection(id: string) {
		active = id;
		if (typeof window !== 'undefined') {
			history.replaceState(null, '', `#${id}`);
		}
	}

	async function save() {
		saving = true;
		try {
			const payload: Record<string, string> = { ...values };
			for (const [key, value] of Object.entries(secrets)) {
				if (value) payload[key] = value;
			}
			await put('/api/settings', payload);
			notify(t('settings.saved'), 'success');
			secrets = {};
			await load();
			// Keep the sidebar warning in sync without a page reload.
			authOffNoticeHidden.set(values.hide_auth_off_notice === 'true');
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			saving = false;
		}
	}

	async function testStorage() {
		const result = await post<any>('/api/settings/storage/test');
		notify(
			result.ok ? t('settings.storageOk', { driver: result.driver }) : t('settings.storageError', { error: result.error }),
			result.ok ? 'success' : 'error'
		);
	}

	function maintenanceSummary(path: string, result: any): string {
		if (path === 'run-cleanup') {
			return t('settings.cleanupDone', {
				requests: result.requests ?? 0,
				logs: result.logs ?? 0,
				sessions: result.sessions ?? 0,
				ms: result.ms ?? 0
			});
		}
		if (path === 'vacuum-now') {
			return t('settings.vacuumDone', { driver: result.driver ?? '' });
		}
		if (typeof result.deleted === 'number') {
			return t('settings.deleted', { n: result.deleted });
		}
		return JSON.stringify(result);
	}

	async function maintenance(path: string) {
		try {
			const result = await post<any>(`/api/maintenance/${path}`);
			notify(maintenanceSummary(path, result), 'success');
			confirmAction = null;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}
</script>

{#snippet control(descriptor: Descriptor)}
	{#if descriptor.type === 'bool'}
		<button
			type="button"
			role="switch"
			aria-checked={values[descriptor.key] === 'true'}
			aria-label={fieldLabel(descriptor)}
			class="relative h-6 w-11 rounded-full transition {(values[descriptor.key] === 'true') ? 'bg-emerald-600' : 'bg-slate-700'} disabled:cursor-not-allowed disabled:opacity-50"
			disabled={isLocked(descriptor)}
			onclick={() => (values[descriptor.key] = values[descriptor.key] === 'true' ? 'false' : 'true')}
		>
			<span
				class="absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all {values[descriptor.key] === 'true'
					? 'left-[22px]'
					: 'left-0.5'}"
			></span>
		</button>
	{:else if descriptor.secret}
		<input
			type="password"
			class="input font-mono text-xs"
			bind:value={secrets[descriptor.key]}
			placeholder={secretSet[descriptor.key] ? t('providers.secretSaved') : ''}
			disabled={isLocked(descriptor)}
		/>
	{:else if descriptor.key === 'log_level'}
		<select class="input" bind:value={values[descriptor.key]} disabled={isLocked(descriptor)}>
			<option value="debug">debug</option>
			<option value="info">info</option>
			<option value="warn">warn</option>
			<option value="error">error</option>
		</select>
	{:else}
		<input class="input" bind:value={values[descriptor.key]} disabled={isLocked(descriptor)} />
	{/if}
{/snippet}

<div class="space-y-4">
	<div class="flex items-center justify-between gap-2">
		<h1 class="text-xl font-semibold">{t('settings.title')}</h1>
		<button class="btn btn-primary" onclick={save} disabled={saving}>
			{saving ? t('common.saving') : t('common.save')}
		</button>
	</div>

	<!-- mobile section picker -->
	<div class="lg:hidden">
		<span class="label">{t('settings.section')}</span>
		<select class="input" value={active} onchange={(e) => selectSection((e.currentTarget as HTMLSelectElement).value)}>
			{#each sections as section (section.id)}
				<option value={section.id}>{t(section.labelKey)}</option>
			{/each}
		</select>
	</div>

	<div class="grid grid-cols-1 gap-4 lg:grid-cols-[230px_1fr]">
		<!-- section menu: one entry per settings group (tab behaviour) -->
		<nav class="sticky top-4 hidden h-fit lg:block" aria-label={t('settings.section')}>
			<ul class="space-y-1">
				{#each sections as section (section.id)}
					<li>
						<button
							class="w-full rounded-lg px-3 py-1.5 text-left text-sm transition {active === section.id
								? 'bg-slate-800 text-emerald-300'
								: 'text-slate-400 hover:bg-slate-800/60'}"
							onclick={() => selectSection(section.id)}
						>
							{t(section.labelKey)}
						</button>
					</li>
				{/each}
			</ul>
		</nav>

		<!-- active section only -->
		{#each sections as section (section.id)}
			{#if active === section.id}
				<div class="space-y-3">
					<h2 class="px-1 text-sm font-semibold uppercase tracking-wide text-slate-400">
						{t(section.labelKey)}
					</h2>

					{#if section.id === 'maintenance'}
						<div class="card">
							<div class="flex flex-wrap gap-2">
								<button class="btn" onclick={() => maintenance('run-cleanup')}>{t('settings.runCleanup')}</button>
								<button class="btn" onclick={() => maintenance('vacuum-now')}>{t('settings.vacuum')}</button>
							</div>
							<div class="mt-3 text-xs text-slate-500">
								{#each Object.entries(jobs) as [name, info] (name)}
									<p>
										{name}: {info.last_run ? t('settings.lastRun', { time: info.last_run }) : t('settings.neverRun')}{info.last_error
											? ' · ' + t('settings.lastError', { error: info.last_error })
											: ''}
									</p>
								{/each}
							</div>
						</div>
					{:else if section.id === 'danger'}
						<div class="card">
							<div class="rounded-lg border border-rose-800/60 bg-rose-950/20 p-3">
								<p class="mb-3 text-xs text-rose-200">{t('settings.dangerHint')}</p>
								<div class="flex flex-wrap gap-2">
									<button class="btn btn-danger" onclick={() => (confirmAction = 'clear-logs')}>
										{t('settings.clearLogs')}
									</button>
									<button class="btn btn-danger" onclick={() => (confirmAction = 'clear-history')}>
										{t('settings.clearHistory')}
									</button>
								</div>
							</div>
						</div>
					{:else}
						<!-- each setting is its own separate card -->
						{#each section.items as descriptor (descriptor.key)}
							<section class="card">
								<div class="mb-2 flex flex-wrap items-center gap-2">
									<h3 class="text-sm font-semibold">{fieldLabel(descriptor)}</h3>
									{#if env[descriptor.key] || descriptor.env_only}
										<span class="badge bg-amber-500/15 text-amber-300">{t('settings.env')}</span>
									{/if}
									{#if descriptor.secret && secretSet[descriptor.key]}
										<span class="text-xs text-emerald-400">{t('settings.set')}</span>
									{/if}
								</div>
								<div class="max-w-xl">
									{@render control(descriptor)}
								</div>
								<p class="mt-1 text-xs text-slate-500">{fieldHint(descriptor)}</p>
								{#if descriptor.env}
									<p class="mt-0.5 font-mono text-[10px] text-slate-600">{descriptor.env}</p>
								{/if}
							</section>
						{/each}

						{#if section.id === 'storage'}
							<div class="card">
								<p class="text-sm">
									{t('settings.driver')} <span class="font-mono">{storage.driver}</span>
								</p>
								<p class="text-xs text-slate-500">{t('settings.sqlite')} {storage.sqlite_path}</p>
								<p class="mt-2 text-xs text-amber-300">{t('settings.storageWarning')}</p>
								<button class="btn mt-2" onclick={testStorage}>{t('settings.testStorage')}</button>
							</div>
						{:else if section.id === 'retention'}
							<div class="card">
								<p class="mb-2 text-xs text-slate-500">{t('settings.cronHint')}</p>
								<button class="btn" onclick={() => maintenance('run-cleanup')}>{t('settings.runCleanupNow')}</button>
							</div>
						{/if}
					{/if}
				</div>
			{/if}
		{/each}
	</div>
</div>

<Confirm
	open={confirmAction !== null}
	message={confirmAction === 'clear-logs' ? t('settings.clearLogsConfirm') : t('settings.clearHistoryConfirm')}
	confirmLabel={t('common.delete')}
	onconfirm={() => maintenance(confirmAction!)}
	oncancel={() => (confirmAction = null)}
/>
