<script lang="ts">
	import { onMount } from 'svelte';
	import { del, get, patch, post, put } from '$lib/api';
	import { notify, session } from '$lib/stores';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import TokenModal from '$lib/components/TokenModal.svelte';
	import { dateTime } from '$lib/format';
	import { t } from '$lib/i18n.svelte';
	import { type DeveloperEndpoint, type DeveloperStatus, type DeveloperToken } from '$lib/developer';

	// modeOnly renders only the enable/disable switch and is used inside Settings.
	// The standalone Developer section shows token management, the API reference
	// and a read-only mode indicator — toggling the mode lives in Settings.
	let { modeOnly = false }: { modeOnly?: boolean } = $props();

	let status = $state<DeveloperStatus | null>(null);
	let endpoints = $state<DeveloperEndpoint[]>([]);
	let loading = $state(true);
	let savingMode = $state(false);

	// Create/edit dialog. token === null means "create".
	let tokenModalOpen = $state(false);
	let tokenTarget = $state<DeveloperToken | null>(null);
	let tokenSaving = $state(false);

	// One-time plaintext reveal after creating a token.
	let createdToken = $state('');
	let createdOpen = $state(false);

	// activeTokens drives the small counter badge next to the table heading.
	const activeTokens = $derived(status ? status.tokens.filter((token) => !token.revoked_at).length : 0);

	onMount(async () => {
		await load();
		if (!modeOnly) await loadEndpoints();
	});

	async function load() {
		loading = true;
		try {
			status = await get<DeveloperStatus>('/api/developer');
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			loading = false;
		}
	}

	async function loadEndpoints() {
		try {
			endpoints = await get<DeveloperEndpoint[]>('/api/developer/endpoints');
		} catch {
			endpoints = [];
		}
	}

	async function toggleMode() {
		if (!status || status.env_locked) return;
		savingMode = true;
		try {
			const updated = await put<DeveloperStatus>('/api/developer/mode', { enabled: !status.enabled });
			status = updated;
			// Keep the sidebar in sync: the section is hidden while the mode is off.
			session.update((s) => (s ? { ...s, developer_mode: updated.enabled } : s));
			notify(updated.enabled ? t('developer.modeOn') : t('developer.modeOff'), 'success');
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			savingMode = false;
		}
	}

	function openCreate() {
		tokenTarget = null;
		tokenModalOpen = true;
	}

	function openEdit(token: DeveloperToken) {
		tokenTarget = token;
		tokenModalOpen = true;
	}

	function closeTokenModal() {
		if (tokenSaving) return;
		tokenModalOpen = false;
		tokenTarget = null;
	}

	async function submitToken(payload: { name: string; fullAccess: boolean; permissions: string[] }) {
		tokenSaving = true;
		try {
			const body = {
				name: payload.name || 'developer token',
				// ["*"] is the explicit full-access wildcard.
				permissions: payload.fullAccess ? ['*'] : payload.permissions
			};
			if (tokenTarget) {
				await patch(`/api/developer/tokens/${tokenTarget.id}`, body);
				notify(t('developer.permissionsSaved'), 'success');
			} else {
				const result = await post<{ token: DeveloperToken; plaintext: string }>(
					'/api/developer/tokens',
					body
				);
				createdToken = result.plaintext;
				createdOpen = true;
				notify(t('developer.createdOk'), 'success');
			}
			tokenModalOpen = false;
			tokenTarget = null;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			tokenSaving = false;
		}
	}

	function dismissCreated() {
		createdOpen = false;
		createdToken = '';
	}

	async function revoke(token: DeveloperToken) {
		try {
			await del(`/api/developer/tokens/${token.id}`);
			notify(t('developer.revokedOk'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	// remove deletes a (typically already revoked) token permanently.
	async function remove(token: DeveloperToken) {
		try {
			await del(`/api/developer/tokens/${token.id}?purge=true`);
			notify(t('developer.deletedOk'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}
</script>

<div class="space-y-4">
	<!-- Developer mode ---------------------------------------------------- -->
	<section class="card">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex items-start gap-3">
				<span class="text-2xl" aria-hidden="true">🛠️</span>
				<div>
					<div class="flex flex-wrap items-center gap-2">
						<h3 class="text-sm font-semibold">{t('developer.mode')}</h3>
						{#if status}
							<span
								class="badge {status.enabled
									? 'bg-emerald-500/15 text-emerald-300'
									: 'bg-slate-700/40 text-slate-400'}"
							>
								{status.enabled ? t('developer.modeOn') : t('developer.modeOff')}
							</span>
						{/if}
					</div>
					<p class="mt-1 max-w-2xl text-xs text-slate-500">{t('developer.modeHint')}</p>
				</div>
			</div>
			{#if status?.env_locked}
				<span class="badge bg-amber-500/15 text-amber-300" title={t('developer.envLockedHint')}>
					🔒 {t('developer.envLocked')}
				</span>
			{:else if modeOnly}
				<button
					type="button"
					role="switch"
					aria-checked={status?.enabled === true}
					aria-label={t('developer.mode')}
					class="relative h-6 w-11 shrink-0 rounded-full transition {status?.enabled
						? 'bg-emerald-600'
						: 'bg-slate-700'} disabled:cursor-not-allowed disabled:opacity-50"
					disabled={savingMode || !status}
					onclick={toggleMode}
				>
					<span
						class="absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all {status?.enabled
							? 'left-[22px]'
							: 'left-0.5'}"
					></span>
				</button>
			{/if}
		</div>
		{#if status && !status.enabled}
			<p class="mt-3 rounded-lg border border-slate-800 bg-slate-950 p-3 text-xs text-slate-400">
				{t('developer.disabledHint')}
			</p>
		{/if}
	</section>

	{#if !modeOnly && status?.enabled}
		<!-- Developer tokens ------------------------------------------------ -->
		<section class="card">
			<div class="mb-3 flex flex-wrap items-center justify-between gap-2">
				<div class="flex items-center gap-2">
					<h3 class="text-sm font-semibold">{t('developer.tokens')}</h3>
					<span class="badge bg-slate-700/40 text-slate-300">{activeTokens}</span>
				</div>
				<div class="flex items-center gap-2">
					<button class="btn text-xs" onclick={load} disabled={loading}>{t('common.refresh')}</button>
					<button class="btn btn-primary text-xs" onclick={openCreate}>
						＋ {t('developer.createToken')}
					</button>
				</div>
			</div>
			<p class="mb-3 text-xs text-slate-500">{t('developer.tokensHint')}</p>

			<div class="overflow-x-auto">
				<table class="table">
					<thead>
						<tr>
							<th>{t('developer.prefix')}</th>
							<th>{t('developer.name')}</th>
							<th>{t('developer.permissions')}</th>
							<th>{t('developer.created')}</th>
							<th>{t('developer.lastUsed')}</th>
							<th></th>
						</tr>
					</thead>
					<tbody>
						{#each status.tokens as token (token.id)}
							<tr class={token.revoked_at ? 'opacity-40' : ''}>
								<td class="font-mono text-xs">{token.prefix}</td>
								<td class="text-xs">{token.name}</td>
								<td class="text-xs">
									{#if token.all_endpoints}
										<span class="badge bg-emerald-500/15 text-emerald-300">
											{t('developer.permissionsFullShort')}
										</span>
									{:else}
										<span class="badge bg-amber-500/15 text-amber-300">
											{t('developer.permissionsRestrictedShort')} · {token.permissions.length}
										</span>
									{/if}
								</td>
								<td class="text-xs">{dateTime(token.created_at)}</td>
								<td class="text-xs">{token.last_used_at ? dateTime(token.last_used_at) : '—'}</td>
								<td>
									<div class="flex items-center justify-end gap-1">
										<button class="btn px-2 py-0.5 text-xs" onclick={() => openEdit(token)}>
											{t('common.edit')}
										</button>
										{#if token.revoked_at}
											<button class="btn btn-danger px-2 py-0.5 text-xs" onclick={() => remove(token)}>
												{t('common.delete')}
											</button>
										{:else}
											<button class="btn btn-danger px-2 py-0.5 text-xs" onclick={() => revoke(token)}>
												{t('developer.revoke')}
											</button>
										{/if}
									</div>
								</td>
							</tr>
						{/each}
						{#if status.tokens.length === 0}
							<tr><td colspan="6" class="text-xs text-slate-500">{t('developer.noTokens')}</td></tr>
						{/if}
					</tbody>
				</table>
			</div>
		</section>

		<!-- API documentation (Swagger UI) ---------------------------------- -->
		<section class="card border-emerald-900/60 bg-emerald-950/10">
			<div class="flex items-start gap-3">
				<span class="text-2xl" aria-hidden="true">📖</span>
				<div class="min-w-0 flex-1">
					<h3 class="text-sm font-semibold">{t('developer.apiSpec')}</h3>
					<p class="mt-1 max-w-2xl text-xs text-slate-400">{t('developer.apiSpecHint')}</p>
					<ul class="mt-2 space-y-0.5 text-xs text-slate-400">
						<li>• {t('developer.docsFeatureAuth')}</li>
						<li>• {t('developer.docsFeatureTry')}</li>
						<li>• {t('developer.docsFeatureSchema')}</li>
					</ul>
					<div class="mt-3 flex flex-wrap items-center gap-2">
						<a class="btn btn-primary text-xs" href="/swagger">🔎 {t('developer.openApiDocs')}</a>
						<a
							class="btn text-xs"
							href="/openapi.yaml"
							download="serpentseek-openapi.yaml"
						>
							⬇ {t('developer.download')}
						</a>
					</div>
				</div>
			</div>
		</section>
	{/if}
</div>

<!-- Create / edit dialog -->
<TokenModal
	open={tokenModalOpen}
	token={tokenTarget}
	{endpoints}
	saving={tokenSaving}
	onsave={submitToken}
	onclose={closeTokenModal}
/>

<!-- One-time plaintext reveal -->
<Modal open={createdOpen} title={t('developer.tokenCreatedTitle')} onclose={dismissCreated}>
	<div class="space-y-3">
		<p class="text-sm text-emerald-300">{t('developer.tokenOnce')}</p>
		<div class="flex items-center gap-2">
			<pre class="flex-1 overflow-x-auto rounded bg-slate-950 p-3 text-xs text-emerald-200">{createdToken}</pre>
			<CopyButton text={createdToken} />
		</div>
	</div>
	{#snippet footer()}
		<button class="btn btn-primary" onclick={dismissCreated}>{t('developer.done')}</button>
	{/snippet}
</Modal>
