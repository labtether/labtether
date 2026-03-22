"use client";

import type { ComponentProps } from "react";

import { Card } from "../../../../components/ui/Card";
import {
  RemoteViewerShell,
  type RemoteViewerShellProps,
} from "../../../../components/RemoteViewerShell";
import { DesktopConnectionControlsCard } from "./DesktopConnectionControlsCard";

export interface DesktopRemoteViewSectionProps {
  controlsCardProps: ComponentProps<typeof DesktopConnectionControlsCard>;
  protocolSwitchNotice?: string | null;
  remoteViewerShellProps: RemoteViewerShellProps;
}

export function DesktopRemoteViewSection({
  controlsCardProps,
  protocolSwitchNotice,
  remoteViewerShellProps,
}: DesktopRemoteViewSectionProps) {
  return (
    <>
      <DesktopConnectionControlsCard {...controlsCardProps} />
      {protocolSwitchNotice ? (
        <Card className="mb-4 py-2 text-sm text-[var(--warn)]">
          {protocolSwitchNotice}
        </Card>
      ) : null}
      <RemoteViewerShell {...remoteViewerShellProps} />
    </>
  );
}
