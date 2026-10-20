import { Button } from '../../components/UI';
import type { ChatChoice, PlanningSlot, SchedulingCard as Card } from '../../types/api';

export function planningSlotLabel(slot: PlanningSlot, zone: string, today: string) {
  const day = new Intl.DateTimeFormat('sv-SE', { timeZone: zone }).format(new Date(slot.start));
  const tomorrow = new Date(`${today}T12:00:00Z`); tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
  const prefix = day === today ? 'Сегодня' : day === tomorrow.toISOString().slice(0, 10) ? 'Завтра' : new Intl.DateTimeFormat('ru-RU', { timeZone: zone, day: 'numeric', month: 'long' }).format(new Date(slot.start));
  const fmt = new Intl.DateTimeFormat('ru-RU', { timeZone: zone, hour: '2-digit', minute: '2-digit' });
  return `${prefix}, ${fmt.format(new Date(slot.start))}–${fmt.format(new Date(slot.end))}`;
}
export function SchedulingCard({ card, active, busy, onChoose }: { card: Card; active: boolean; busy: boolean; onChoose: (text: string, choice: ChatChoice) => void }) {
  const choose = (text: string, action: ChatChoice['action'], index?: number, slot?: PlanningSlot) => onChoose(text, { token: card.token, action, index, slot });
  const disabled = !active || busy;
  return <section className="chat-scheduling-card" aria-label={card.kind === 'slots' ? 'Выбор времени встречи' : 'Выбор существующей встречи'}>
    <h3>{card.kind === 'slots' ? 'Ближайшее свободное время' : 'Существующие встречи'}</h3>
    {card.title && <p>{card.title}</p>}
    {card.requested && <small>Исходное время: {planningSlotLabel(card.requested, card.timezone, card.today)}</small>}
    <div className="chat-scheduling-options">{card.slots?.map((slot, i) => { const label = planningSlotLabel(slot, card.timezone, card.today); return <Button key={slot.start} variant="soft" disabled={disabled} onClick={() => choose(label, 'slot', i + 1, slot)}>{label}</Button>; })}
      {card.meetings?.map((event, i) => <Button key={event.id} variant="soft" disabled={disabled} onClick={() => choose(`Выбираю встречу: ${event.display_title || event.title}`, 'meeting', i + 1)}><span>{event.display_title || event.title}<small>{event.date}, {event.time} · {event.organizer_name}</small></span></Button>)}
    </div>
    {!card.complete && <p role="status" className="field-hint">Внешние календари проверены не полностью. Доступность предварительная.</p>}
    <div className="chat-scheduling-actions">{card.kind === 'slots' && !!card.slots?.length && <Button variant="ghost" disabled={disabled} onClick={() => choose('Показать ещё варианты', 'more')}>Показать ещё</Button>}
      {card.kind === 'existing' && <Button disabled={disabled} onClick={() => choose('Добавить Zoom к этой встрече', 'confirm')}>Добавить Zoom</Button>}
      {card.kind !== 'slots' && <Button variant="ghost" disabled={disabled} onClick={() => choose('Создать отдельную новую встречу', 'new')}>Создать отдельную</Button>}
      <Button variant="ghost" disabled={disabled} onClick={() => choose('Отмена', 'cancel')}>Отмена</Button></div>
    {!active && <small>Выбор завершён. Для нового времени напишите Гене.</small>}
  </section>;
}
