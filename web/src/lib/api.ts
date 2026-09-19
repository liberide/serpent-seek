import { goto } from '$app/navigation';

/** Reads a cookie by name. */
export function getCookie(name: string): string | null {
	const match = document.cookie.match(new RegExp('(^|; )' + name + '=([^;]*)'));
	return match ? decodeURIComponent(match[2]) : null;
}

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

type ApiOptions = {
	method?: string;
	body?: unknown;
	noAuthRedirect?: boolean;
	raw?: boolean;
};

/** Typed fetch wrapper with automatic CSRF header and 401 redirect. */
export async function api<T = unknown>(path: string, options: ApiOptions = {}): Promise<T> {
	const method = (options.method ?? 'GET').toUpperCase();
	const headers: Record<string, string> = { Accept: 'application/json' };
	if (options.body !== undefined) {
		headers['Content-Type'] = 'application/json';
	}
	if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
		const csrf = getCookie('seek_csrf');
		if (csrf) headers['X-CSRF-Token'] = csrf;
	}
	const res = await fetch(path, {
		method,
		headers,
		credentials: 'same-origin',
		body: options.body !== undefined ? JSON.stringify(options.body) : undefined
	});
	if (res.status === 401 && !options.noAuthRedirect) {
		await goto('/login');
		throw new ApiError(401, 'authentication required');
	}
	if (options.raw) {
		if (!res.ok) throw new ApiError(res.status, res.statusText);
		return res as unknown as T;
	}
	const text = await res.text();
	let data: unknown = null;
	if (text) {
		try {
			data = JSON.parse(text);
		} catch {
			data = text;
		}
	}
	if (!res.ok) {
		const message =
			(data as { error?: { message?: string } })?.error?.message ?? res.statusText ?? 'request failed';
		throw new ApiError(res.status, message);
	}
	return data as T;
}

export const get = <T = unknown>(path: string, noAuthRedirect = false) =>
	api<T>(path, { noAuthRedirect });
export const post = <T = unknown>(path: string, body?: unknown) =>
	api<T>(path, { method: 'POST', body });
export const put = <T = unknown>(path: string, body?: unknown) => api<T>(path, { method: 'PUT', body });
export const patch = <T = unknown>(path: string, body?: unknown) =>
	api<T>(path, { method: 'PATCH', body });
export const del = <T = unknown>(path: string, body?: unknown) =>
	api<T>(path, { method: 'DELETE', body });
