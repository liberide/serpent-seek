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
	let update = $state<any>(null);
	let updateDismissed = $state('');
	const publicRoutes = ['/login', '/setup'];
	const isPublic = $derived(publicRoutes.includes(page.url.pathname));

	const showUpdate = $derived(
		!!update?.update_available && !!update?.latest && update.latest !== updateDismissed
	);

	function dismissUpdate() {
		updateDismissed = update?.latest ?? 'dismissed';
		if (typeof localStorage !== 'undefined') {
			localStorage.setItem('seek_update_dismissed', updateDismissed);
		}
	}

	type NavChild = { href: string; labelKey: string; adminOnly?: boolean };
	type NavItem = {
		href: string;
		labelKey: string;
		icon: string;
		// Optional expandable sub-sections rendered under the item (selected
		// from the sidebar instead of in-page tabs).
		children?: NavChild[];
	};

	const nav: NavItem[] = [
		{ href: '/', labelKey: 'nav.dashboard', icon: '📊' },
		{
			href: '/analytics',
			labelKey: 'nav.analytics',
			icon: '📈',
			children: [
				{ href: '/analytics', labelKey: 'analytics.nav.overview' },
				{ href: '/analytics/users', labelKey: 'analytics.nav.users', adminOnly: true }
			]
		},
		{ href: '/history', labelKey: 'nav.history', icon: '🕘' },
		{ href: '/chains', labelKey: 'nav.chains', icon: '🔗' },
		{ href: '/providers', labelKey: 'nav.providers', icon: '🧩' },
		{ href: '/playground', labelKey: 'nav.playground', icon: '🔎' },
		{ href: '/mcp', labelKey: 'nav.mcp', icon: '🤖' },
		{ href: '/logs', labelKey: 'nav.logs', icon: '📝' },
		{ href: '/audit', labelKey: 'nav.audit', icon: '📋' },
		{ href: '/users', labelKey: 'nav.users', icon: '👤' },
		{ href: '/settings', labelKey: 'nav.settings', icon: '⚙️' }
	];

	// Which sidebar groups are expanded, keyed by the parent href. A group
	// auto-opens whenever the current route is inside it, so the active
	// section is always visible; manual collapses persist until navigation.
	let groupOpen = $state<Record<string, boolean>>({});
	function inGroup(item: NavItem): boolean {
		return (
			page.url.pathname === item.href || page.url.pathname.startsWith(item.href + '/')
		);
	}
	$effect(() => {
		for (const item of nav) {
			if (item.children && inGroup(item)) groupOpen[item.href] = true;
		}
	});
	// Auto-open on first render too (the effect only re-runs on navigation).
	for (const item of nav) {
		if (item.children && inGroup(item)) groupOpen[item.href] = true;
	}

	// Only sections available to the current identity are listed.
	const visibleChildren = (item: NavItem): NavChild[] =>
		(item.children ?? []).filter((child) => !child.adminOnly || $session?.admin);

	// Exact match for children: '/analytics' must not highlight on
	// '/analytics/users'. Query parameters are not part of pathname.
	const childActive = (href: string): boolean => page.url.pathname === href;

	onMount(async () => {
		// Public endpoint, non-fatal: the sidebar shows the build version.
		if (typeof localStorage !== 'undefined') {
			updateDismissed = localStorage.getItem('seek_update_dismissed') ?? '';
		}
		api<string | { version?: string }>('/version', { noAuthRedirect: true })
			.then((value) => {
				version = (typeof value === 'string' ? value : (value?.version ?? '')).trim();
			})
			.catch(() => {});
		api<any>('/api/update', { noAuthRedirect: true })
			.then((value) => {
				update = value;
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
{#if ready && (isPublic || !$session)}
	<main class="flex min-h-screen flex-col items-center justify-center gap-6 p-6">
		{@render children()}
		<div class="flex flex-col items-center">
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
	</main>
{:else if ready}
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
					{#if item.children}
						{@const active = inGroup(item)}
						<div>
							<div
								class="flex items-center rounded-lg transition hover:bg-slate-800 {active
									? 'bg-slate-800 text-emerald-300'
									: 'text-slate-300'}"
							>
								<a
									href={item.href}
									class="flex flex-1 items-center gap-2 px-3 py-2 text-sm"
									title={t(item.labelKey)}
								>
									<span>{item.icon}</span>{t(item.labelKey)}
								</a>
								<button
									type="button"
									class="mr-1 rounded-md p-1.5 text-slate-500 transition hover:bg-slate-700 hover:text-slate-200"
									aria-expanded={groupOpen[item.href] ?? false}
									aria-label={t(item.labelKey)}
									onclick={() => (groupOpen[item.href] = !groupOpen[item.href])}
								>
									<svg
										class="h-3 w-3 transition-transform {groupOpen[item.href] ? 'rotate-90' : ''}"
										viewBox="0 0 16 16"
										fill="none"
										aria-hidden="true"
									>
										<path
											d="M6 4l4 4-4 4"
											stroke="currentColor"
											stroke-width="1.5"
											stroke-linecap="round"
											stroke-linejoin="round"
										/>
									</svg>
								</button>
							</div>
							{#if groupOpen[item.href]}
								<div class="mt-1 ml-6 space-y-0.5 border-l border-slate-800 pl-2">
									{#each visibleChildren(item) as child (child.href)}
										<a
											href={child.href}
											class="block rounded-md px-3 py-1.5 text-sm transition {childActive(child.href)
												? 'bg-slate-800 text-emerald-300'
												: 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'}"
										>
											{t(child.labelKey)}
										</a>
									{/each}
								</div>
							{/if}
						</div>
					{:else}
						<a
							href={item.href}
							class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition hover:bg-slate-800 {page.url.pathname === item.href
								? 'bg-slate-800 text-emerald-300'
								: 'text-slate-300'}"
						>
							<span>{item.icon}</span>{t(item.labelKey)}
						</a>
					{/if}
				{/each}
			</nav>
			{#if authDisabled && !$authOffNoticeHidden}
				<div class="mt-4 rounded-lg border border-amber-700 bg-amber-950/40 p-2 text-xs text-amber-200">
					{t('statusbar.authOff')}
				</div>
			{/if}
			<div class="mt-auto px-2 pt-4">
				{#if showUpdate}
					<div class="mb-1 flex items-center gap-1 text-xs">
						<a
							class="text-amber-300 transition hover:text-amber-200"
							href={update.url || 'https://github.com/liberide/serpent-seek/releases'}
							target="_blank"
							rel="noopener noreferrer"
							title={t('settings.info.updateAvailable', { version: update.latest })}
						>
							↑ {t('settings.info.updateAvailable', { version: update.latest })}
						</a>
						<button
							class="text-slate-500 transition hover:text-slate-300"
							title={t('common.dismiss')}
							aria-label={t('common.dismiss')}
							onclick={dismissUpdate}
						>
							×
						</button>
					</div>
				{/if}
				{#if update?.unofficial}
					<div
						class="mb-1 text-[10px] font-semibold uppercase tracking-wide text-amber-400"
						title={t('settings.info.unofficialHint')}
					>
						{t('settings.info.unofficial')}
					</div>
				{/if}
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
