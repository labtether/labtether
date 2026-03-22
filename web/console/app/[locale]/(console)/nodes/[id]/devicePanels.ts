import { buildConnectorPanels } from "./devicePanelConnectorPanels";
import { buildCorePanels } from "./devicePanelCorePanels";
import type { PanelContext, PanelDef } from "./devicePanelTypes";

export type { ConnectionBadge, PanelContext, PanelDef } from "./devicePanelTypes";

export function buildAvailablePanels(ctx: PanelContext): PanelDef[] {
  return [
    ...buildCorePanels(ctx),
    ...buildConnectorPanels(ctx),
  ];
}
