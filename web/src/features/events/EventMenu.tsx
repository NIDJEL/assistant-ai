import { useState } from 'react';
import { MoreHorizontal, Pencil, Trash2 } from 'lucide-react';
import { useEventActions } from '../../app/EventActionsContext';
import { IconButton } from '../../components/UI';
import type { RhythmEvent } from '../../types/api';
export function EventMenu({ event }: { event: RhythmEvent }) {
  const [open, setOpen] = useState(false); const actions = useEventActions();
  return <div className="event-menu"><IconButton icon={MoreHorizontal} label={`Действия: ${event.title}`} aria-expanded={open} onClick={() => setOpen(!open)}/>{open && <><button className="menu-dismiss" aria-label="Закрыть меню" onClick={() => setOpen(false)}/><div className="row-menu"><button onClick={() => { setOpen(false); actions.edit(event); }}><Pencil size={15}/>Редактировать</button><button onClick={() => { setOpen(false); actions.remove(event); }}><Trash2 size={15}/>Удалить</button></div></>}</div>;
}
