import { ApiError, request } from './client';

it.each([403, 429, 500, 502, 503, 504])('keeps HTTP status %s when a proxy returns an HTML error page', async status => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>Unavailable</html>', { status, headers: { 'Content-Type': 'text/html' } })));
  await expect(request('/api/events')).rejects.toMatchObject({ name: 'ApiError', status, code: 'server_error' });
});
it('distinguishes an invalid successful response from a network failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>not JSON</html>')));
  await expect(request('/api/events')).rejects.toMatchObject({ status: 502, code: 'invalid_response' });
  vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('offline'); }));
  await expect(request('/api/events')).rejects.toBeInstanceOf(ApiError);
  await expect(request('/api/events')).rejects.toMatchObject({ status: 0, code: 'network_error' });
});
