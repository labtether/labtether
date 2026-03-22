"use client";

import { useCallback, useEffect, useState, type RefObject } from "react";

import type { GuacamoleViewerHandle } from "../../../../components/GuacamoleViewer";
import type { SPICEViewerHandle } from "../../../../components/SPICEViewer";
import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type { WebRTCViewerHandle } from "../../../../components/WebRTCViewer";

import type { DesktopProtocol } from "./desktopTabPreferences";

type DesktopViewerFocusOptions = {
  protocol: DesktopProtocol;
  viewerWrapperRef: RefObject<HTMLDivElement | null>;
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
};

export function useDesktopViewerFocus({
  protocol,
  viewerWrapperRef,
  vncRef,
  guacRef,
  spiceRef,
  webrtcRef,
}: DesktopViewerFocusOptions) {
  const [viewerFocused, setViewerFocused] = useState(false);

  useEffect(() => {
    const updateViewerFocus = () => {
      const wrapper = viewerWrapperRef.current;
      const active = document.activeElement;
      setViewerFocused(
        Boolean(wrapper && active instanceof Node && wrapper.contains(active)),
      );
    };

    const scheduleUpdateViewerFocus = () => {
      window.setTimeout(updateViewerFocus, 0);
    };

    updateViewerFocus();
    document.addEventListener("focusin", updateViewerFocus);
    document.addEventListener("focusout", scheduleUpdateViewerFocus);
    return () => {
      document.removeEventListener("focusin", updateViewerFocus);
      document.removeEventListener("focusout", scheduleUpdateViewerFocus);
    };
  }, [viewerWrapperRef]);

  const focusActiveViewer = useCallback(() => {
    if (protocol === "vnc") {
      vncRef.current?.focus?.();
      return;
    }
    if (protocol === "rdp") {
      guacRef.current?.focus?.();
      return;
    }
    if (protocol === "spice") {
      spiceRef.current?.focus?.();
      return;
    }
    if (protocol === "webrtc") {
      webrtcRef.current?.focus?.();
    }
  }, [guacRef, protocol, spiceRef, vncRef, webrtcRef]);

  const restoreViewerFocus = useCallback(() => {
    window.requestAnimationFrame(() => {
      focusActiveViewer();
    });
  }, [focusActiveViewer]);

  return {
    viewerFocused,
    focusActiveViewer,
    restoreViewerFocus,
  };
}
