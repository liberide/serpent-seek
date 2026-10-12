<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { session, notify, authOffNoticeHidden, sidebarCollapsed } from '$lib/stores';
	import { t, getLocale, setLocale, availableLanguages, type Locale } from '$lib/i18n.svelte';
	import Toast from '$lib/components/Toast.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import type { IconName } from '$lib/components/icons';
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
		icon: IconName;
		// When set, the item is only rendered for administrators.
		adminOnly?: boolean;
		// When set, the item is only rendered while developer mode is enabled.
		developerOnly?: boolean;
		// Optional expandable sub-sections rendered under the item (selected
		// from the sidebar instead of in-page tabs).
		children?: NavChild[];
	};

	const nav: NavItem[] = [
		{ href: '/', labelKey: 'nav.dashboard', icon: 'dashboard' },
		{
			href: '/analytics',
			labelKey: 'nav.analytics',
			icon: 'analytics',
			children: [
				{ href: '/analytics', labelKey: 'analytics.nav.overview' },
				{ href: '/analytics/users', labelKey: 'analytics.nav.users', adminOnly: true }
			]
		},
		{ href: '/history', labelKey: 'nav.history', icon: 'history' },
		{ href: '/chains', labelKey: 'nav.chains', icon: 'chains' },
		{ href: '/providers', labelKey: 'nav.providers', icon: 'providers' },
		{ href: '/playground', labelKey: 'nav.playground', icon: 'playground' },
		{ href: '/mcp', labelKey: 'nav.mcp', icon: 'mcp' },
		{ href: '/logs', labelKey: 'nav.logs', icon: 'logs' },
		{ href: '/audit', labelKey: 'nav.audit', icon: 'audit' },
		{ href: '/users', labelKey: 'nav.users', icon: 'users' },
		{ href: '/developer', labelKey: 'nav.developer', icon: 'developer', adminOnly: true, developerOnly: true },
		{ href: '/settings', labelKey: 'nav.settings', icon: 'settings' }
	];

	// Top-level items hidden from non-admins (e.g. Developer mode) or while
	// developer mode is disabled.
	const visibleNav = $derived(
		nav.filter(
			(item) =>
				(!item.adminOnly || $session?.admin) &&
				(!item.developerOnly || $session?.developer_mode)
		)
	);

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

	// Collapsing keeps only the icon rail; the choice is persisted by the store.
	function toggleSidebar(): void {
		sidebarCollapsed.update((value) => !value);
		collapsedFlyout = null;
		collapsedTip = null;
	}

	// --- Collapsed rail: fly-out submenu -------------------------------------
	// Grouped items (e.g. Analytics) have no room for their sections on the
	// icon rail, so hovering/focusing them opens a small menu next to the rail.
	// The menu is `position: fixed` (computed from the anchor rect) so it is not
	// clipped by the scrollable <nav>.
	type CollapsedFlyout = {
		href: string;
		labelKey: string;
		top: number;
		left: number;
		children: NavChild[];
	};
	let collapsedFlyout = $state<CollapsedFlyout | null>(null);
	let flyoutTimer: ReturnType<typeof setTimeout> | undefined;

	// Simple fixed-position label shown next to a rail icon on hover/focus.
	// Grouped items are excluded: their fly-out already shows the section name.
	let collapsedTip = $state<{ label: string; top: number; left: number } | null>(null);

	function showCollapsedTip(item: NavItem, event: Event): void {
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		collapsedTip = {
			label: t(item.labelKey),
			top: rect.top + rect.height / 2,
			left: rect.right + 8
		};
	}

	function hideCollapsedTip(): void {
		collapsedTip = null;
	}

	function openCollapsedFlyout(item: NavItem, event: Event): void {
		clearTimeout(flyoutTimer);
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		const maxTop = Math.max(8, window.innerHeight - 150);
		collapsedFlyout = {
			href: item.href,
			labelKey: item.labelKey,
			top: Math.min(rect.top, maxTop),
			left: rect.right + 8,
			children: visibleChildren(item)
		};
	}

	function scheduleCloseFlyout(): void {
		clearTimeout(flyoutTimer);
		flyoutTimer = setTimeout(() => (collapsedFlyout = null), 180);
	}

	function cancelCloseFlyout(): void {
		clearTimeout(flyoutTimer);
	}

	// Navigating anywhere (including via the fly-out) dismisses the menu and tip.
	$effect(() => {
		page.url.pathname;
		collapsedFlyout = null;
		collapsedTip = null;
	});

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
		<aside
			class="sticky top-0 hidden h-screen shrink-0 flex-col border-r border-slate-800 bg-slate-900/60 transition-[width] duration-200 ease-out md:flex {$sidebarCollapsed
				? 'w-[4.5rem] px-2 py-4'
				: 'w-60 p-4'}"
		>
			<!-- Brand mark and the collapse/expand toggle -->
			<div class="mb-6 flex items-center gap-2 {$sidebarCollapsed ? 'flex-col' : 'px-2'}">
				<span class="shrink-0 text-2xl" title="SerpentSeek">🐍</span>
				{#if !$sidebarCollapsed}
					<div class="min-w-0 flex-1">
						<p class="truncate font-semibold leading-tight">SerpentSeek</p>
						<p class="truncate text-xs text-slate-500">{t('statusbar.subtitle')}</p>
					</div>
				{/if}
				<button
					type="button"
					class="shrink-0 rounded-md p-1.5 text-slate-500 transition hover:bg-slate-800 hover:text-slate-200 {$sidebarCollapsed
						? ''
						: 'ml-auto'}"
					title={$sidebarCollapsed ? t('statusbar.expandMenu') : t('statusbar.collapseMenu')}
					aria-label={$sidebarCollapsed ? t('statusbar.expandMenu') : t('statusbar.collapseMenu')}
					aria-expanded={!$sidebarCollapsed}
					onclick={toggleSidebar}
				>
					<Icon name={$sidebarCollapsed ? 'expand' : 'collapse'} class="h-4 w-4" />
				</button>
			</div>
			<nav class="min-h-0 flex-1 space-y-1 overflow-y-auto overflow-x-hidden">
				{#each visibleNav as item (item.href)}
					{#if item.children && !$sidebarCollapsed}
						{@const active = inGroup(item)}
						<div>
							<div
								class="flex items-center rounded-lg transition hover:bg-slate-800 {active
									? 'bg-slate-800 text-emerald-300'
									: 'text-slate-300'}"
							>
								<a
									href={item.href}
									class="flex min-w-0 flex-1 items-center gap-2.5 px-3 py-2 text-sm"
									title={t(item.labelKey)}
								>
									<Icon name={item.icon} class="h-[18px] w-[18px] shrink-0" />
									<span class="truncate">{t(item.labelKey)}</span>
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
					{:else if $sidebarCollapsed}
						<!-- Icon-only rail: every item collapses to a single glyph with its name on hover. -->
						{#if item.children}
							<!-- Grouped item: hovering or focusing it opens a fly-out with its sections. -->
							<a
								href={item.href}
								aria-label={t(item.labelKey)}
								aria-haspopup="true"
								aria-expanded={collapsedFlyout?.href === item.href}
								class="relative flex items-center justify-center rounded-lg p-2.5 transition {inGroup(
									item
								)
									? 'bg-slate-800 text-emerald-300'
									: 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'}"
								onmouseenter={(event) => openCollapsedFlyout(item, event)}
								onfocus={(event) => openCollapsedFlyout(item, event)}
								onmouseleave={scheduleCloseFlyout}
								onblur={scheduleCloseFlyout}
								onclick={() => (collapsedFlyout = null)}
							>
								<Icon name={item.icon} class="h-5 w-5" />
								<svg
									class="absolute right-0.5 top-1/2 h-2.5 w-2.5 -translate-y-1/2 text-slate-600"
									viewBox="0 0 16 16"
									fill="none"
									aria-hidden="true"
								>
									<path
										d="M6 4l4 4-4 4"
										stroke="currentColor"
										stroke-width="1.8"
										stroke-linecap="round"
										stroke-linejoin="round"
									/>
								</svg>
							</a>
						{:else}
							<a
								href={item.href}
								aria-label={t(item.labelKey)}
								class="flex items-center justify-center rounded-lg p-2.5 transition {inGroup(item)
									? 'bg-slate-800 text-emerald-300'
									: 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'}"
								onmouseenter={(event) => showCollapsedTip(item, event)}
								onfocus={(event) => showCollapsedTip(item, event)}
								onmouseleave={hideCollapsedTip}
								onblur={hideCollapsedTip}
							>
								<Icon name={item.icon} class="h-5 w-5" />
							</a>
						{/if}
					{:else}
						<a
							href={item.href}
							title={t(item.labelKey)}
							class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition hover:bg-slate-800 {page.url.pathname === item.href
								? 'bg-slate-800 text-emerald-300'
								: 'text-slate-300'}"
						>
							<Icon name={item.icon} class="h-[18px] w-[18px] shrink-0" />
							<span class="truncate">{t(item.labelKey)}</span>
						</a>
					{/if}
				{/each}
			</nav>
			{#if authDisabled && !$authOffNoticeHidden && !$sidebarCollapsed}
				<div class="mt-4 rounded-lg border border-amber-700 bg-amber-950/40 p-2 text-xs text-amber-200">
					{t('statusbar.authOff')}
				</div>
			{/if}
			{#if $sidebarCollapsed}
				<!--
				  Collapsed footer: the version and the support link stay visible on
				  the rail, so the project can be supported without expanding the menu.
				-->
				<div class="mt-auto flex flex-col items-center gap-2 pt-4">
					{#if showUpdate}
						<a
							href={update.url || 'https://github.com/liberide/serpent-seek/releases'}
							target="_blank"
							rel="noopener noreferrer"
							class="block h-2 w-2 rounded-full bg-amber-400 shadow-[var(--accent-glow)]"
							title={t('settings.info.updateAvailable', { version: update.latest })}
							aria-label={t('settings.info.updateAvailable', { version: update.latest })}
						></a>
					{/if}
					<div
						class="rail-badge"
						title={version ? `SerpentSeek v${version} · Liberide 2026` : 'SerpentSeek · Liberide 2026'}
					>
						<SupportButton compact />
						<a
							href="https://github.com/liberide/serpent-seek"
							target="_blank"
							rel="noopener noreferrer"
							class="rail-badge-version"
							aria-label={version ? `SerpentSeek v${version}` : 'SerpentSeek'}
						>
							{#if version}v{version}{:else}↗{/if}
						</a>
					</div>
					{#if update?.unofficial}
						<span
							class="flex h-4 w-4 items-center justify-center rounded-full bg-amber-500/20 text-[10px] font-bold text-amber-400"
							title={t('settings.info.unofficialHint')}
							aria-label={t('settings.info.unofficial')}
						>!</span>
					{/if}
				</div>
			{:else}
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
			{/if}
		</aside>
		{#if $sidebarCollapsed && collapsedTip && !collapsedFlyout}
			<!-- Immediate name label for a rail icon (grouped items use the fly-out). -->
			<div
				class="collapsed-tip pointer-events-none fixed z-50 -translate-y-1/2 whitespace-nowrap rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-100 shadow-lg"
				style:top={`${collapsedTip.top}px`}
				style:left={`${collapsedTip.left}px`}
				role="tooltip"
			>
				{collapsedTip.label}
			</div>
		{/if}
		{#if $sidebarCollapsed && collapsedFlyout}
			{@const flyout = collapsedFlyout}
			<!-- Fly-out submenu for a grouped rail item, anchored to its trigger. -->
			<div
				class="collapsed-flyout fixed z-50 min-w-44 rounded-lg border border-slate-700 bg-slate-900 p-1 shadow-2xl"
				style:top={`${flyout.top}px`}
				style:left={`${flyout.left}px`}
				role="group"
				aria-label={t(flyout.labelKey)}
				onmouseenter={cancelCloseFlyout}
				onmouseleave={scheduleCloseFlyout}
				onfocusin={cancelCloseFlyout}
				onfocusout={scheduleCloseFlyout}
			>
				<p class="px-3 py-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
					{t(flyout.labelKey)}
				</p>
				{#each flyout.children as child (child.href)}
					<a
						href={child.href}
						class="block rounded-md px-3 py-1.5 text-sm transition {childActive(child.href)
							? 'bg-slate-800 text-emerald-300'
							: 'text-slate-300 hover:bg-slate-800 hover:text-slate-200'}"
						onclick={() => (collapsedFlyout = null)}
					>
						{t(child.labelKey)}
					</a>
				{/each}
			</div>
		{/if}
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
