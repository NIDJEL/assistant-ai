import { useEffect, useState } from 'react';
import { listEvents } from '../api/events';
import { errorMessage } from '../api/client';
import { useData } from '../app/DataContext';
import { useDebounced } from './useDebounced';
import type { RhythmEvent } from '../types/api';
export function useEventSearch(query: string) {
  const { events, revision } = useData(); const debounced = useDebounced(query.trim());
  const [result, setResult] = useState<RhythmEvent[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (!debounced) { setError(''); setLoading(false); return; }
    const controller = new AbortController(); setLoading(true); setError('');
    listEvents({ q: debounced }, controller.signal).then(setResult).catch(e => { if (!controller.signal.aborted) setError(errorMessage(e)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [debounced, revision]);
  const current = new Map(events.map(event => [event.id, event]));
  const matched = result.flatMap(event => { const latest = current.get(event.id); return latest ? [{ ...latest, score: event.score }] : []; });
  return { events: debounced ? matched : events, error, loading: loading || query.trim() !== debounced };
}
