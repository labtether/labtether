import type { ReactNode } from "react";
import { DockerInspectTab } from "./DockerInspectTab";
import { DockerLogsTab } from "./DockerLogsTab";
import { DockerStackContainersTab } from "./DockerStackContainersTab";
import { DockerStackInspectTab } from "./DockerStackInspectTab";
import { DockerStatsTab } from "./DockerStatsTab";
import { NodeLogsTabCard } from "./NodeLogsTabCard";
import type { NodePanelRendererContext } from "./nodePanelRenderers";

export function renderDockerLogsPanel(context: NodePanelRendererContext): ReactNode {
  if (context.isDockerContainer) {
    return <DockerLogsTab containerId={context.dockerContainerId} />;
  }
  return <NodeLogsTabCard {...context.nodeLogsTabCardProps} />;
}

export function renderDockerStatsPanel(context: NodePanelRendererContext): ReactNode {
  if (!context.isDockerContainer) return null;
  return <DockerStatsTab containerId={context.dockerContainerId} />;
}

export function renderDockerInspectPanel(context: NodePanelRendererContext): ReactNode {
  if (context.isDockerContainer) {
    return <DockerInspectTab containerId={context.dockerContainerId} />;
  }
  if (context.isDockerStack) {
    return <DockerStackInspectTab hostId={context.dockerStackHostId} stackName={context.dockerStackName} />;
  }
  return null;
}

export function renderDockerStackContainersPanel(context: NodePanelRendererContext): ReactNode {
  if (!context.isDockerStack) return null;
  return <DockerStackContainersTab hostId={context.dockerStackHostId} stackName={context.dockerStackName} />;
}
