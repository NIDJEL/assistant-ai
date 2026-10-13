import { useState } from 'react';
import { UserRound, LogOut } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { Button } from '../../components/UI';
import { errorMessage } from '../../api/client';
export function AccountCard() {
 const { user, logout } = useAuth(); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [params] = useSearchParams();
 async function leave() { if (busy) return; setBusy(true); try { await logout(); } catch (e) { setError(errorMessage(e)); setBusy(false); } }
 return <section className="settings-card account-card"><h2><UserRound size={22}/>Аккаунт</h2><div className="account-profile"><span className="account-avatar">{user!.avatar_url ? <img src={user!.avatar_url} alt="" referrerPolicy="no-referrer"/> : user!.name.split(' ').map(part => part[0]).slice(0, 2).join('').toUpperCase()}</span><div><strong>{user!.name}</strong><p className="muted">{user!.email || 'Почта не указана'}</p></div></div><h3>Способы входа</h3><div className="account-methods">{[{ id: 'yandex', label: 'Яндекс' }, { id: 'vk', label: 'VK ID' }, { id: 'email', label: 'Email' }].map(({ id, label }) => <div key={id}><strong>{label}</strong>{user!.providers.includes(id) ? <span>Подключён</span> : id === 'email' ? <span>Не подключён</span> : <a href={`/api/auth/${id}/start?link=1`}>Подключить</a>}</div>)}</div>{params.get('linked') && <p role="status" className="saved">Способ входа подключён</p>}{(error || params.get('error')) && <p role="alert" className="auth-error">{error || (params.get('error') === 'oauth_unavailable' ? 'Этот способ входа ещё не настроен.' : 'Не удалось подключить способ входа. Попробуйте снова.')}</p>}<Button variant="soft" disabled={busy} onClick={() => void leave()}><LogOut size={18}/>{busy ? 'Выходим…' : 'Выйти'}</Button></section>;
}
