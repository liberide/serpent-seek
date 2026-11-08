import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const gotoMock = vi.hoisted(() => vi.fn());
vi.mock('$app/navigation', () => ({ goto: gotoMock }));

import { api, ApiError, get, getCookie, post } from './api';

function jsonResponse(status: number, body: unknown): Response {
	return new Response(body === undefined ? '' : JSON.stringify(body), {
		status,
		statusText: status === 200 ? 'OK' : 'Bad',
		headers: { 'Content-Type': 'application/json' }
	});
}

describe('getCookie', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('reads a cookie value by name', () => {
		vi.stubGlobal('document', { cookie: 'a=1; seek_csrf=tok%3Den; b=2' });
		expect(getCookie('seek_csrf')).toBe('tok=en');
		expect(getCookie('a')).toBe('1');
	});

	it('returns null when the cookie is absent', () => {
		vi.stubGlobal('document', { cookie: 'a=1' });
		expect(getCookie('seek_csrf')).toBeNull();
	});
});

/** Returns the error from a rejected promise. */
async function capture(promise: Promise<unknown>): Promise<ApiError> {
	try {
		await promise;
	} catch (e) {
		return e as ApiError;
	}
	throw new Error('expected the promise to reject');
}

describe('api client', () => {
	const fetchMock = vi.fn();

	beforeEach(() => {
		fetchMock.mockReset();
		gotoMock.mockReset();
		vi.stubGlobal('fetch', fetchMock);
		vi.stubGlobal('document', { cookie: 'seek_csrf=csrf-token' });
	});
	afterEach(() => vi.unstubAllGlobals());

	it('GET returns parsed JSON without a CSRF header', async () => {
		fetchMock.mockResolvedValue(jsonResponse(200, { ok: 1 }));
		const data = await get<{ ok: number }>('/api/thing');
		expect(data).toEqual({ ok: 1 });

		const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
		const headers = init.headers as Record<string, string>;
		expect(init.method).toBe('GET');
		expect('X-CSRF-Token' in headers).toBe(false);
		expect('Content-Type' in headers).toBe(false);
	});

	it('POST serializes the body and attaches the CSRF token', async () => {
		fetchMock.mockResolvedValue(jsonResponse(200, {}));
		await post('/api/thing', { q: 'test' });

		const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
		const headers = init.headers as Record<string, string>;
		expect(init.method).toBe('POST');
		expect(headers['X-CSRF-Token']).toBe('csrf-token');
		expect(headers['Content-Type']).toBe('application/json');
		expect(init.body).toBe('{"q":"test"}');
	});

	it('throws ApiError with the server-provided message', async () => {
		fetchMock.mockResolvedValue(jsonResponse(422, { error: { message: 'chain is invalid' } }));
		const err = await capture(get('/api/chains'));
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(422);
		expect(err.message).toBe('chain is invalid');
	});

	it('falls back to statusText for non-JSON error bodies', async () => {
		fetchMock.mockResolvedValue(new Response('<html>oops</html>', { status: 502, statusText: 'Bad Gateway' }));
		const err = await capture(get('/api/thing'));
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(502);
		expect(err.message).toBe('Bad Gateway');
	});

	it('redirects to /login on 401 and always throws', async () => {
		fetchMock.mockResolvedValue(jsonResponse(401, { error: { message: 'nope' } }));
		const err = await capture(get('/api/me'));
		expect(gotoMock).toHaveBeenCalledWith('/login');
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(401);
	});

	it('does not redirect on 401 when noAuthRedirect is set', async () => {
		fetchMock.mockResolvedValue(jsonResponse(401, { error: { message: 'nope' } }));
		await expect(get('/api/me', true)).rejects.toBeInstanceOf(ApiError);
		expect(gotoMock).not.toHaveBeenCalled();
	});

	it('raw mode returns the untouched Response', async () => {
		const res = jsonResponse(200, { a: 1 });
		fetchMock.mockResolvedValue(res);
		const out = await api<Response>('/api/thing', { raw: true });
		expect(out).toBe(res);
		expect(out.ok).toBe(true);
	});

	it('raw mode still rejects on HTTP errors', async () => {
		fetchMock.mockResolvedValue(jsonResponse(500, { error: { message: 'boom' } }));
		await expect(api('/api/thing', { raw: true })).rejects.toBeInstanceOf(ApiError);
	});
});
