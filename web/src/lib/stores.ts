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
