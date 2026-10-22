import type { Page } from '@playwright/test';

/** A targeted text/surface audit; illustrations, gradients and disabled controls are reviewed visually. */
export async function themeContrastIssues(page: Page) {
  return page.evaluate(() => {
    type Color = [number, number, number, number];
    const parse = (value: string): Color | null => {
      const parts = value.match(/^rgba?\(([^)]+)\)$/)?.[1].split(/[\s,/]+/).filter(Boolean).map(Number);
      return parts && parts.length >= 3 ? [parts[0], parts[1], parts[2], parts[3] ?? 1] : null;
    };
    const blend = (top: Color, bottom: Color): Color => {
      const alpha = top[3] + bottom[3] * (1 - top[3]);
      return [0, 1, 2].map(i => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / (alpha || 1)).concat(alpha) as Color;
    };
    const luminance = (color: Color) => color.slice(0, 3).map(value => { const v = value / 255; return v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4; }).reduce((sum, value, index) => sum + value * [.2126, .7152, .0722][index], 0);
    const problems: { text: string; selector: string; ratio: number; foreground: string; background: Color }[] = [];
    for (const node of document.querySelectorAll<HTMLElement>('h1,h2,h3,p,span,small,button,a,label,legend,time,strong,dt,dd,input,textarea')) {
      const text = node instanceof HTMLInputElement || node instanceof HTMLTextAreaElement ? node.value || node.placeholder : [...node.childNodes].filter(child => child.nodeType === Node.TEXT_NODE).map(child => child.textContent).join('').trim();
      // Selected auth tabs have a separate animated blue underlay, not an ancestor background.
      if (!text || node.closest('[aria-hidden="true"],:disabled') || node.matches('input[type="radio"],input[type="checkbox"],.auth-segment [aria-selected="true"]')) continue;
      const rect = node.getBoundingClientRect(); const style = getComputedStyle(node);
      if (!rect.width || !rect.height || rect.bottom < 0 || rect.top > innerHeight || style.visibility === 'hidden' || Number(style.opacity) < .95) continue;
      const foreground = parse(style.color); if (!foreground) continue;
      let background: Color = [0, 0, 0, 0]; let skip = false;
      for (let ancestor: HTMLElement | null = node; ancestor; ancestor = ancestor.parentElement) {
        const s = getComputedStyle(ancestor);
        if (Number(s.opacity) < .95) { skip = true; break; }
        if (ancestor.matches('button,.theme-option') && s.backgroundImage !== 'none') { skip = true; break; }
        const layer = parse(s.backgroundColor); if (layer) background = blend(background, layer);
        if (background[3] >= .999) break;
      }
      if (skip) continue;
      background = blend(background, [255, 255, 255, 1]);
      const rendered = blend(foreground, background); const light = luminance(rendered); const dark = luminance(background);
      const ratio = (Math.max(light, dark) + .05) / (Math.min(light, dark) + .05);
      const large = parseFloat(style.fontSize) >= 24 || parseFloat(style.fontSize) >= 18.66 && Number(style.fontWeight) >= 700;
      if (ratio < (large ? 3 : 4.5) - .05) problems.push({ text: text.slice(0, 65), selector: `${node.tagName.toLowerCase()}.${node.className}`, ratio: Math.round(ratio * 100) / 100, foreground: style.color, background });
    }
    return problems;
  });
}
