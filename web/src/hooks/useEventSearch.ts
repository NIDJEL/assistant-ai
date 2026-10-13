import { useEffect, useState } from 'react';
import { listEvents } from '../api/events';
import { errorMessage } from '../api/client';
import { useData } from '../app/DataContext';
import { useDebounced } from './useDebounced';
import type { RhythmEvent } from '../types/api';
export function useEventSearch(query: string) {
  const { events, revision } = useData(); const debounced = useDebounced(query.trim());
  const [result, setResult] = useState<RhythmEvent[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(false);
  const [resolvedQuery, setResolvedQuery] = useState('');
  useEffect(() => {
    if (!debounced) { setError(''); setLoading(false); return; }
    const controller = new AbortController(); setLoading(true); setError('');
    listEvents({ q: debounced }, controller.signal).then(items => { if (!controller.signal.aborted) { setResult(items); setResolvedQuery(debounced); } }).catch(e => { if (!controller.signal.aborted) setError(errorMessage(e)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [debounced, revision]);
  const current = new Map(events.map(event => [event.id, event]));
  const matched = result.flatMap(event => { const latest = current.get(event.id); return latest ? [{ ...latest, score: event.score }] : []; });
  // Keep existing rows mounted while a new search is in flight, preserving editors/menus.
  return { events: debounced && resolvedQuery === debounced ? matched : events, error, loading: loading || query.trim() !== debounced };
}
