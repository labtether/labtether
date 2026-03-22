"use client";

import { useCallback, useEffect, useState } from "react";

export function useFullscreen(elementRef: React.RefObject<HTMLElement | null>) {
  const [isFullscreen, setIsFullscreen] = useState(false);

  useEffect(() => {
    const handleChange = () => {
      setIsFullscreen(document.fullscreenElement === elementRef.current);
    };
    document.addEventListener("fullscreenchange", handleChange);
    return () => document.removeEventListener("fullscreenchange", handleChange);
  }, [elementRef]);

  const toggle = useCallback(() => {
    if (!elementRef.current) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      elementRef.current.requestFullscreen().catch(() => {});
    }
  }, [elementRef]);

  return { isFullscreen, toggleFullscreen: toggle };
}
