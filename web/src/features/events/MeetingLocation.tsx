import { MapPin, Video } from 'lucide-react';
import type { RhythmEvent } from '../../types/api';

export function MeetingLocation({ event, size = 18 }: { event: RhythmEvent; size?: number }) {
  let hostname = '';
  try { hostname = new URL(event.meeting_url).hostname; } catch { /* Older events may have an incomplete link. */ }
  return <>{event.meeting_url ? <Video size={size}/> : <MapPin size={size}/>}<span>{event.location || hostname || (event.meeting_url ? 'Онлайн-встреча' : 'Место не указано')}</span></>;
}
