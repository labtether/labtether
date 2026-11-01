import type { Asset } from "../../../../console/models";
import type { DeviceOffers } from "./DeviceMenu";
import { deriveEffectiveKind, deriveShouldShowDesktop, isInfraAsset } from "./nodeDetailPageModelBuilders";

/**
 * What a Devices list row's ⋯ can open, by the same rules the device page uses for its panels: Terminal and Files
 * through a connected agent, Remote view on hosts and Proxmox guests (not containers, stacks or storage).
 */
export function rowOffers(asset: Asset, hasAgent: boolean): DeviceOffers {
  const proxmoxKind = (asset.metadata?.proxmox_type ?? "").toLowerCase();
  const desktop = deriveShouldShowDesktop({
    assetType: asset.type,
    isDockerContainer: asset.type === "docker-container" && asset.source === "docker",
    isInfra: isInfraAsset(asset),
    isDockerHost: asset.source === "docker" && asset.type === "container-host",
    isProxmoxAsset: asset.source === "proxmox",
    effectiveKind: deriveEffectiveKind(asset, proxmoxKind),
  });
  return { terminal: hasAgent, files: hasAgent, desktop };
}
