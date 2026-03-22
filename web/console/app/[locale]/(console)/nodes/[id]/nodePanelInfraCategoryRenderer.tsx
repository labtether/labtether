import type { ReactNode } from "react";
import type { CategorySlug } from "../../../../console/taxonomy";
import { CategoryTab } from "./CategoryTab";
import { StorageOperationsTab } from "./StorageOperationsTab";
import type { NodePanelRendererContext } from "./nodePanelRenderers";

export function renderInfraCategoryPanel(context: NodePanelRendererContext): ReactNode {
  const panel = context.activePanel;
  if (!panel) return null;
  if (!context.infraCategories.has(panel as CategorySlug)) return null;

  if (panel === "storage" && context.isProxmoxAsset && context.effectiveKind === "node") {
    return (
      <StorageOperationsTab
        hostAsset={context.asset}
        proxmoxDetails={context.proxmoxDetailsTabProps.proxmoxDetails}
        proxmoxLoading={context.proxmoxDetailsTabProps.proxmoxLoading}
        proxmoxError={context.proxmoxDetailsTabProps.proxmoxError}
        onRetry={context.proxmoxDetailsTabProps.onRetry}
        onRunProxmoxAction={context.onRunStorageProxmoxAction}
        proxmoxActionRunning={context.proxmoxDetailsTabProps.proxmoxActionRunning}
        proxmoxActionMessage={context.proxmoxActionMessage}
        proxmoxActionError={context.proxmoxActionError}
        onOpenWorkloads={
          context.infraCategories.has("compute" as CategorySlug)
            ? () => context.openPanel("compute")
            : undefined
        }
      />
    );
  }

  const category = context.infraCategories.get(panel as CategorySlug);
  if (!category) return null;

  return (
    <CategoryTab
      category={category.def}
      hostAsset={context.asset}
    />
  );
}
