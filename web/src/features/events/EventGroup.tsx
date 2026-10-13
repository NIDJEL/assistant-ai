import { useState } from 'react';
import type { LucideIcon } from 'lucide-react';
import { Button } from '../../components/UI';
import { EventRow } from './EventRow';
import type { RhythmEvent } from '../../types/api';
export function EventGroup({ title, events, icon: Icon }: { title: string; events: RhythmEvent[]; icon: LucideIcon }) {
  const [limit, setLimit] = useState(20);
  if (!events.length) return null;
  return <section className="event-group"><h2><Icon size={22} aria-hidden="true"/>{title}<span className="count">{events.length}</span></h2><div className="event-rows">{events.slice(0, limit).map(event => <EventRow key={event.id} event={event}/>)}</div>{events.length > limit && <Button variant="ghost" onClick={() => setLimit(current => current + 20)}>Показать ещё ({events.length - limit})</Button>}</section>;
}
