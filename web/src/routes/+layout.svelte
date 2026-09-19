<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { session, notify, authOffNoticeHidden } from '$lib/stores';
	import { t, getLocale, setLocale, availableLanguages, type Locale } from '$lib/i18n.svelte';
	import Toast from '$lib/components/Toast.svelte';
	import SupportButton from '$lib/components/SupportButton.svelte';
	import SupportModal from '$lib/components/SupportModal.svelte';

	let { children } = $props();

	let ready = $state(false);
	let authDisabled = $state(false);
	let version = $state('');
	const publicRoutes = ['/login', '/setup'];
	const isPublic = $derived(publicRoutes.includes(page.url.pathname));

	const nav = [
		{ href: '/', labelKey: 'nav.dashboard', icon: '📊' },
		{ href: '/history', labelKey: 'nav.history', icon: '🕘' },
		{ href: '/chains', labelKey: 'nav.chains', icon: '🔗' },
		{ href: '/providers', labelKey: 'nav.providers', icon: '🧩' },
		{ href: '/playground', labelKey: 'nav.playground', icon: '🔎' },
		{ href: '/mcp', labelKey: 'nav.mcp', icon: '🤖' },
		{ href: '/logs', labelKey: 'nav.logs', icon: '📝' },
		{ href: '/users', labelKey: 'nav.users', icon: '👤' },
		{ href: '/settings', labelKey: 'nav.settings', icon: '⚙️' }
	];

	onMount(async () => {
		// Public endpoint, non-fatal: the sidebar shows the build version.
		api<string | { version?: string }>('/version', { noAuthRedirect: true })
			.then((value) => {
				version = (typeof value === 'string' ? value : (value?.version ?? '')).trim();
			})
			.catch(() => {});

		try {
			const status = await api<any>('/api/setup/status', { noAuthRedirect: true });
			authDisabled = !status.auth_enabled;
			authOffNoticeHidden.set(!!status.auth_off_notice_hidden);
			if (status.needs_setup && status.auth_enabled) {
				if (page.url.pathname !== '/setup') await goto('/setup');
				ready = true;
				return;
			}
			const me = await api<any>('/api/me', { noAuthRedirect: true });
			session.set(me);
		} catch {
			if (!isPublic) await goto('/login');
		}
		ready = true;
	});

	async function logout() {
		try {
			await api('/api/auth/logout', { method: 'POST' });
		} catch (error) {
			notify((error as Error).message, 'error');
		}
		session.set(null);
		await goto('/login');
	}
</script>

<Toast />
<SupportModal />
{#if isPublic || !$session}
	<main class="flex min-h-screen flex-col items-center justify-center gap-6 p-6">
		{@render children()}
		<div class="flex flex-col items-center">
			<a
				href="https://github.com/liberide/serpent-seek"
				target="_blank"
				rel="noopener noreferrer"
				class="font-mono text-xs text-slate-500 transition hover:text-emerald-300"
			>
				{#if version}<span>v{version}</span><span class="mx-1">·</span>{/if}<span>Liberide 2026</span>
			</a>
			<SupportButton />
		</div>
	</main>
{:else}
	<div class="flex min-h-screen">
		<aside class="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-slate-800 bg-slate-900/60 p-4 md:flex">
			<div class="mb-6 flex items-center gap-2 px-2">
				<span class="text-2xl">🐍</span>
				<div>
					<p class="font-semibold leading-tight">SerpentSeek</p>
					<p class="text-xs text-slate-500">{t('statusbar.subtitle')}</p>
				</div>
			</div>
			<nav class="space-y-1">
				{#each nav as item (item.href)}
					<a
						href={item.href}
						class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition hover:bg-slate-800 {page.url.pathname === item.href
							? 'bg-slate-800 text-emerald-300'
							: 'text-slate-300'}"
					>
						<span>{item.icon}</span>{t(item.labelKey)}
					</a>
				{/each}
			</nav>
			{#if authDisabled && !$authOffNoticeHidden}
				<div class="mt-4 rounded-lg border border-amber-700 bg-amber-950/40 p-2 text-xs text-amber-200">
					{t('statusbar.authOff')}
				</div>
			{/if}
			<div class="mt-auto px-2 pt-4">
				<a
					href="https://github.com/liberide/serpent-seek"
					target="_blank"
					rel="noopener noreferrer"
					class="block font-mono text-xs text-slate-500 transition hover:text-emerald-300"
				>
					{#if version}<span>v{version}</span><span class="mx-1">·</span>{/if}<span>Liberide 2026</span>
				</a>
				<SupportButton />
			</div>
		</aside>
		<div class="flex min-w-0 flex-1 flex-col">
			<header class="flex items-center justify-between border-b border-slate-800 bg-slate-900/40 px-4 py-3">
				<div class="text-sm text-slate-400">
					{#if $session?.user}{$session.user.name}{:else}—{/if}
					{#if $session?.admin}<span class="badge ml-2 bg-emerald-500/15 text-emerald-300">admin</span>{/if}
				</div>
				<div class="flex items-center gap-2">
					<select
						class="btn px-2 py-1 text-xs"
						value={getLocale()}
						onchange={(e) => setLocale((e.currentTarget as HTMLSelectElement).value as Locale)}
						aria-label={t('statusbar.language')}
					>
						{#each availableLanguages as lang (lang.code)}
							<option value={lang.code}>{lang.label}</option>
						{/each}
					</select>
					<a class="btn text-xs" href="/playground">{t('statusbar.quickSearch')}</a>
					{#if !authDisabled}
						<button class="btn text-xs" onclick={logout}>{t('statusbar.logout')}</button>
					{/if}
				</div>
			</header>
			<main class="min-w-0 flex-1 p-4 md:p-6">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
