import { useLayoutEffect, useRef, type TextareaHTMLAttributes } from 'react';

export function AutoTextarea({ value, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const element = ref.current!;
    function resize() { element.style.height = '0px'; element.style.height = `${element.scrollHeight + element.offsetHeight - element.clientHeight}px`; }
    resize(); let width = element.clientWidth;
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => { if (element.clientWidth !== width) { width = element.clientWidth; resize(); } });
    observer?.observe(element); return () => observer?.disconnect();
  }, [value]);
  return <textarea {...props} ref={ref} value={value} className={`auto-textarea ${props.className || ''}`}/>;
}
