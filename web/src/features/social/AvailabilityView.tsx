import { Button } from '../../components/UI';
import { useSocial } from './SocialContext';
import type { Availability, Slot } from './types';
import { slotLabel } from './types';
export function AvailabilityView({ value, zone, onSelect }: { value: Availability; zone: string; onSelect: (slot: Slot) => void }) {
  const { people } = useSocial(); const names = new Map(people?.friends.map(p => [p.id, p.name])); if (people) names.set(people.profile.id, 'Вы');
  return <div className="availability-result" aria-live="polite"><div className="availability-people">{value.users.map(p => <span key={p.user_id} className={`availability-status ${p.status}`}><i/>{names.get(p.user_id) || 'Участник'}: {{ free: 'свободен', busy: 'занят', unknown: 'неизвестно' }[p.status]}</span>)}</div>{!value.complete && <p className="field-hint">Доступность проверена не полностью: один из внешних календарей недоступен. Варианты предварительные.</p>}<strong>Общее свободное время</strong><small className="muted">Часовой пояс: {zone}</small><div className="available-slots">{value.slots.map(slot => <Button key={slot.start} variant="soft" onClick={() => onSelect(slot)}>{slotLabel(slot, zone)}</Button>)}</div>{!value.slots.length && <p className="muted">В ближайшие 14 дней подходящих интервалов нет. Попробуйте другую длительность или дату.</p>}</div>;
}
