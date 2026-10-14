import { type ReactNode } from 'react';
import { Presence, usePresent } from '../../components/motion/Presence';

function PanelSurface({ integration, children }: { integration: boolean; children: ReactNode }) {
  const present = usePresent();
  return <aside className={`widgets-panel ${integration ? 'integration-panel' : ''}`} aria-label={integration ? 'Интеграция' : 'Виджеты'} data-open={present} aria-hidden={!present} inert={!present}>{children}</aside>;
}

export function WidgetRegion({ visible, integration, children }: { visible: boolean; integration: boolean; children: ReactNode }) {
  return <div className="widgets-region"><Presence present={visible}><PanelSurface integration={integration}>{children}</PanelSurface></Presence></div>;
}
