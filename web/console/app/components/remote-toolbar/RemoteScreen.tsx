"use client";

import dynamic from "next/dynamic";
import type { RefObject } from "react";
import type { GuacamoleViewerHandle } from "../GuacamoleViewer";
import type { SPICEViewerHandle } from "../SPICEViewer";
import type { VNCCredentialRequest, VNCViewerHandle } from "../VNCViewer";
import type { WebRTCConnectionStats, WebRTCDisplayLayout, WebRTCViewerHandle } from "../WebRTCViewer";
import type { DesktopProtocol, SpiceTicket } from "../SessionPanel";
import type { ScalingMode } from "./types";

// While a viewer's code loads the screen stays black; the state over it (connecting, signing in) says what is going on.
const blank = () => <div className="vncContainer" />;
const VNCViewer = dynamic(() => import("../VNCViewer"), { ssr: false, loading: blank });
const GuacamoleViewer = dynamic(() => import("../GuacamoleViewer"), { ssr: false, loading: blank });
const SPICEViewer = dynamic(() => import("../SPICEViewer"), { ssr: false, loading: blank });
const WebRTCViewer = dynamic(() => import("../WebRTCViewer"), { ssr: false, loading: blank });

export interface RemoteScreenProps {
  protocol: DesktopProtocol;
  wsUrl: string | null;
  spiceTicket: SpiceTicket | null;
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
  quality: string;
  scalingMode: ScalingMode;
  viewOnly: boolean;
  audioEnabled: boolean;
  volume: number;
  onWebRTCStats?: (stats: WebRTCConnectionStats) => void;
  onWebRTCStream?: (stream: MediaStream | null) => void;
  webrtcDisplayLayout?: WebRTCDisplayLayout[];
  onDesktopConnect: () => void;
  onDesktopDisconnect: (detail: { clean: boolean; reason?: string }) => void;
  onDesktopError: (message: string) => void;
  onCredentialsRequired: (request: VNCCredentialRequest) => void;
}

/** The remote screen itself: the viewer for the session's protocol, filling the stage. */
export function RemoteScreen(props: RemoteScreenProps) {
  const { protocol, wsUrl, spiceTicket } = props;
  if (protocol === "rdp") {
    return (
      <GuacamoleViewer
        ref={props.guacRef}
        wsUrl={wsUrl}
        onConnect={props.onDesktopConnect}
        onDisconnect={props.onDesktopDisconnect}
        onError={props.onDesktopError}
      />
    );
  }
  if (protocol === "spice") {
    return spiceTicket ? (
      <SPICEViewer
        ref={props.spiceRef}
        wsUrl={spiceTicket.wsUrl}
        password={spiceTicket.password}
        onConnect={props.onDesktopConnect}
        onDisconnect={props.onDesktopDisconnect}
      />
    ) : (
      blank()
    );
  }
  if (protocol === "webrtc") {
    return (
      <WebRTCViewer
        key={wsUrl ?? "webrtc-idle"}
        ref={props.webrtcRef}
        wsUrl={wsUrl}
        scalingMode={props.scalingMode}
        audioEnabled={props.audioEnabled}
        volume={props.volume}
        onStats={props.onWebRTCStats}
        onStream={props.onWebRTCStream}
        displayLayout={props.webrtcDisplayLayout}
        onConnect={props.onDesktopConnect}
        onDisconnect={props.onDesktopDisconnect}
      />
    );
  }
  return (
    <VNCViewer
      ref={props.vncRef}
      wsUrl={wsUrl}
      onConnect={props.onDesktopConnect}
      onDisconnect={props.onDesktopDisconnect}
      onError={props.onDesktopError}
      onCredentialsRequired={props.onCredentialsRequired}
      quality={props.quality}
      scalingMode={props.scalingMode}
      viewOnly={props.viewOnly}
    />
  );
}
