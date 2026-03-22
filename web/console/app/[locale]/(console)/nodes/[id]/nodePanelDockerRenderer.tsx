import type { ReactNode } from "react";
import { Card } from "../../../../components/ui/Card";
import { SegmentedTabs } from "../../../../components/ui/SegmentedTabs";
import { DockerContainersTab } from "./DockerContainersTab";
import { DockerImagesTab } from "./DockerImagesTab";
import { DockerStacksTab } from "./DockerStacksTab";
import type { NodePanelRendererContext } from "./nodePanelRenderers";

export function renderDockerPanel(context: NodePanelRendererContext): ReactNode {
  return (
    <>
      <Card className="mb-4 flex items-center justify-between py-2">
        <SegmentedTabs
          value={context.activeSub ?? "containers"}
          options={[
            { id: "containers", label: "Containers" },
            { id: "stacks", label: "Stacks" },
            { id: "images", label: "Images" },
          ]}
          onChange={context.replaceDockerSub}
        />
      </Card>
      <div className="mt-3">
        {(context.activeSub ?? "containers") === "containers" && (
          <DockerContainersTab hostId={context.dockerHostForPanel} />
        )}
        {context.activeSub === "stacks" && <DockerStacksTab hostId={context.dockerHostForPanel} />}
        {context.activeSub === "images" && <DockerImagesTab hostId={context.dockerHostForPanel} />}
      </div>
    </>
  );
}
