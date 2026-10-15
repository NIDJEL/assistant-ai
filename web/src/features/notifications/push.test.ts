import { enablePush, disablePush, getDevices, pushCapability, deviceID, type Device } from './push';
import { registerNotifications } from './notifications';
vi.mock('./notifications', () => ({ registerNotifications: vi.fn() }));
const device: Device = { id: 'device-1', platform: 'Windows', disabled_at: null, last_success_at: null, failure_count: 0 };
let subscription: PushSubscription | null;
let unsubscribe: ReturnType<typeof vi.fn>;
let subscribe: ReturnType<typeof vi.fn>;
let permission = 'default';
let requestPermission: ReturnType<typeof vi.fn>;
let fetch: ReturnType<typeof vi.fn>;
function fakeSubscription(expirationTime: number | null = null) {
  return { expirationTime, options: {}, toJSON: () => ({ endpoint: 'https://push.test/device', keys: { p256dh: 'public', auth: 'auth' }, expirationTime }), unsubscribe } as unknown as PushSubscription;
}
beforeEach(() => {
  localStorage.clear(); permission = 'default';
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Chrome Windows');
  vi.spyOn(navigator, 'platform', 'get').mockReturnValue('Win32');
  vi.stubGlobal('isSecureContext', true); vi.stubGlobal('PushManager', class {});
  vi.stubGlobal('MessageChannel', class { port1 = { onmessage: null as (() => void) | null, close() {} }; port2 = { postMessage: () => this.port1.onmessage?.() }; });
  requestPermission = vi.fn(async () => { permission = 'granted'; return permission; });
  vi.stubGlobal('Notification', { get permission() { return permission; }, requestPermission });
  unsubscribe = vi.fn(async () => { subscription = null; return true; });
  subscription = null; subscribe = vi.fn(async () => { subscription = fakeSubscription(); return subscription; });
  const registration = { active: { postMessage: (_: unknown, ports: { postMessage: () => void }[]) => ports[0].postMessage() }, pushManager: { getSubscription: vi.fn(async () => subscription), subscribe } };
  Object.defineProperty(navigator,'serviceWorker',{configurable:true,value:{getRegistration:async()=>registration}});
  vi.mocked(registerNotifications).mockResolvedValue(registration as unknown as ServiceWorkerRegistration);
  fetch = vi.fn(async (path: string) => new Response(JSON.stringify(path.endsWith('/config') ? { enabled: true, public_key: 'BAAAAAAAAAAAAAAAAAAAAA' } : path.endsWith('/subscriptions') ? { subscription: device, subscriptions: [device] } : {}), { status: path.includes('/subscriptions/') ? 204 : 200 }));
  // 204 responses cannot have a body.
  fetch.mockImplementation(async (path: string) => path.includes('/subscriptions/') ? new Response(null, { status: 204 }) : new Response(JSON.stringify(path.endsWith('/config') ? { enabled: true, public_key: 'BAAAAAAAAAAAAAAAAAAAAA' } : { subscription: device, subscriptions: [device] })));
  vi.stubGlobal('fetch', fetch);
});
afterEach(() => { delete (navigator as unknown as Record<string,unknown>).serviceWorker; });
it('detects unsupported browsers without requesting permission', async () => {
  vi.stubGlobal('PushManager', undefined); delete (window as unknown as Record<string, unknown>).PushManager;
  expect(pushCapability()).toBe('unsupported'); await expect(enablePush('alice')).rejects.toThrow(); expect(requestPermission).not.toHaveBeenCalled();
});
it('keeps default permission unchanged until an explicit enable action', async () => {
  expect(pushCapability()).toBe('supported'); expect(requestPermission).not.toHaveBeenCalled();
  await enablePush('alice'); expect(requestPermission).toHaveBeenCalledOnce(); expect(subscribe).toHaveBeenCalledWith(expect.objectContaining({ userVisibleOnly: true }));
  expect(deviceID('alice')).toBe(device.id); const body = JSON.parse(fetch.mock.calls.find(([path,init]) => path.endsWith('/subscriptions') && init.method==='POST')![1].body);
  expect(body.platform).toBe('Windows'); expect(body).not.toHaveProperty('expirationTime');
});
it('uses granted permission without asking again', async () => { permission = 'granted'; await enablePush('alice'); expect(requestPermission).not.toHaveBeenCalled(); });
it('does not subscribe after permission is denied', async () => { requestPermission.mockResolvedValue('denied'); expect(await enablePush('alice')).toBeUndefined(); expect(subscribe).not.toHaveBeenCalled(); expect(fetch).not.toHaveBeenCalled(); });
it('registers an already subscribed browser without creating another subscription', async () => { permission = 'granted'; subscription = fakeSubscription(); await enablePush('alice'); expect(subscribe).not.toHaveBeenCalled(); expect(deviceID('alice')).toBe(device.id); });
it('renews an expired browser subscription', async () => { permission = 'granted'; subscription = fakeSubscription(Date.now() - 1); await enablePush('alice'); expect(unsubscribe).toHaveBeenCalledOnce(); expect(subscribe).toHaveBeenCalledOnce(); });
it('renews a subscription disabled by the server after a dead endpoint', async () => {
  permission='granted';subscription=fakeSubscription();localStorage.setItem('gena.push.device.alice',device.id);
  fetch.mockImplementation(async (path:string,init?:RequestInit)=>new Response(JSON.stringify(path.endsWith('/config')?{enabled:true,public_key:'BAAAAAAAAAAAAAAAAAAAAA'}:init?.method==='POST'?{subscription:device}:{subscriptions:[{...device,disabled_at:new Date().toISOString()}]})));
  await enablePush('alice');expect(unsubscribe).toHaveBeenCalledOnce();expect(subscribe).toHaveBeenCalledOnce();
});
it('removes the backend subscription and unsubscribes locally', async () => { await enablePush('alice'); await disablePush('alice'); expect(fetch).toHaveBeenCalledWith('/api/notifications/push/subscriptions/device-1', expect.objectContaining({ method: 'DELETE' })); expect(unsubscribe).toHaveBeenCalledOnce(); expect(deviceID('alice')).toBeNull(); });
it('requires installation on iPhone before asking permission', async () => { vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('iPhone'); vi.stubGlobal('matchMedia', () => ({ matches: false })); expect(pushCapability()).toBe('ios-install'); await expect(enablePush('alice')).rejects.toThrow(); expect(requestPermission).not.toHaveBeenCalled(); });
it('supports iPhone and iPad standalone mode', async () => { vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('iPad'); vi.stubGlobal('matchMedia', () => ({ matches: true })); expect(pushCapability()).toBe('supported'); await enablePush('alice'); expect(requestPermission).toHaveBeenCalledOnce(); });
it('rolls back a newly created subscription when the backend rejects it', async () => {
  fetch.mockImplementation(async (path: string,init?:RequestInit) => path.endsWith('/config') ? new Response(JSON.stringify({ enabled: true, public_key: 'BAAAAAAAAAAAAAAAAAAAAA' })) : init?.method !== 'POST' ? new Response(JSON.stringify({subscriptions:[]})) : new Response(JSON.stringify({ error: { code: 'validation_error', message: 'Некорректные настройки' } }), { status: 422 }));
  await expect(enablePush('alice')).rejects.toThrow('Некорректные настройки'); expect(unsubscribe).toHaveBeenCalledOnce(); expect(deviceID('alice')).toBeNull();
});
it('does not register a device when the server has no keys', async () => { fetch.mockImplementation(async()=>new Response(JSON.stringify({ enabled: false, public_key: '',subscriptions:[] }))); await expect(enablePush('alice')).rejects.toThrow('сервере'); expect(subscribe).not.toHaveBeenCalled(); });
it('only exposes sanitized device information', async () => { expect((await getDevices()).subscriptions).toEqual([device]); });
