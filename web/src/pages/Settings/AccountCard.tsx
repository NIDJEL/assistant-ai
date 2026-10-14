import { useEffect, useRef, useState } from 'react';
import { UserRound, LogOut, Upload, Trash2 } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { useData } from '../../app/DataContext';
import { usePreferences } from '../../app/PreferencesContext';
import { Avatar } from '../../components/Avatar';
import { Button } from '../../components/UI';
import { errorMessage } from '../../api/client';

async function avatarData(file: File) {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) throw new Error('Выберите изображение JPG, PNG или WebP.');
  if (file.size > 5 * 1024 * 1024) throw new Error('Размер фото должен быть не больше 5 МБ.');
  let bitmap: ImageBitmap;
  try { bitmap = await createImageBitmap(file); } catch { throw new Error('Не удалось прочитать фото. Выберите другое изображение.'); }
  try {
    const canvas = document.createElement('canvas'); canvas.width = 256; canvas.height = 256;
    const context = canvas.getContext('2d'); if (!context) throw new Error('Не удалось обработать фото.');
    const side = Math.min(bitmap.width, bitmap.height);
    context.fillStyle = '#fff'; context.fillRect(0, 0, 256, 256);
    context.drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, 256, 256);
    const data = canvas.toDataURL('image/jpeg', .8);
    return data.length <= 54000 ? data : canvas.toDataURL('image/jpeg', .6);
  } finally { bitmap.close(); }
}

export function AccountCard() {
  const { user, logout, updateProfile } = useAuth(); const { health } = useData(); const { update } = usePreferences();
  const [name, setName] = useState(user!.name); const [busy, setBusy] = useState(''); const [error, setError] = useState(''); const [saved, setSaved] = useState(''); const [params] = useSearchParams();
  const upload = useRef<HTMLInputElement>(null); const locked = useRef(false);
  useEffect(() => setName(user!.name), [user!.name]);
  async function save(action: 'name' | 'avatar' | 'remove', file?: File) {
    if (locked.current) return; locked.current = true; setBusy(action); setError(''); setSaved('');
    try {
      if (action === 'name') { await updateProfile({ name: name.trim() }); update({ name: name.trim() }); setSaved('Имя сохранено'); }
      else { await updateProfile({ avatar_url: file ? await avatarData(file) : '' }); setSaved(file ? 'Фото сохранено' : 'Фото удалено'); }
    } catch (e) { setError(errorMessage(e)); }
    finally { locked.current = false; setBusy(''); }
  }
  async function leave() { if (locked.current) return; locked.current = true; setBusy('logout'); setError(''); try { await logout(); } catch (e) { setError(errorMessage(e)); locked.current = false; setBusy(''); } }
  return <section className="settings-card account-card"><h2><UserRound size={22}/>Аккаунт</h2>
    <div className="account-profile"><Avatar name={user!.name} url={user!.avatar_url} className="account-avatar"/><div><strong>{user!.name}</strong><p className="muted">{user!.email || 'Почта не указана'}</p></div></div>
    <div className="account-avatar-actions"><input ref={upload} type="file" className="sr-only" tabIndex={-1} aria-label="Фото профиля" accept="image/jpeg,image/png,image/webp" disabled={!!busy} onChange={event => { const file = event.target.files?.[0]; event.target.value = ''; if (file) void save('avatar', file); }}/><Button variant="soft" disabled={!!busy} onClick={() => upload.current?.click()}><Upload size={17}/>{busy === 'avatar' ? 'Загружаю…' : user!.avatar_url ? 'Изменить фото' : 'Загрузить фото'}</Button>{user!.avatar_url && <Button variant="ghost" disabled={!!busy} onClick={() => void save('remove')}><Trash2 size={17}/>Удалить фото</Button>}</div>
    <p className="muted account-photo-hint">JPG, PNG или WebP · до 5 МБ</p>
    <form onSubmit={event => { event.preventDefault(); void save('name'); }}><label>Ваше имя<input value={name} onChange={event => { setName(event.target.value); setSaved(''); }} required maxLength={100} disabled={!!busy}/></label><Button type="submit" disabled={!!busy || !name.trim() || name.trim() === user!.name}>{busy === 'name' ? 'Сохраняю…' : 'Сохранить имя'}</Button></form>
    {saved && <p className="saved" role="status">{saved}</p>}
    <p className="muted account-timezone">Часовой пояс: {health!.timezone}</p>
    <h3>Способы входа</h3><div className="account-methods">{[{ id: 'yandex', label: 'Яндекс' }, { id: 'vk', label: 'VK ID' }, { id: 'email', label: 'Email' }].map(({ id, label }) => <div key={id}><strong>{label}</strong>{user!.providers.includes(id) ? <span>Подключён</span> : id === 'email' ? <span>Не подключён</span> : <a href={`/api/auth/${id}/start?link=1`}>Подключить</a>}</div>)}</div>
    {params.get('linked') && <p role="status" className="saved">Способ входа подключён</p>}{(error || params.get('error')) && <p role="alert" className="auth-error">{error || (params.get('error') === 'oauth_unavailable' ? 'Этот способ входа ещё не настроен.' : 'Не удалось подключить способ входа. Попробуйте снова.')}</p>}
    <Button variant="danger" disabled={!!busy} onClick={() => void leave()}><LogOut size={18}/>{busy === 'logout' ? 'Выходим…' : 'Выйти'}</Button>
  </section>;
}
