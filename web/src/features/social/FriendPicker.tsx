import { useState } from 'react';
import { useSocial } from './SocialContext';
import { Avatar } from '../../components/Avatar';
import { SearchBox } from '../../components/UI';
export function FriendPicker({ value, onChange, disabled = false, exclude = [] }: { value: string[]; onChange: (ids: string[]) => void; disabled?: boolean; exclude?: string[] }) {
  const { people, error } = useSocial(); const [query, setQuery] = useState('');
  const friends = (people?.friends || []).filter(p => !exclude.includes(p.id) && `${p.name} ${p.email || ''} ${p.tags.join(' ')}`.toLocaleLowerCase('ru').includes(query.toLocaleLowerCase('ru')));
  return <fieldset className="friend-picker"><legend>Пригласить друзей{value.length > 0 && ` · ${value.length}`}</legend><SearchBox value={query} onChange={setQuery} label="Поиск друзей" placeholder="Имя или тег друга"/>{error && <p role="alert" className="muted">{error}</p>}<div className="friend-picker-list">{friends.map(p => <label key={p.id}><input type="checkbox" checked={value.includes(p.id)} disabled={disabled} onChange={e => onChange(e.target.checked ? [...value, p.id] : value.filter(id => id !== p.id))}/><Avatar name={p.name} url={p.avatar_url}/><span><strong>{p.name}</strong><small>{p.tags.join(' · ') || p.email}</small></span></label>)}{!friends.length && <p className="muted">{query ? 'Друзья не найдены' : 'Добавьте друзей по персональному коду в разделе «Друзья».'}</p>}</div><p className="field-hint">Приглашённые сами подтверждают участие. Их личные события остаются приватными.</p></fieldset>;
}
