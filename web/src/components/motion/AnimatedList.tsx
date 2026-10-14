import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Collapsible } from './Collapsible';
import { exitDuration, reducedMotion } from './motion';

// Retain removed rows in their original positions until their height collapses.
export function AnimatedList<T>({ items, itemKey, renderItem, className = '' }: { items: T[]; itemKey: (item: T) => string | number; renderItem: (item: T) => ReactNode; className?: string }) {
  const keys = items.map(item => String(itemKey(item))); const signature = JSON.stringify(keys);
  const [rendered, setRendered] = useState(keys); const [previous, setPrevious] = useState(signature);
  const content = useRef(new Map<string, ReactNode>());
  const initialKeys = useRef(new Set(keys));
  useEffect(() => { initialKeys.current.clear(); }, []);
  items.forEach((item, index) => content.current.set(keys[index], renderItem(item)));
  if (signature !== previous) {
    const next = [...keys];
    if (!reducedMotion()) for (const key of rendered) if (!keys.includes(key)) {
      const following = rendered.slice(rendered.indexOf(key) + 1).find(candidate => next.includes(candidate));
      next.splice(following ? next.indexOf(following) : next.length, 0, key);
    }
    setPrevious(signature); setRendered(next);
  }
  useEffect(() => {
    if (rendered.every(key => keys.includes(key))) return;
    const timer = window.setTimeout(() => {
      setRendered(current => current.filter(key => keys.includes(key)));
      for (const key of content.current.keys()) if (!keys.includes(key)) content.current.delete(key);
    }, reducedMotion() ? 0 : exitDuration + 20);
    return () => window.clearTimeout(timer);
  }, [signature, rendered]);
  return <div className={`motion-list ${className}`}>{rendered.map(key => <Collapsible key={key} open={keys.includes(key)} animateInitial={!initialKeys.current.has(key)}>{content.current.get(key)}</Collapsible>)}</div>;
}
