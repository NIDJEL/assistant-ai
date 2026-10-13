import { useId } from 'react';
export function BrandMark({ small = false, gradient = false }: { small?: boolean; gradient?: boolean }) {
  const id = useId();
  return <svg className={small ? 'brand-mark small' : 'brand-mark'} viewBox="0 0 64 42" aria-hidden="true">{gradient && <defs><linearGradient id={id}><stop stopColor="#0878ff"/><stop offset=".55" stopColor="#338bff"/><stop offset="1" stopColor="#9ac6ff"/></linearGradient></defs>}<path d="M6 31 Q26 -3 31 18 T58 12" fill="none" stroke={gradient ? `url(#${id})` : 'currentColor'} strokeWidth="8" strokeLinecap="round" /></svg>;
}
export function Mascot() {
  const id = useId();
  return <svg className="mascot" viewBox="0 0 390 190" aria-hidden="true">
    <defs><linearGradient id={id}><stop stopColor="#e6f3ff"/><stop offset="1" stopColor="#d4e4ff"/></linearGradient></defs>
    <path d="M390 5 C323 -14 307 77 250 50 C211 31 191 14 156 18 C95 22 54 75 71 126 C88 180 144 166 184 151 C227 135 258 158 291 169 C328 182 356 162 390 141" fill="none" stroke="#e1edff" strokeWidth="7"/>
    <path d="M390 20 C328 -10 307 98 250 75 C208 58 191 24 152 37 C104 54 90 91 107 123 C126 157 166 142 195 129 C231 113 261 142 291 145 C330 149 355 105 390 114 L390 158 C351 146 335 185 289 179 C254 175 227 147 197 154 C158 165 107 191 83 147 C58 103 89 38 139 25 C193 9 204 49 245 47 C286 47 300 -18 349 0 L390 0Z" fill={`url(#${id})`} opacity=".54"/>
    <path d="M114 113 Q132 126 140 108 M163 99 Q180 112 190 94" fill="none" stroke="#1425b0" strokeWidth="3" strokeLinecap="round"/>
  </svg>;
}
