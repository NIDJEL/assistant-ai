import { useState } from 'react';
import { importCSV, importDemo } from '../../api/events';
import { errorMessage } from '../../api/client';
import { useData } from '../../app/DataContext';
import { Button } from '../../components/UI';

export function DemoDataCard() {
  const { refresh } = useData();
  const [busy, setBusy] = useState<'demo' | 'file' | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  async function load(source: 'demo' | 'file') {
    if (source === 'file' && !file) return;
    setBusy(source); setMessage(''); setError('');
    try {
      const result = source === 'demo' ? await importDemo() : await importCSV(file!);
      setMessage(result.imported ? `Добавлено записей: ${result.imported}. Уже загруженных: ${result.skipped}. Прошлые события можно найти в архиве или через Гену.` : 'Этот набор уже загружен. Повторных записей не добавлено.');
      await refresh();
    } catch (e) { setError(errorMessage(e)); }
    finally { setBusy(null); }
  }
  function chooseFile(selected: File | null) {
    setMessage(''); setError(''); setFile(null);
    if (!selected) return;
    if (!selected.name.toLowerCase().endsWith('.csv')) { setError('Выберите файл с расширением .csv'); return; }
    if (selected.size === 0) { setError('CSV-файл пуст'); return; }
    if (selected.size > 5 * 1024 * 1024) { setError('CSV должен быть не больше 5 МБ'); return; }
    setFile(selected);
  }
  const template = 'id,date,time,text\n1,2026-12-01,10:00,Подготовить отчёт\n2,2026-12-02,15:00,Встреча с командой\n';
  return <section className="settings-card data-import-card" aria-busy={!!busy}>
    <h2>Загрузка данных</h2>
    <p className="muted">Загрузите готовую историю из 1000 задач и встреч или выберите свой CSV. Записи добавятся только в ваш аккаунт. Повторная загрузка того же файла, даже под другим именем, не создаёт дубли.</p>
    <Button variant="soft" onClick={() => void load('demo')} disabled={!!busy}>{busy === 'demo' ? 'Загружаю примеры…' : 'Загрузить примеры'}</Button>
    <form className="csv-import-form" onSubmit={e => { e.preventDefault(); void load('file'); }}>
      <label>Свой CSV-файл<input type="file" accept=".csv,text/csv" disabled={!!busy} onChange={e => chooseFile(e.target.files?.[0] ?? null)} aria-describedby="csv-format"/></label>
      <p id="csv-format" className="muted">До 5 МБ и 10 000 записей, UTF-8, разделитель — запятая. Столбцы: <code>id,date,time,text</code>. Дата: <code>2026-12-01</code>, время: <code>10:00</code>, id — уникальное положительное число. Изменённый файл считается новым набором.</p>
      <a download="events-template.csv" href={'data:text/csv;charset=utf-8,%EF%BB%BF' + encodeURIComponent(template)}>Скачать образец CSV</a>
      <Button type="submit" variant="soft" disabled={!!busy || !file}>{busy === 'file' ? 'Загружаю файл…' : 'Загрузить свой CSV'}</Button>
    </form>
    {message && <p role="status">{message}</p>}{error && <p role="alert" className="error-text">{error}</p>}
  </section>;
}
