import { PageHeader } from '../../components/UI';
import { useVoiceDraft } from '../../features/voice/useVoiceDraft';
import { useAssistant } from '../../features/assistant/useAssistant';
import { Composer } from '../../features/assistant/Composer';
import { Conversation } from '../../features/assistant/Conversation';
export function TodayPage() {
  const voice = useVoiceDraft(); const assistant = useAssistant();
  const hasMessages = assistant.items.length > 0;
  async function send() { if (assistant.busy) return; const draft = voice.currentDraft(); const source = voice.currentSource(); voice.stop(); if (await assistant.send(draft, source)) { voice.commit(); voice.setDraft(''); } }
  const draft = (text: string) => { voice.setDraft(text); document.getElementById('assistant-draft')?.focus(); };
  return <div className={`page today-page ${hasMessages ? 'has-messages' : ''}`} aria-busy={assistant.loading}>{hasMessages ? <h1 className="sr-only">Чат с Геной</h1> : !assistant.loading && <PageHeader title="Чем помочь?" subtitle="Скажите задачу или просто попросите."/>}
    <Conversation items={assistant.items} busy={assistant.busy} loading={assistant.loading} error={assistant.historyError} onDraft={draft} onChoose={(text, choice) => { voice.stop(); void assistant.send(text, 'text', choice); }}/><Composer voice={voice} busy={assistant.busy} onSend={send}/>
  </div>;
}
