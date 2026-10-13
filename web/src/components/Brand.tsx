import { useId } from 'react';
export function BrandMark({ small = false }: { small?: boolean }) {
  return <svg className={small ? 'brand-mark small' : 'brand-mark'} viewBox="0 0 64 42" aria-hidden="true"><path d="M6 31 Q26 -3 31 18 T58 12" fill="none" stroke="currentColor" strokeWidth="8" strokeLinecap="round" /></svg>;
}
export function Mascot() {
  const id = useId();
  return <svg className="mascot" viewBox="0 0 390 190" aria-hidden="true">
    <defs><linearGradient id={id}><stop stopColor="#e6f3ff"/><stop offset="1" stopColor="#d4e4ff"/></linearGradient></defs>
    <path d="M380 14 C295 -20 286 86 230 38 C158 -21 61 31 66 98 C69 167 154 158 197 143 C272 117 296 193 390 166" fill="none" stroke="#eff6ff" strokeWidth="10"/>
    <path d="M385 35 C314 6 298 109 241 63 C173 8 92 42 89 100 C87 151 158 132 190 125 C265 108 301 170 390 140 L390 105 C319 135 287 59 232 101 C190 135 121 127 131 83 C143 41 182 62 220 84 C277 129 316 46 387 66Z" fill={`url(#${id})`} opacity=".7"/>
    <path d="M131 97 Q148 111 156 91 M178 85 Q194 99 204 78" fill="none" stroke="#2c43b7" strokeWidth="3" strokeLinecap="round"/>
  </svg>;
}
