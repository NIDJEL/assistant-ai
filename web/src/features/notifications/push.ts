import { request } from '../../api/client';
import { registerNotifications } from './notifications';

export type Capability = 'supported' | 'unsupported' | 'ios-install';
export interface Device { id: string; platform: string; disabled_at: string | null; last_success_at: string | null; failure_count: number }
export interface PushConfig { enabled: boolean; public_key: string }
const key = (userId: string) => `gena.push.device.${userId}`;
export function deviceID(userId: string) { try { return localStorage.getItem(key(userId)); } catch { return null; } }
function saveDevice(userId: string, id: string | null) { try { if (id) localStorage.setItem(key(userId), id); else localStorage.removeItem(key(userId)); } catch { /* Device detection still uses server state. */ } }
export function isIOS() { return /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1); }
export function isStandalone() { return window.matchMedia?.('(display-mode: standalone)').matches || !!(navigator as Navigator & { standalone?: boolean }).standalone; }
export function pushCapability(): Capability {
  if (isIOS() && !isStandalone()) return 'ios-install';
  return window.isSecureContext && 'Notification' in window && 'PushManager' in window && !!navigator.serviceWorker ? 'supported' : 'unsupported';
}
export function platformName() {
  if (isIOS()) return /iPhone/.test(navigator.userAgent) ? 'iPhone' : 'iPad';
  if (/Android/.test(navigator.userAgent)) return 'Android';
  if (/Windows/.test(navigator.userAgent)) return 'Windows';
  if (/Mac/.test(navigator.userAgent)) return 'Mac';
  if (/Linux/.test(navigator.userAgent)) return 'Linux';
  return 'Этот браузер';
}
export function applicationKey(value: string): Uint8Array<ArrayBuffer> {
  const binary = atob(value.replace(/-/g, '+').replace(/_/g, '/'));
  return Uint8Array.from(binary, character => character.charCodeAt(0));
}
export const getPushConfig = () => request<PushConfig>('/api/notifications/push/config');
export const getDevices = () => request<{ subscriptions: Device[] }>('/api/notifications/push/subscriptions');
export async function setPushUser(userId: string | null) {
  if (!navigator.serviceWorker || !window.isSecureContext) return;
  const registration = await registerNotifications();
  if (!registration?.active) return;
  await new Promise<void>((resolve, reject) => {
    const channel = new MessageChannel();
    const timer = setTimeout(() => { channel.port1.close(); reject(new Error('Не удалось подготовить уведомления.')); }, 5000);
    channel.port1.onmessage = () => { clearTimeout(timer); channel.port1.close(); resolve(); };
    registration.active!.postMessage({ type: 'GENA_USER', userId }, [channel.port2]);
  });
}
export async function enablePush(userId: string): Promise<Device | undefined> {
  if (pushCapability() !== 'supported') throw new Error('Для уведомлений установите Гену или откройте поддерживаемый браузер.');
  // Permission must be requested before any await that could lose the iOS user gesture.
  const permission = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission();
  if (permission !== 'granted') return;
  const [config,devices] = await Promise.all([getPushConfig(),getDevices()]);
  if (!config.enabled || !config.public_key) throw new Error('Уведомления пока не настроены на сервере.');
  const registration = await registerNotifications();
  if (!registration) throw new Error('Не удалось подготовить уведомления.');
  let subscription = await registration.pushManager.getSubscription();
  const serverKey = applicationKey(config.public_key);
  const previousKey = subscription?.options.applicationServerKey;
  const inactive = devices.subscriptions?.some(device => device.id === deviceID(userId) && !!device.disabled_at);
  if (subscription && (inactive || subscription.expirationTime && subscription.expirationTime <= Date.now() || previousKey && Array.from(new Uint8Array(previousKey)).join() !== Array.from(serverKey).join())) {
    await subscription.unsubscribe(); subscription = null;
  }
  let created = false;
  if (!subscription) { subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: serverKey }); created = true; }
  try {
    await setPushUser(userId);
    const result = await request<{ subscription: Device }>('/api/notifications/push/subscriptions', { method: 'POST', body: JSON.stringify({ ...subscription.toJSON(), expirationTime: undefined, platform: platformName() }) });
    saveDevice(userId, result.subscription.id);
    return result.subscription;
  } catch (error) { if (created) await subscription.unsubscribe().catch(() => false); throw error; }
}
export async function disablePush(userId: string) {
  const id = deviceID(userId);
  if (id) await request(`/api/notifications/push/subscriptions/${encodeURIComponent(id)}`, { method: 'DELETE' });
  await unsubscribePush();
  saveDevice(userId, null);
}
async function unsubscribePush() {
  const registration = navigator.serviceWorker ? await navigator.serviceWorker.getRegistration('/') : undefined;
  // iOS browser tabs can expose Service Workers without exposing PushManager.
  const subscription = await registration?.pushManager?.getSubscription();
  if (subscription && !await subscription.unsubscribe()) throw new Error('Не удалось отключить уведомления в браузере. Повторите действие.');
}
export async function releasePushOnLogout(userId: string) {
  try {
    await disablePush(userId);
  } catch {
    // Still remove the browser subscription if the device DELETE failed.
    await unsubscribePush().catch(() => {});
  } finally {
    saveDevice(userId, null);
    await setPushUser(null).catch(() => {});
  }
}
export const testPush = (id: string) => request('/api/notifications/push/test', { method: 'POST', body: JSON.stringify({ subscription_id: id }) });
