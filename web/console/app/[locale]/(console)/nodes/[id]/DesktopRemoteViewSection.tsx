"use client";

import { RemoteViewerShell, type RemoteViewerShellProps } from "../../../../components/RemoteViewerShell";
import { DesktopConnectBar, type DesktopConnectBarProps } from "./DesktopConnectBar";

export interface DesktopRemoteViewSectionProps {
  connectBarProps: DesktopConnectBarProps;
  protocolSwitchNotice?: string | null;
  remoteViewerShellProps: RemoteViewerShellProps;
}

/** A device's Desktop tab: its remote screen, with the connect options in the row above it until it connects. */
export function DesktopRemoteViewSection({
  connectBarProps,
  protocolSwitchNotice,
  remoteViewerShellProps,
}: DesktopRemoteViewSectionProps) {
  return (
    <RemoteViewerShell
      {...remoteViewerShellProps}
      offlineTools={<DesktopConnectBar {...connectBarProps} />}
      notice={protocolSwitchNotice || undefined}
      startLabel="connect"
    />
  );
}
