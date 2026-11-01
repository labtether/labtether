import type { ReactNode } from "react";
import { DockerPanel } from "./DockerPanel";
import type { NodePanelRendererContext } from "./nodePanelRenderers";

export function renderDockerPanel(context: NodePanelRendererContext): ReactNode {
  return <DockerPanel hostId={context.dockerHostForPanel} sub={context.activeSub} onSub={context.replaceDockerSub} />;
}
