import { passwordError } from '../app/accountValidation';
import '../styles/password-strength.css';

export function PasswordStrength({ password, id }: { password: string; id: string }) {
  const error = passwordError(password);
  const length = [...password].filter(char => !/\p{White_Space}/u.test(char)).length;
  const level = !password ? 0 : error ? 1 : length >= 16 ? 3 : 2;
  return <div id={id} className="password-strength" data-level={level}>
    {!!password && <><div className="password-strength-bars" aria-hidden="true"><span/><span/><span/></div><p className="password-strength-label" aria-live="polite">{error ? 'Пока слабый' : level === 3 ? 'Хороший запас длины' : 'Подходит'}</p></>}
    <p className="password-strength-hint">{password && error ? error : 'От 10 символов без учёта пробелов. Буквы и цифра или символ; можно фразу из четырёх разных слов.'}</p>
  </div>;
}
