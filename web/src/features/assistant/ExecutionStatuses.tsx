import type { ActionResult, ExecutionStatus } from '../../types/api';

const providers: Record<string, string> = { google: 'Google Calendar', yandex: 'Яндекс Календарь', zoom: 'Zoom', web_push: 'Уведомления' };

export function ExecutionStatuses({ statuses, steps = [] }: { statuses: ExecutionStatus[]; steps?: ActionResult[] }) {
  if (!statuses.length) return null;
  return <section className="chat-execution-statuses" aria-label="Отправка и синхронизация">
    <h3>Отправка и синхронизация</h3>
    <ul>{statuses.map(status => <li key={`${status.event_id}:${status.provider}`}>
      <strong>{providers[status.provider] || status.provider}</strong>
      {status.step_keys?.map(key => steps.findIndex(step => step.key === key)).filter(index => index >= 0).map(index => <span key={index} className="muted"> · Действие {index + 1}</span>)}
      <p>{status.message}</p>
    </li>)}</ul>
  </section>;
}
