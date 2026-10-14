import { useEffect, useState, type ReactNode } from 'react';
import { Presence, usePresent } from '../../components/motion/Presence';
import { reducedMotion } from '../../components/motion/motion';

const panelDuration = 440;

// Animate the grid and the whole fixed-width panel, rather than its individual cards.
export function useWidgetPanelMotion(visible: boolean) {
  const [previous, setPrevious] = useState(visible); const [moving, setMoving] = useState(false);
  if (previous !== visible) { setPrevious(visible); setMoving(!reducedMotion()); }
  useEffect(() => {
    if (!moving) return;
    const timer = window.setTimeout(() => setMoving(false), panelDuration);
    return () => window.clearTimeout(timer);
  }, [moving, visible]);
  return moving;
}

function PanelSurface({ integration, children }: { integration: boolean; children: ReactNode }) {
  const present = usePresent();
  return <aside className={`widgets-panel ${integration ? 'integration-panel' : ''}`} aria-label={integration ? 'Интеграция' : 'Виджеты'} data-open={present} aria-hidden={!present} inert={!present}>{children}</aside>;
}

export function WidgetRegion({ visible, integration, children }: { visible: boolean; integration: boolean; children: ReactNode }) {
  return <div className="widgets-region"><Presence present={visible} duration={panelDuration}><PanelSurface integration={integration}>{children}</PanelSurface></Presence></div>;
}
