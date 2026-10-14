import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { DndContext, DragOverlay, KeyboardSensor, PointerSensor, closestCorners, pointerWithin, useSensor, useSensors, type DragEndEvent, type KeyboardCoordinateGetter, type CollisionDetection } from '@dnd-kit/core';
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, Plus, Settings, Trash2, ChevronRight } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { usePreferences, type WidgetKey, type WidgetLayout } from '../../app/PreferencesContext';
import { widgetRegistry } from './Widgets';
import { Dialog } from '../../components/Dialog';
import { Presence } from '../../components/motion/Presence';
import { Button, IconButton } from '../../components/UI';
import { reducedMotion } from '../../components/motion/motion';
import { useWidgetMotion } from './useWidgetMotion';
export const widgetTitles: Record<WidgetKey, string> = { calendar: 'Календарь', today: 'Сегодня', tasks: 'Задачи', meetings: 'Ближайшие встречи', progress: 'Мой прогресс', quick: 'Быстрое планирование', tags: 'Метки' };
const defaults: Record<string, WidgetKey[]> = { '/tasks': ['progress', 'tasks', 'tags'], '/meetings': ['calendar', 'today', 'quick'], '/reminders': ['calendar', 'today'], '/archive': ['progress', 'calendar'], '/settings': ['progress', 'meetings'] };
const FrameContext = createContext<{ handle?: ReactNode; configure?: () => void; limit: number; taskTitle?: string }>({ limit: 3 });
export function useWidgetFrame() { return useContext(FrameContext); }
function SortableWidget({ type, limit, configure, taskTitle }: { type: WidgetKey; limit: number; configure: () => void; taskTitle?: string }) {
  const { setNodeRef, setActivatorNodeRef, attributes, listeners, transform, transition, isDragging } = useSortable({ id: type, transition: { duration: 440, easing: 'cubic-bezier(.22,1,.36,1)' } });
  const Component = widgetRegistry[type];
  return <div ref={setNodeRef} data-widget={type} className={isDragging ? 'widget-placeholder' : ''} style={{ transform: CSS.Transform.toString(transform), transition }}><FrameContext value={{ limit, configure, taskTitle, handle: <button ref={setActivatorNodeRef} className="drag-handle" {...attributes} {...listeners} aria-label={`Переместить: ${widgetTitles[type]}`}><GripVertical size={19}/></button> }}><Component/></FrameContext></div>;
}
export function WidgetPanel({ drawer = false, onCollapse }: { drawer?: boolean; onCollapse?: () => void }) {
  const { preferences, update } = usePreferences(); const page = useLocation().pathname;
  const raw = preferences.layouts[page]; const layout: WidgetLayout = raw || { order: defaults[page] || ['calendar', 'today', 'tasks'], hidden: [], settings: {} };
  const order = [...new Set(layout.order)].filter(type => type in widgetRegistry); const visible = order.filter(type => !layout.hidden.includes(type));
  const [picker, setPicker] = useState(false); const [editing, setEditing] = useState<WidgetKey | null>(null); const [active, setActive] = useState<WidgetKey | null>(null);
  const keyboardTarget = useRef<WidgetKey | null>(null);
  const dragLayer = useRef<HTMLDivElement>(null);
  // A viewport-positioned overlay must escape transformed, scrolling ancestors.
  // The popover top layer also keeps it above a modal widget drawer.
  useLayoutEffect(() => {
    const layer = dragLayer.current;
    if (!layer || typeof layer.showPopover !== 'function') return;
    if (active) { layer.showPopover(); return; }
    const timer = setTimeout(() => layer.hidePopover(), reducedMotion() ? 0 : 440);
    return () => clearTimeout(timer);
  }, [active]);
  const list = useRef<HTMLDivElement>(null); useWidgetMotion(list, !!active);
  useEffect(() => { setPicker(false); setEditing(null); }, [page]);
  // Unequal card heights need logical adjacent moves, rather than a distance jump.
  const keyboardCoordinates: KeyboardCoordinateGetter = (event, { context }) => {
    const direction = event.code === 'ArrowDown' ? 1 : event.code === 'ArrowUp' ? -1 : 0;
    if (!direction || !context.active) return;
    event.preventDefault();
    const index = visible.indexOf((keyboardTarget.current || context.active.id) as WidgetKey);
    const target = visible[index + direction]; const rect = context.droppableRects.get(target);
    if (!target || !rect) return;
    keyboardTarget.current = target;
    return { x: rect.left, y: rect.top };
  };
  const collision: CollisionDetection = args => {
    if (keyboardTarget.current) return [{ id: keyboardTarget.current }];
    const underPointer = pointerWithin(args);
    return underPointer.length ? underPointer : closestCorners(args);
  };
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), useSensor(KeyboardSensor, { coordinateGetter: keyboardCoordinates }));
  const save = (value: WidgetLayout) => update({ layouts: { ...preferences.layouts, [page]: value } });
  function add(type: WidgetKey) { save({ ...layout, order: order.includes(type) ? order : [...order, type], hidden: layout.hidden.filter(key => key !== type) }); }
  function end(event: DragEndEvent) {
    setActive(null); keyboardTarget.current = null; if (!event.over || event.active.id === event.over.id) return;
    // Reorder the full list, retaining the positions of hidden cards.
    save({ ...layout, order: arrayMove(order, order.indexOf(event.active.id as WidgetKey), order.indexOf(event.over.id as WidgetKey)) });
  }
  const ActiveComponent = active ? widgetRegistry[active] : null;
  return <><div className={`widgets-heading ${onCollapse ? 'has-toggle' : ''}`}>{onCollapse && <IconButton icon={ChevronRight} label="Скрыть виджеты" className="widget-panel-toggle" onClick={onCollapse}/>}<div><h2>Виджеты</h2></div><IconButton icon={Settings} label="Настройки виджетов" onClick={() => setPicker(true)}/><IconButton icon={Plus} label="Добавить виджет" onClick={() => setPicker(true)}/><p>Настройте панель под себя</p></div><DndContext sensors={sensors} collisionDetection={collision} onDragStart={event => { keyboardTarget.current = null; setActive(event.active.id as WidgetKey); }} onDragCancel={() => { keyboardTarget.current = null; setActive(null); }} onDragEnd={end}><SortableContext items={visible} strategy={verticalListSortingStrategy}><div ref={list} className="widget-list">{visible.map(type => <SortableWidget key={type} type={type} limit={layout.settings[type]?.limit || 3} taskTitle={page === '/tasks' ? 'Ближайшие задачи' : undefined} configure={() => setEditing(type)}/>)}</div></SortableContext>{createPortal(<div ref={dragLayer} className="widget-drag-layer" popover="manual" aria-hidden="true"><DragOverlay dropAnimation={{ duration: reducedMotion() ? 0 : 440, easing: 'cubic-bezier(.22,1,.36,1)' }}>{ActiveComponent && <div className="widget-drag-overlay" inert><FrameContext value={{ limit: layout.settings[active!]?.limit || 3, taskTitle: page === '/tasks' ? 'Ближайшие задачи' : undefined, handle: <GripVertical size={19}/> }}><ActiveComponent/></FrameContext></div>}</DragOverlay></div>, document.body)}</DndContext>
    <button className="widget-drop-zone" onClick={() => setPicker(true)}><Plus size={27}/><span>Добавить виджет<br/>в панель</span></button>
    <Presence present={picker}>{picker && <Dialog title="Ваши виджеты" onClose={() => setPicker(false)}><p className="muted">Для этой страницы. Перемещайте карточки за ручку.</p><div className="widget-picker">{(Object.keys(widgetRegistry) as WidgetKey[]).map(type => <div key={type}><strong>{widgetTitles[type]}</strong>{visible.includes(type) ? <Button variant="ghost" onClick={() => save({ ...layout, hidden: [...layout.hidden, type] })}><Trash2 size={16}/>Убрать</Button> : <Button variant="soft" onClick={() => add(type)}><Plus size={16}/>Добавить</Button>}</div>)}</div></Dialog>}</Presence>
    <Presence present={!!editing}>{editing && <Dialog title={`Настройки: ${widgetTitles[editing]}`} onClose={() => setEditing(null)}><label className="widget-limit">Количество записей<select value={layout.settings[editing]?.limit || 3} disabled={['calendar', 'progress', 'quick'].includes(editing)} onChange={e => save({ ...layout, settings: { ...layout.settings, [editing]: { limit: Number(e.target.value) } } })}>{[3, 4, 5].map(n => <option key={n} value={n}>{n}</option>)}</select></label><footer className="dialog-actions"><Button variant="danger" onClick={() => { save({ ...layout, hidden: [...layout.hidden, editing] }); setEditing(null); }}><Trash2 size={17}/>Убрать виджет</Button><Button onClick={() => setEditing(null)}>Готово</Button></footer></Dialog>}</Presence>
  </>;
}
