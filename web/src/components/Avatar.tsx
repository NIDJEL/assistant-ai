import { useEffect, useState } from 'react';

export function Avatar({ name, url, className = 'avatar' }: { name: string; url: string; className?: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  const initials = name.trim().split(/\s+/).map(part => part[0]).slice(0, 2).join('').toUpperCase();
  return <span className={className} aria-hidden="true">{url && !failed ? <img src={url} alt="" referrerPolicy="no-referrer" onError={() => setFailed(true)}/> : initials}</span>;
}
