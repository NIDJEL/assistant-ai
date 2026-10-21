import { useEffect, useRef, useState } from 'react';
import { UserRound, LogOut, Upload, Trash2, Mail, Link2, CheckCircle2 } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { usePreferences } from '../../app/PreferencesContext';
import { Avatar } from '../../components/Avatar';
import { Button } from '../../components/UI';
import { errorMessage } from '../../api/client';
import { Presence, usePresent } from '../../components/motion/Presence';

function EmailPasswordForm({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const { enableEmailLogin } = useAuth(); const present = usePresent();
  const [password, setPassword] = useState(''); const [confirmation, setConfirmation] = useState(''); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const locked = useRef(false);
  async function submit() {
    if (locked.current || !present) return;
    if (password !== confirmation) { setError('Пароли не совпадают'); return; }
    locked.current = true; setBusy(true); setError('');
    try { await enableEmailLogin(password, confirmation); setPassword(''); setConfirmation(''); onSaved(); }
    catch (e) { setError(errorMessage(e)); }
    finally { locked.current = false; setBusy(false); }
  }
  return <form className={`account-password-form ${present ? '' : 'closing'}`} aria-label="Пароль для входа по почте" inert={!present} onSubmit={event => { event.preventDefault(); void submit(); }}>
    <p>Создайте пароль для входа в Гену по вашей почте. Пароль от Яндекса вводить не нужно.</p>
    <div className="form-grid"><label>Новый пароль<input type="password" autoComplete="new-password" minLength={10} maxLength={1024} required value={password} disabled={busy} onChange={event => { setPassword(event.target.value); setError(''); }} aria-describedby="email-password-hint"/></label><label>Повторите новый пароль<input type="password" autoComplete="new-password" minLength={10} maxLength={1024} required value={confirmation} disabled={busy} onChange={event => { setConfirmation(event.target.value); setError(''); }}/></label></div>
    <p id="email-password-hint">От 10 символов.</p>{error && <p role="alert" className="auth-error">{error}</p>}
    <div className="account-password-actions"><Button variant="ghost" disabled={busy} onClick={onClose}>Отмена</Button><Button type="submit" disabled={busy}>{busy ? 'Сохраняю…' : 'Сохранить пароль'}</Button></div>
  </form>;
}

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
  const { user, logout, updateProfile } = useAuth(); const { update } = usePreferences();
  const [settingPassword, setSettingPassword] = useState(false);
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
    <div className="settings-row"><div className="account-profile"><Avatar name={user!.name} url={user!.avatar_url} className="account-avatar"/><div><strong>{user!.name}</strong><p className="muted">{user!.email || 'Почта не указана'}</p></div></div>
    <div className="account-photo-controls"><div className="account-avatar-actions"><input ref={upload} type="file" className="sr-only" tabIndex={-1} aria-label="Фото профиля" accept="image/jpeg,image/png,image/webp" disabled={!!busy} onChange={event => { const file = event.target.files?.[0]; event.target.value = ''; if (file) void save('avatar', file); }}/><Button variant="soft" disabled={!!busy} onClick={() => upload.current?.click()}><Upload size={17}/>{busy === 'avatar' ? 'Загружаю…' : user!.avatar_url ? 'Изменить фото' : 'Загрузить фото'}</Button>{user!.avatar_url && <Button variant="ghost" disabled={!!busy} onClick={() => void save('remove')}><Trash2 size={17}/>Удалить фото</Button>}</div>
    <p className="muted account-photo-hint">JPG, PNG или WebP · до 5 МБ</p></div></div>
    <form className="settings-row account-name-row" onSubmit={event => { event.preventDefault(); void save('name'); }}><div className="settings-row-label"><label htmlFor="account-name">Ваше имя</label><p>Так вас видят друзья и участники встреч.</p></div><div className="settings-row-control"><input id="account-name" value={name} onChange={event => { setName(event.target.value); setSaved(''); }} required maxLength={100} disabled={!!busy}/><Button type="submit" disabled={!!busy || !name.trim() || name.trim() === user!.name}>{busy === 'name' ? 'Сохраняю…' : 'Сохранить имя'}</Button></div></form>
    {saved && <p className="saved" role="status">{saved}</p>}
    <h3>Способы входа</h3><div className="account-methods">
      <div className="settings-row"><div className="settings-row-label"><strong><Link2 size={18}/>Яндекс</strong><p>Вход через ваш аккаунт Яндекса.</p></div><div className="settings-row-control">{user!.providers.includes('yandex') ? <span className="connection-status connected"><CheckCircle2 size={16}/>Подключён</span> : <a className="button soft" href="/api/auth/yandex/start?link=1">Подключить Яндекс</a>}</div></div>
      <div className="settings-row"><div className="settings-row-label"><strong><Mail size={18}/>Email</strong><p>{user!.email || 'Почта не передана Яндексом'}{user!.email && !user!.providers.includes('email') && <><br/>Для входа по почте задайте пароль.</>}</p></div><div className="settings-row-control">{user!.email && <span className="connection-status connected"><CheckCircle2 size={16}/>Подключена</span>}{user!.email && user!.providers.includes('yandex') && !user!.providers.includes('email') && <Button variant="soft" disabled={settingPassword || !!busy} onClick={() => { setSaved(''); setSettingPassword(true); }}>Задать пароль</Button>}</div></div>
    </div>
    <Presence present={settingPassword}>{settingPassword && <EmailPasswordForm onClose={() => setSettingPassword(false)} onSaved={() => { setSettingPassword(false); setSaved('Вход по почте подключён'); }}/>}</Presence>
    {params.get('linked') && <p role="status" className="saved">Способ входа подключён</p>}{(error || params.get('error')) && <p role="alert" className="auth-error">{error || (params.get('error') === 'oauth_unavailable' ? 'Этот способ входа ещё не настроен.' : 'Не удалось подключить способ входа. Попробуйте снова.')}</p>}
    <div className="settings-row"><div className="settings-row-label"><strong>Сессия</strong><p>Вы вошли в Гену на этом устройстве.</p></div><div className="settings-row-control"><Button variant="danger" disabled={!!busy} onClick={() => void leave()}><LogOut size={18}/>{busy === 'logout' ? 'Выходим…' : 'Выйти'}</Button></div></div>
  </section>;
}
