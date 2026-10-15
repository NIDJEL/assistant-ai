import { Mic, CalendarDays, CheckSquare } from 'lucide-react';
import { PageHeader } from '../../components/UI';
import { useEventActions } from '../../app/EventActionsContext';
import { useVoiceDraft } from '../../features/voice/useVoiceDraft';
import { useAssistant } from '../../features/assistant/useAssistant';
import { Composer } from '../../features/assistant/Composer';
import { Conversation } from '../../features/assistant/Conversation';
export function TodayPage() {
  const actions = useEventActions(); const voice = useVoiceDraft(); const assistant = useAssistant();
  async function send() { if (assistant.busy) return; const draft = voice.currentDraft(); const source = voice.currentSource(); voice.stop(); if (await assistant.send(draft, source)) { voice.commit(); voice.setDraft(''); } }
  const draft = (text: string) => { voice.setDraft(text); document.getElementById('assistant-draft')?.focus(); };
  return <div className="page today-page"><PageHeader title="Чем помочь?" subtitle="Скажите задачу или просто попросите."/><div className="quick-actions"><button type="button" onClick={() => actions.create('reminder')}><Mic size={25}/>Напомнить</button><button type="button" onClick={() => actions.create('meeting')}><CalendarDays size={25}/>Добавить в календарь</button><button type="button" onClick={() => actions.create('task')}><CheckSquare size={25}/>Создать задачу</button></div>
    <Conversation items={assistant.items} busy={assistant.busy} loading={assistant.loading} error={assistant.historyError} onDraft={draft}/><Composer voice={voice} busy={assistant.busy} onSend={send}/>
  </div>;
}
