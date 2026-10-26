import { Button } from '../../components/UI';
import type { ActionResult, ChatChoice } from '../../types/api';

const labels = { done: 'Выполнено', waiting: 'Нужно уточнение', failed: 'Не выполнено', blocked: 'Ожидает', cancelled: 'Отменено' };

export function ActionSteps({ steps, active, busy, onChoose }: { steps: ActionResult[]; active: boolean; busy: boolean; onChoose: (text: string, choice: ChatChoice) => void }) {
  const choose = (step: ActionResult, action: ChatChoice['action'], text: string) => {
    if (!step.control_token || !active || busy) return;
    onChoose(text, { token: step.control_token, key: step.key, action });
  };
  return <section className="chat-action-steps" aria-label="Действия из запроса">
    <h3>Действия из запроса</h3>
    <ol>{steps.map((step, index) => {
      const unfinished = !['done', 'cancelled'].includes(step.status);
      return <li key={step.key} aria-current={step.active ? 'step' : undefined}>
        <div className="chat-action-step-label"><strong>Действие {index + 1}</strong><span role="status">{labels[step.status]}{step.active && unfinished ? ' · выбрано' : ''}</span></div>
        <p>{step.reply.replace(/#\d+\s*/g, '')}</p>
        {active && unfinished && step.control_token && <div className="reply-choices">
          {!step.active && <Button variant="soft" disabled={busy} onClick={() => choose(step, 'action_focus', `Уточнить действие ${index + 1}`)}>Уточнить это действие</Button>}
          {step.can_confirm && <Button disabled={busy} onClick={() => choose(step, 'action_confirm', step.atomic_group ? 'Подтверждаю все изменения в выбранной группе' : `Подтверждаю действие ${index + 1}`)}>{step.atomic_group ? 'Подтвердить всю группу' : 'Подтвердить действие'}</Button>}
          <Button variant="ghost" disabled={busy} onClick={() => choose(step, 'action_cancel', step.atomic_group ? 'Отменить выбранную группу' : `Отменить действие ${index + 1}`)}>{step.atomic_group ? 'Отменить группу' : 'Отменить это действие'}</Button>
        </div>}
      </li>;
    })}</ol>
  </section>;
}
