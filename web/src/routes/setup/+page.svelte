<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, post } from '$lib/api';
	import { notify } from '$lib/stores';
	import { t } from '$lib/i18n.svelte';

	let token = $state('');
	let name = $state('admin');
	let busy = $state(false);
	let createdKey = $state('');

	onMount(async () => {
		try {
			const status = await api<any>('/api/setup/status', { noAuthRedirect: true });
			if (!status.needs_setup) await goto('/login');
		} catch {
			/* ignore */
		}
	});

	async function submit() {
		busy = true;
		try {
			const result = await post<any>('/api/auth/setup', { token, name });
			createdKey = result.api_key;
			notify(t('setup.created'), 'success');
		} catch (error) {
			notify((error as Error).message, 'error');
		} finally {
			busy = false;
		}
	}
</script>

<div class="card w-full max-w-lg">
	<h1 class="mb-1 text-xl font-semibold">{t('setup.title')}</h1>
	<p class="mb-4 text-sm text-slate-400">
		{t('setup.hint')}
	</p>
	{#if !createdKey}
		<form
			onsubmit={(event) => {
				event.preventDefault();
				submit();
			}}
		>
			<span class="label">{t('setup.token')}</span>
			<input class="input font-mono text-xs" bind:value={token} placeholder="например ff1089a6" />
			<span class="label mt-3">{t('setup.adminName')}</span>
			<input class="input" bind:value={name} />
			<button class="btn btn-primary mt-4 w-full" type="submit" disabled={busy || !token}>
				{t('setup.create')}
			</button>
		</form>
	{:else}
		<p class="mb-2 text-sm text-emerald-300">{t('setup.saveKey')}</p>
		<pre class="overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs text-emerald-200">{createdKey}</pre>
		<a class="btn btn-primary mt-4 w-full" href="/login">{t('setup.goLogin')}</a>
	{/if}
</div>
