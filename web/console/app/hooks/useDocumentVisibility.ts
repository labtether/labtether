"use client";

import { useEffect, useState } from "react";

function currentVisibility(): boolean {
  if (typeof document === "undefined") {
    return true;
  }
  return document.visibilityState !== "hidden";
}

export function useDocumentVisibility(): boolean {
  const [isVisible, setIsVisible] = useState<boolean>(() => currentVisibility());

  useEffect(() => {
    const updateVisibility = () => {
      setIsVisible(currentVisibility());
    };

    updateVisibility();
    document.addEventListener("visibilitychange", updateVisibility);
    return () => {
      document.removeEventListener("visibilitychange", updateVisibility);
    };
  }, []);

  return isVisible;
}
