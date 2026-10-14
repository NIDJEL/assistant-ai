import { useState } from 'react';
import { MoreHorizontal, Pencil, Trash2 } from 'lucide-react';
import { useEventActions } from '../../app/EventActionsContext';
import { IconButton } from '../../components/UI';
import type { RhythmEvent } from '../../types/api';
import { Presence, usePresent } from '../../components/motion/Presence';
function MenuSurface({ event, close }: { event: RhythmEvent; close: () => void }) {
  const present = usePresent(); const actions = useEventActions();
  return <><button className={`menu-dismiss ${present ? '' : 'closing'}`} aria-label="Закрыть меню" disabled={!present} onClick={close}/><div className={`row-menu ${present ? '' : 'closing'}`} inert={!present} aria-hidden={!present}><button onClick={() => { close(); actions.edit(event); }}><Pencil size={15}/>Редактировать</button><button onClick={() => { close(); actions.remove(event); }}><Trash2 size={15}/>Удалить</button></div></>;
}
export function EventMenu({ event }: { event: RhythmEvent }) {
  const [open, setOpen] = useState(false);
  return <div className="event-menu"><IconButton icon={MoreHorizontal} label={`Действия: ${event.title}`} aria-expanded={open} onClick={() => setOpen(!open)}/><Presence present={open}><MenuSurface event={event} close={() => setOpen(false)}/></Presence></div>;
}
