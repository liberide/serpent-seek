<script lang="ts">
	import { onMount } from 'svelte';
	import { del, get, patch, post } from '$lib/api';
	import { notify, session } from '$lib/stores';
	import Modal from '$lib/components/Modal.svelte';
	import { dateTime } from '$lib/format';
	import { t } from '$lib/i18n.svelte';

	type User = { id: string; name: string; role: string; disabled: boolean; created_at: string };
	type Key = {
		id: string;
		user_id: string;
		name: string;
		prefix: string;
		scopes: string;
		last_used_at: string;
		revoked_at: string;
	};
	type Passkey = { id: string; name: string; created_at: string; last_used_at: string };

	let users = $state<User[]>([]);
	let keys = $state<Key[]>([]);
	let passkeys = $state<Passkey[]>([]);
	let newUser = $state({ name: '', role: 'viewer' });
	let keyModal = $state(false);
	let keyForm = $state({ user_id: '', name: '', scope: 'search' });
	let createdKey = $state('');
	let passkeyName = $state('passkey');

	async function load() {
		users = await get<User[]>('/api/users');
		keys = await get<Key[]>('/api/keys');
		if ($session?.user?.id) {
			passkeys = await get<Passkey[]>(`/api/users/${$session.user.id}/passkeys`).catch(() => []);
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
				<thead><tr><th>{t('common.name')}</th><th>{t('users.role')}</th><th>{t('common.status')}</th><th></th></tr></thead>
				<tbody>
					{#each users as user (user.id)}
						<tr>
							<td>{user.name}</td>
							<td>{user.role}</td>
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
				<button class="btn text-xs" onclick={() => { keyModal = true; createdKey = ''; }}>{t('users.addKey')}</button>
			</div>
			<table class="table">
				<thead><tr><th>{t('users.prefix')}</th><th>{t('common.name')}</th><th>{t('users.scopes')}</th><th>{t('users.used')}</th><th></th></tr></thead>
				<tbody>
					{#each keys as key (key.id)}
						<tr class={key.revoked_at ? 'opacity-40' : ''}>
							<td class="font-mono text-xs">{key.prefix}</td>
							<td class="text-xs">{key.name}</td>
							<td class="text-xs">{key.scopes}</td>
							<td class="text-xs">{key.last_used_at ? dateTime(key.last_used_at) : '—'}</td>
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
			<button class="btn btn-primary w-full" onclick={createKey} disabled={!keyForm.user_id}>{t('users.create')}</button>
		</div>
	{:else}
		<p class="mb-2 text-sm text-emerald-300">{t('users.keyOnce')}</p>
		<pre class="overflow-x-auto rounded bg-slate-950 p-3 text-xs text-emerald-200">{createdKey}</pre>
		<button class="btn btn-primary mt-3 w-full" onclick={() => (keyModal = false)}>{t('users.done')}</button>
	{/if}
</Modal>
