import { writable } from 'svelte/store';

export type Toast = { message: string; type: 'info' | 'success' | 'error' };

export type Session = {
	via: string;
	admin: boolean;
	scopes: Record<string, boolean>;
	passkeys_enabled: boolean;
	// developer_mode is true while the instance-level developer mode is on; the
	// sidebar only shows the Developer section when it is enabled.
	developer_mode?: boolean;
	user?: { id: string; name: string; role: string };
};

export const session = writable<Session | null>(null);
export const toast = writable<Toast | null>(null);

/**
 * Sidebar collapsed state (icon-only rail). Persisted under
 * `seek_sidebar_collapsed` so the choice survives reloads. The value is read
 * synchronously at module load, before the first render, to avoid a width
 * flash. The store is a module singleton, so the subscription intentionally
 * lives for the lifetime of the page.
 */
export const sidebarCollapsed = (() => {
	const initial =
		typeof localStorage !== 'undefined' && localStorage.getItem('seek_sidebar_collapsed') === '1';
	const store = writable(initial);
	if (typeof localStorage !== 'undefined') {
		store.subscribe((value) =>
			localStorage.setItem('seek_sidebar_collapsed', value ? '1' : '0')
		);
	}
	return store;
})();

/** Hides the sidebar "authentication disabled" warning (Settings → Authentication). */
export const authOffNoticeHidden = writable(false);

/** Controls the global "Support the project" dialog. */
export const supportOpen = writable(false);

let toastTimer: ReturnType<typeof setTimeout> | undefined;

export function notify(message: string, type: Toast['type'] = 'info'): void {
	toast.set({ message, type });
	clearTimeout(toastTimer);
	toastTimer = setTimeout(() => toast.set(null), 4000);
}
