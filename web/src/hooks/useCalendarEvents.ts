import { useEffect, useState } from 'react';
import { listEvents } from '../api/events';
import { errorMessage } from '../api/client';
import { useData } from '../app/DataContext';
import { useDebounced } from './useDebounced';
import type { RhythmEvent } from '../types/api';

export function useCalendarEvents(query: string, start: string, end: string) {
  const { revision } = useData(); const debounced = useDebounced(query.trim());
  const [events, setEvents] = useState<RhythmEvent[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(false);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    listEvents({ q: debounced, start, end, expand: 'true' }, controller.signal, status => { if (!controller.signal.aborted && status?.warning) setError(status.warning); })
      .then(items => { if (!controller.signal.aborted) setEvents(items); })
      .catch(e => { if (!controller.signal.aborted) { setEvents([]); setError(errorMessage(e)); } })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [debounced, start, end, revision]);
  return { events, error, loading: loading || query.trim() !== debounced };
}
