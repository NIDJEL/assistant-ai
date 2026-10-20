import { useState } from 'react';
import { importDemo } from '../../api/events';
import { errorMessage } from '../../api/client';
import { useData } from '../../app/DataContext';
import { Button } from '../../components/UI';

export function DemoDataCard() {
  const { refresh } = useData();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  async function load() {
    setBusy(true); setMessage(''); setError('');
    try {
      const result = await importDemo();
      setMessage(result.imported ? `Добавлено записей: ${result.imported}. Ищите их в архиве или попросите Гену найти событие за прошлые годы.` : 'Примеры уже загружены. Повторных записей не добавлено.');
      await refresh();
    } catch (e) { setError(errorMessage(e)); }
    finally { setBusy(false); }
  }
  return <section className="settings-card"><h2>Примеры для знакомства</h2><p className="muted">Добавьте историю из 1000 задач и встреч, чтобы попробовать поиск по прошлым событиям. Повторная загрузка не создаёт дубли.</p><Button variant="soft" onClick={() => void load()} disabled={busy}>{busy ? 'Загружаю примеры…' : 'Загрузить примеры'}</Button>{message && <p role="status">{message}</p>}{error && <p role="alert" className="error-text">{error}</p>}</section>;
}
