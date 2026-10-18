import { useEffect, useState } from 'react';
import { getIntegrations, type IntegrationState } from '../../api/integrations';
import { errorMessage } from '../../api/client';
const changed = 'rhythm.integrations.changed';
export function refreshIntegrations() { window.dispatchEvent(new Event(changed)); }
export function useYandexIntegration() {
  const [data, setData] = useState<IntegrationState>();
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController(); let pending = false;
    async function load() {
      if (pending) return; pending = true;
      try { const next = await getIntegrations(controller.signal); if (!controller.signal.aborted) { setData(next); setError(''); } }
      catch (e) { if (!controller.signal.aborted) setError(errorMessage(e)); }
      finally { pending = false; }
    }
    void load(); const timer = window.setInterval(() => void load(), 5000);
    window.addEventListener(changed, load);
    return () => { controller.abort(); window.clearInterval(timer); window.removeEventListener(changed, load); };
  }, []);
  return { data, connection: data?.connections.find(c => c.provider === 'yandex'), configured: data?.yandex_configured, loaded: !!data, error };
}
