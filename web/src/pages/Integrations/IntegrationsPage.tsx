import { useState } from 'react';
import { CalendarDays, Send, Video, Link, Settings } from 'lucide-react';
import { PageHeader, SearchBox, Button, Empty } from '../../components/UI';
import { Dialog } from '../../components/Dialog';
const services = [
  { id: 'yandex', title: 'Яндекс Календарь', description: 'События и встречи в вашем Яндекс Календаре.', letter: 'Я', color: '#ff513f', calendar: true },
  { id: 'telegram', title: 'Telegram', description: 'Уведомления о задачах и встречах.', icon: Send, color: '#00a8e8', calendar: false },
  { id: 'google', title: 'Google Calendar', description: 'События вашего Google Календаря.', letter: '31', color: '#4285f4', calendar: true },
  { id: 'zoom', title: 'Zoom', description: 'Ссылки для ваших онлайн-встреч.', icon: Video, color: '#228aff', calendar: false },
  { id: 'notion', title: 'Notion', description: 'Задачи и заметки из Notion.', letter: 'N', color: '#151d31', calendar: false },
  { id: 'slack', title: 'Slack', description: 'Уведомления о важных событиях.', icon: Link, color: '#8b53bc', calendar: false },
];
export function IntegrationsPage() {
  const [query, setQuery] = useState(''); const [config, setConfig] = useState(false);
  const items = services.filter(item => item.title.toLowerCase().includes(query.toLowerCase().trim()));
  return <div className="page"><PageHeader title="Интеграции" subtitle="Подключите нужные сервисы"/><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Найти интеграцию"/></div><div className="integration-grid">{items.map(item => <section className={`integration-card ${item.id === 'yandex' ? 'prepared' : ''}`} key={item.id}><header><span className="service-logo" style={{ background: item.color }} aria-hidden="true">{item.icon ? <item.icon size={32}/> : item.letter}</span><div><h2>{item.title}</h2><span className="integration-state">{item.id === 'yandex' ? 'Не подключён' : 'Скоро'}</span></div></header><p>{item.description}</p>{item.id === 'yandex' ? <Button variant="soft" onClick={() => setConfig(true)}><Settings size={19}/>Настроить</Button> : <span className="coming-soon">Скоро</span>}</section>)}</div>{!items.length && <Empty>Интеграция не найдена</Empty>}
    {config && <Dialog title="Яндекс Календарь" onClose={() => setConfig(false)}><div className="integration-info"><CalendarDays size={40}/><span className="integration-state">Не подключён</span><p>Подключение будет доступно на этапе P2.</p><p className="muted">После подключения созданные в Ритм встречи появятся в вашем Яндекс Календаре.</p></div><footer className="dialog-actions"><Button onClick={() => setConfig(false)}>Понятно</Button></footer></Dialog>}
  </div>;
}
