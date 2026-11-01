"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import type { Asset } from "../../console/models";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/Button";
import { EndedSessions, RunningSessions, SavedConnections } from "./SessionsPanel";
import TerminalPicker from "./TerminalPicker";
import type { TerminalSessionLibrary } from "./useTerminalSessionLibrary";

type TerminalLandingProps = {
  assets: Asset[];
  connectedAgentIds: ReadonlySet<string>;
  lib: TerminalSessionLibrary;
  nameOf: (id: string) => string;
  onPick: (id: string) => void;
  onAddress: (address: string) => void;
  onSaveConnection: () => void;
  onSettings: () => void;
};

/**
 * Terminal with nothing open: "Connect to…" over running sessions, Recent and every device, then saved connections,
 * ended sessions and the devices that are offline. Choosing a row opens its shell in the workspace.
 */
export default function TerminalLanding({ assets, connectedAgentIds, lib, nameOf, onPick, onAddress, onSaveConnection, onSettings }: TerminalLandingProps) {
  const t = useTranslations("terminal");
  const filtering = lib.searchQuery.trim() !== "";
  return (
    <div className="tm-landing">
      <PageHeader
        title={t("title")}
        action={
          <div className="tm-acts">
            <Button onClick={() => onAddress("")}>{t("landing.address")}</Button>
            <Menu.Root>
              <Menu.Trigger className="tm-icon tm-acts-more" aria-label={t("landing.more")} title={t("landing.more")}>
                <MoreHorizontal aria-hidden="true" />
              </Menu.Trigger>
              <Menu.Portal>
                <Menu.Content className="lt-menu" align="end" sideOffset={6}>
                  <Menu.Item className="lt-menu-item" onSelect={onSaveConnection}>{t("landing.saveConnection")}</Menu.Item>
                  <Menu.Item className="lt-menu-item" onSelect={onSettings}>{t("landing.settings")}</Menu.Item>
                </Menu.Content>
              </Menu.Portal>
            </Menu.Root>
          </div>
        }
      />
      <TerminalPicker
        assets={assets}
        connectedAgentIds={connectedAgentIds}
        onPick={onPick}
        onAddress={onAddress}
        query={lib.searchQuery}
        onQueryChange={lib.setSearchQuery}
        autoFocus
        before={filtering ? null : <RunningSessions lib={lib} nameOf={nameOf} />}
        after={
          <>
            <SavedConnections lib={lib} nameOf={nameOf} />
            <EndedSessions lib={lib} nameOf={nameOf} />
          </>
        }
      />
    </div>
  );
}
