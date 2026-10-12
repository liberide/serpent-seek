<script lang="ts">
	import { onMount } from 'svelte';
	import { del, get, patch, post, put } from '$lib/api';
	import { notify, session } from '$lib/stores';
	import Modal from '$lib/components/Modal.svelte';
	import QuotaModal, { type QuotaPayload, type QuotaStatus } from '$lib/components/QuotaModal.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { dateTime } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type User = {
		id: string;
		name: string;
		role: string;
		disabled: boolean;
		created_at: string;
		quota?: QuotaStatus | null;
	};
	type Key = {
		id: string;
		user_id: string;
		name: string;
		prefix: string;
		scopes: string;
		chain_id: string;
		last_used_at: string;
		revoked_at: string;
		used_today: number;
	};
	type Chain = { id: string; name: string; active: boolean };
	type Passkey = { id: string; name: string; created_at: string; last_used_at: string };

	let users = $state<User[]>([]);
	let keys = $state<Key[]>([]);
	let chains = $state<Chain[]>([]);
	let passkeys = $state<Passkey[]>([]);
	let newUser = $state({ name: '', role: 'viewer' });
	let keyModal = $state(false);
	let keyForm = $state({ user_id: '', name: '', scope: 'search', chain_id: '' });
	let createdKey = $state('');
	let passkeyName = $state('passkey');
	// Chain binding dialog (explicit Save/Cancel; never commits on selection).
	let chainModal = $state(false);
	let chainTarget = $state<Key | null>(null);
	let chainDraft = $state('');
	let chainSaving = $state(false);
	let chainError = $state('');
	// Per-user quota dialog.
	let quotaModal = $state(false);
	let quotaTarget = $state<User | null>(null);
	let quotaStatus = $state<QuotaStatus | null>(null);

	async function load() {
		users = await get<User[]>('/api/users');
		keys = await get<Key[]>('/api/keys');
		await refreshChains();
		if ($session?.user?.id) {
			passkeys = await get<Passkey[]>(`/api/users/${$session.user.id}/passkeys`).catch(() => []);
		}
	}

	async function refreshChains() {
		chains = await get<Chain[]>('/api/chains').catch(() => []);
	}

	// chainName renders the token's effective chain (default when unbound).
	function chainName(id: string): string {
		if (!id) return t('users.activeChain');
		return chains.find((c) => c.id === id)?.name ?? t('users.unknownChain');
	}

	// chainMissing reports a binding whose chain no longer exists (FK clears it
	// server-side, so this is only a transient/foreign-data state).
	function chainMissing(key: Key): boolean {
		return !!key.chain_id && !chains.some((c) => c.id === key.chain_id);
	}

	async function openKeyModal() {
		createdKey = '';
		keyForm = { ...keyForm, chain_id: '' };
		keyModal = true;
		await refreshChains();
	}

	async function openChainModal(key: Key) {
		chainTarget = key;
		chainDraft = key.chain_id ?? '';
		chainError = '';
		chainModal = true;
		// Re-read the chain list so a chain created in another tab is selectable
		// and a concurrently deleted one is flagged before saving.
		await refreshChains();
	}

	function closeChainModal() {
		if (chainSaving) return;
		chainModal = false;
		chainTarget = null;
	}

	async function saveChain() {
		if (!chainTarget) return;
		chainSaving = true;
		chainError = '';
		try {
			await patch(`/api/keys/${chainTarget.id}`, { chain_id: chainDraft });
			notify(t('users.chainUpdated'), 'success');
			chainModal = false;
			chainTarget = null;
			await load();
		} catch (error) {
			// Keep the dialog open; the server is the source of truth, so refresh
			// the chain list in case the target chain was removed meanwhile.
			chainError = (error as Error).message;
			await refreshChains();
		} finally {
			chainSaving = false;
		}
	}

	onMount(load);

	async function createUser() {
		try {
			await post('/api/users', newUser);
			newUser = { name: '', role: 'viewer' };
			notify(t('users.created'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function toggle(user: User) {
		try {
			await patch(`/api/users/${user.id}`, { disabled: !user.disabled });
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function removeUser(user: User) {
		try {
			await del(`/api/users/${user.id}`);
			notify(t('users.deleted'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function createKey() {
		try {
			const result = await post<any>('/api/keys', keyForm);
			createdKey = result.plaintext;
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function revoke(key: Key) {
		try {
			await del(`/api/keys/${key.id}`);
			notify(t('users.keyRevoked'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function registerPasskey() {
		try {
			const begin = await post<any>('/api/auth/passkey/register/begin');
			const { startRegistration } = await import('@simplewebauthn/browser');
			const credential = await startRegistration({ optionsJSON: begin.options.publicKey });
			await post(
				`/api/auth/passkey/register/finish?session=${begin.session}&name=${encodeURIComponent(passkeyName)}`,
				credential
			);
			notify(t('users.passkeyAdded'), 'success');
			await load();
		} catch (error) {
			notify((error as Error).message, 'error');
		}
	}

	async function deletePasskey(id: string) {
		await del(`/api/passkeys/${id}`);
		await load();
	}

	async function openQuotaModal(user: User) {
		quotaTarget = user;
		quotaStatus = user.quota ?? null;
		quotaModal = true;
		// Re-read for a fresh usage count (the list row may be stale).
		try {
			quotaStatus = await get<QuotaStatus>(`/api/users/${user.id}/quota`);
			if (!quotaStatus || !quotaStatus.requests) quotaStatus = null;
		} catch {
			// Keep the embedded status; the modal still works without it.
		}
	}

	async function saveQuota(payload: QuotaPayload) {
		if (!quotaTarget) return;
		if (payload.requests > 0) {
			await put<QuotaStatus>(`/api/users/${quotaTarget.id}/quota`, payload);
			notify(t('users.quotaSaved'), 'success');
		} else {
			// "Unlimited" checkbox: remove the quota row entirely so the user
			// falls back to the global settings. DELETE is used instead of
			// PUT requests=0 so no limit/period state survives on the server
			// and cannot leak back into the modal later.
			await del(`/api/users/${quotaTarget.id}/quota`);
			notify(t('users.quotaRemoved'), 'success');
		}
		quotaModal = false;
		quotaTarget = null;
		quotaStatus = null;
		await load();
	}

	async function removeQuota() {
		if (!quotaTarget) return;
		await del(`/api/users/${quotaTarget.id}/quota`);
		notify(t('users.quotaRemoved'), 'success');
		quotaModal = false;
		quotaTarget = null;
		quotaStatus = null;
		await load();
	}

	// quotaLabel renders the per-user allowance for the users table.
	function quotaLabel(user: User): string {
		const q = user.quota;
		if (!q || !q.requests) return t('users.quotaUnlimited');
		const periodKey = `users.quotaShort${q.period.charAt(0).toUpperCase()}${q.period.slice(1)}`;
		return `${q.remaining}/${q.requests} · ${t(periodKey)}`;
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold">{t('users.title')}</h1>
		<button class="btn" onclick={load}>{t('common.refresh')}</button>
	</div>

	<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
		<div class="card">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">{t('users.users')}</h2>
			<table class="table">
				<thead><tr><th>{t('common.name')}</th><th>{t('users.role')}</th><th>{t('users.quota')}</th><th>{t('common.status')}</th><th></th></tr></thead>
				<tbody>
					{#each users as user (user.id)}
						<tr>
							<td>{user.name}</td>
							<td>{user.role}</td>
							<td>
								<button
									class="badge border border-slate-700 bg-slate-800 text-slate-200 hover:bg-slate-700"
									title={t('users.quotaEdit')}
									onclick={() => openQuotaModal(user)}
								>
									{#if user.quota && user.quota.requests}
										🚦 {quotaLabel(user)}
									{:else}
										<span class="text-slate-400">∞ {t('users.quotaUnlimited')}</span>
									{/if}
								</button>
							</td>
							<td>{user.disabled ? t('users.disabled') : t('users.enabled')}</td>
							<td class="flex gap-1">
								<button class="btn px-2 py-0.5 text-xs" onclick={() => toggle(user)}>
									{user.disabled ? t('users.enable') : t('users.disable')}
								</button>
								<button class="btn btn-danger px-2 py-0.5 text-xs" onclick={() => removeUser(user)}>✕</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
			<div class="mt-3 flex flex-wrap items-end gap-2">
				<div class="flex-1">
					<span class="label">{t('common.name')}</span>
					<input class="input" bind:value={newUser.name} />
				</div>
				<div>
					<span class="label">{t('users.role')}</span>
					<select class="input" bind:value={newUser.role}>
						<option value="viewer">viewer</option>
						<option value="admin">admin</option>
					</select>
				</div>
				<button class="btn btn-primary" onclick={createUser}>{t('users.add')}</button>
			</div>
		</div>

		<div class="card">
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{t('users.apiKeys')}</h2>
				<button class="btn text-xs" onclick={openKeyModal}>{t('users.addKey')}</button>
			</div>
			<table class="table">
				<thead><tr><th>{t('users.prefix')}</th><th>{t('common.name')}</th><th>{t('users.scopes')}</th><th>{t('users.chain')}</th><th>{t('users.used')}</th><th>{t('users.usedToday')}</th><th></th></tr></thead>
				<tbody>
					{#each keys as key (key.id)}
						<tr class={key.revoked_at ? 'opacity-40' : ''}>
							<td class="font-mono text-xs">{key.prefix}</td>
							<td class="text-xs">{key.name}</td>
							<td class="text-xs">{key.scopes}</td>
							<td class="text-xs">
								{#if key.revoked_at}
									<span class="text-slate-500">{chainName(key.chain_id)}</span>
								{:else}
									<button
										class="badge border border-slate-700 bg-slate-800 text-slate-200 hover:bg-slate-700"
										title={chainMissing(key) ? t('users.chainMissing') : t('users.changeChain')}
										onclick={() => openChainModal(key)}
									>
										{#if chainMissing(key)}⚠ {t('users.unknownChain')}
										{:else if key.chain_id}🔗 {chainName(key.chain_id)}
										{:else}<span class="text-slate-400">{t('users.activeChain')}</span>{/if}
									</button>
								{/if}
							</td>
							<td class="text-xs">{key.last_used_at ? dateTime(key.last_used_at) : '—'}</td>
							<td class="text-xs">{key.used_today ?? 0}</td>
							<td>
								{#if !key.revoked_at}
									<button class="btn btn-danger px-2 py-0.5 text-xs" onclick={() => revoke(key)}>{t('users.revoke')}</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card">
		<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">{t('users.passkeys')}</h2>
		{#if $session?.passkeys_enabled}
			<div class="mb-3 flex items-end gap-2">
				<div class="flex-1">
					<span class="label">{t('common.name')}</span>
					<input class="input" bind:value={passkeyName} />
				</div>
				<button class="btn btn-primary" onclick={registerPasskey}>{t('users.addPasskey')}</button>
			</div>
		{:else}
			<p class="mb-3 text-xs text-amber-300">
				{t('users.passkeysHint')}
			</p>
		{/if}
		<ul class="space-y-1 text-sm">
			{#each passkeys as passkey (passkey.id)}
				<li class="flex items-center justify-between border-b border-slate-800/60 py-1">
					<span>{passkey.name || 'passkey'} · {dateTime(passkey.created_at)}</span>
					<button class="btn btn-danger px-2 py-0.5 text-xs" onclick={() => deletePasskey(passkey.id)}>{t('common.delete')}</button>
				</li>
			{/each}
			{#if passkeys.length === 0}<li class="text-slate-500">{t('users.noPasskeys')}</li>{/if}
		</ul>
	</div>
</div>

<Modal open={keyModal} title={t('users.newKeyTitle')} onclose={() => (keyModal = false)}>
	{#if !createdKey}
		<div class="space-y-3">
			<div>
				<span class="label">{t('users.user')}</span>
				<select class="input" bind:value={keyForm.user_id}>
					<option value="">{t('users.select')}</option>
					{#each users as user (user.id)}<option value={user.id}>{user.name}</option>{/each}
				</select>
			</div>
			<div>
				<span class="label">{t('common.name')}</span>
				<input class="input" bind:value={keyForm.name} />
			</div>
			<div>
				<span class="label">{t('users.type')}</span>
				<select class="input" bind:value={keyForm.scope}>
					<option value="search">search</option>
					<option value="admin">admin</option>
				</select>
			</div>
			<div>
				<span class="label">{t('users.chain')}</span>
				<select class="input" bind:value={keyForm.chain_id}>
					<option value="">{t('users.activeChain')}</option>
					{#each chains as chain (chain.id)}<option value={chain.id}>{chain.name}{chain.active ? ` (${t('common.active')})` : ''}</option>{/each}
				</select>
			</div>
			<button class="btn btn-primary w-full" onclick={createKey} disabled={!keyForm.user_id}>{t('users.create')}</button>
		</div>
	{:else}
		<p class="mb-2 text-sm text-emerald-300">{t('users.keyOnce')}</p>
		<div class="flex items-center gap-2">
			<pre class="flex-1 overflow-x-auto rounded bg-slate-950 p-3 text-xs text-emerald-200">{createdKey}</pre>
			<CopyButton text={createdKey} />
		</div>
		<button class="btn btn-primary mt-3 w-full" onclick={() => (keyModal = false)}>{t('users.done')}</button>
	{/if}
</Modal>

<QuotaModal
	open={quotaModal}
	userName={quotaTarget?.name ?? ''}
	status={quotaStatus}
	onsave={saveQuota}
	onremove={removeQuota}
	onclose={() => {
		if (!quotaModal) return;
		quotaModal = false;
		quotaTarget = null;
		quotaStatus = null;
	}}
/>

<Modal open={chainModal} title={t('users.bindTitle')} onclose={closeChainModal}>
	{#if chainTarget}
		<p class="mb-3 text-xs text-slate-400">
			<span class="font-mono text-slate-300">{chainTarget.prefix}</span> · {chainTarget.name}
		</p>
		<label class="label" for="token-chain">{t('users.chain')}</label>
		<select id="token-chain" class="input" bind:value={chainDraft}>
			<option value="">{t('users.activeChain')}</option>
			{#each chains as chain (chain.id)}
				<option value={chain.id}>{chain.name}{chain.active ? ` (${t('common.active')})` : ''}</option>
			{/each}
		</select>
		<p class="mt-2 text-xs text-slate-500">{t('users.bindHint')}</p>
		{#if chainDraft && !chains.some((c) => c.id === chainDraft)}
			<p class="mt-2 text-xs text-amber-300">{t('users.chainMissing')}</p>
		{/if}
		{#if chainError}<p class="mt-2 text-xs text-rose-300">{chainError}</p>{/if}
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<button class="btn" onclick={closeChainModal} disabled={chainSaving}>{t('common.cancel')}</button>
			<button class="btn btn-primary" onclick={saveChain} disabled={chainSaving}>
				{chainSaving ? t('common.saving') : t('common.save')}
			</button>
		</div>
	{/snippet}
</Modal>
