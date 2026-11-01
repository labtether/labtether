"use client";

import { FolderOpen, Monitor, SquareTerminal } from "lucide-react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import type { Asset } from "../../../../console/models";
import { useAuth } from "../../../../contexts/AuthContext";
import { canAccessHostData } from "../../../../lib/roles";
import { DeviceMenu, type DeviceOffers } from "./DeviceMenu";
import { panelHref } from "./deviceTabsModel";

/** A desktop OS, where Remote view is the second button instead of Files. */
const isDesktopOS = (asset: Asset) => /windows|darwin|mac/i.test(`${asset.platform ?? ""} ${asset.metadata?.os_name ?? ""}`);

/**
 * Two connect buttons at most, then ⋯ (which holds the rest): Terminal, and Remote view on a desktop or a guest
 * without an agent, else Files.
 */
export function DeviceHeaderActions({ asset, offers, onPower }: { asset: Asset; offers: DeviceOffers; onPower?: (action: "restart" | "shutdown") => void }) {
  const t = useTranslations("devices.page.menu");
  const { user } = useAuth();
  const hostData = canAccessHostData(user?.role);
  const shown: Array<keyof DeviceOffers> = [];
  if (hostData) {
    if (offers.terminal) shown.push("terminal");
    if (offers.desktop && (isDesktopOS(asset) || !offers.terminal)) shown.push("desktop");
    else if (offers.files) shown.push("files");
  }
  const icons = { terminal: SquareTerminal, files: FolderOpen, desktop: Monitor };

  return (
    <>
      {shown.map((k) => {
        const Icon = icons[k];
        return (
          <Link key={k} href={panelHref(asset.id, k)} className="dv-btn">
            <Icon aria-hidden="true" strokeWidth={1.75} />
            {t(k)}
          </Link>
        );
      })}
      <DeviceMenu asset={asset} offers={offers} skip={shown} onPower={onPower} />
    </>
  );
}
