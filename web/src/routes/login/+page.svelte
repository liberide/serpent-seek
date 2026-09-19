<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, post } from '$lib/api';
	import { session, notify } from '$lib/stores';
	import { t } from '$lib/i18n.svelte';

	let key = $state('');
	let busy = $state(false);
	let passkeysEnabled = $state(false);
	let passkeyError = $state('');

	onMount(async () => {
		try {
			const status = await api<any>('/api/setup/status', { noAuthRedirect: true });
			if (status.needs_setup) await goto('/setup');
			passkeysEnabled = status.passkeys_enabled;
		} catch {
			/* ignore */
		}
	});

	async function loginKey() {
		busy = true;
		try {
			await post('/api/auth/login', { key });
			await finishLogin();
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			busy = false;
		}
	}

	async function loginPasskey() {
		busy = true;
		passkeyError = '';
		try {
			const begin = await post<any>('/api/auth/passkey/assert/begin', {});
			const { startAuthentication } = await import('@simplewebauthn/browser');
			const assertion = await startAuthentication({ optionsJSON: begin.options.publicKey });
			await post(`/api/auth/passkey/assert/finish?session=${begin.session}`, assertion);
			await finishLogin();
		} catch (error) {
			passkeyError = (error as Error).message;
			notify(passkeyError, 'error');
		} finally {
			busy = false;
		}
	}

	async function finishLogin() {
		// Populate the session store before navigating so the full layout
		// (sidebar, header) renders immediately on client-side navigation.
		session.set(await api<any>('/api/me'));
		notify(t('login.welcome'), 'success');
		await goto('/');
	}
</script>

<div class="card w-full max-w-md">
	<h1 class="mb-1 text-xl font-semibold">{t('login.title')}</h1>
	<p class="mb-4 text-sm text-slate-400">{t('login.hint')}</p>
	<form
		onsubmit={(event) => {
			event.preventDefault();
			loginKey();
		}}
	>
		<span class="label">{t('login.apiKey')}</span>
		<input class="input font-mono text-xs" bind:value={key} placeholder="seek_ak_..." autocomplete="off" />
		<button class="btn btn-primary mt-3 w-full" type="submit" disabled={busy || !key}>{t('login.signIn')}</button>
	</form>
	{#if passkeysEnabled}
		<button class="btn mt-3 w-full" onclick={loginPasskey} disabled={busy}>{t('login.signInPasskey')}</button>
	{/if}
	{#if passkeyError}<p class="mt-2 text-xs text-rose-400">{passkeyError}</p>{/if}
</div>
