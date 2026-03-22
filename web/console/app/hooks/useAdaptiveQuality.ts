"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ViewerMetrics } from "../types/viewer";

type QualityLevel = "low" | "medium" | "high";

interface AdaptiveQualityState {
  suggestion: "upgrade" | "downgrade" | null;
  autoEnabled: boolean;
  currentQuality: QualityLevel;
}

const DOWNGRADE_FPS_THRESHOLD = 10;
const UPGRADE_FPS_THRESHOLD = 25;
const EVALUATION_WINDOW_MS = 5000;
const SAMPLE_INTERVAL_MS = 1000;

export function useAdaptiveQuality(
  metrics: ViewerMetrics | null,
  initialQuality: QualityLevel = "medium",
) {
  const [state, setState] = useState<AdaptiveQualityState>({
    suggestion: null,
    autoEnabled: false,
    currentQuality: initialQuality,
  });

  const samplesRef = useRef<number[]>([]);
  const lastChangeRef = useRef(Date.now());

  // Collect FPS samples
  useEffect(() => {
    if (!metrics?.fps) return;
    samplesRef.current.push(metrics.fps);

    // Keep only samples within evaluation window
    const maxSamples = Math.ceil(EVALUATION_WINDOW_MS / SAMPLE_INTERVAL_MS);
    if (samplesRef.current.length > maxSamples) {
      samplesRef.current = samplesRef.current.slice(-maxSamples);
    }
  }, [metrics?.fps]);

  // Evaluate quality periodically
  useEffect(() => {
    const interval = setInterval(() => {
      const samples = samplesRef.current;
      if (samples.length < 3) return;

      // Don't suggest changes within 10s of last change
      if (Date.now() - lastChangeRef.current < 10000) return;

      const avgFps = samples.reduce((a, b) => a + b, 0) / samples.length;

      setState((prev) => {
        let suggestion: "upgrade" | "downgrade" | null = null;

        if (avgFps < DOWNGRADE_FPS_THRESHOLD && prev.currentQuality !== "low") {
          suggestion = "downgrade";
        } else if (avgFps > UPGRADE_FPS_THRESHOLD && prev.currentQuality !== "high") {
          suggestion = "upgrade";
        }

        if (suggestion !== prev.suggestion) {
          return { ...prev, suggestion };
        }
        return prev;
      });
    }, EVALUATION_WINDOW_MS);

    return () => clearInterval(interval);
  }, []);

  const setAutoEnabled = useCallback((enabled: boolean) => {
    setState((prev) => ({ ...prev, autoEnabled: enabled }));
  }, []);

  const applyQuality = useCallback((quality: QualityLevel) => {
    lastChangeRef.current = Date.now();
    samplesRef.current = [];
    setState((prev) => ({ ...prev, currentQuality: quality, suggestion: null }));
  }, []);

  const dismissSuggestion = useCallback(() => {
    setState((prev) => ({ ...prev, suggestion: null }));
  }, []);

  // Get next quality level for a direction
  const getNextQuality = useCallback(
    (direction: "upgrade" | "downgrade"): QualityLevel | null => {
      const levels: QualityLevel[] = ["low", "medium", "high"];
      const idx = levels.indexOf(state.currentQuality);
      if (direction === "upgrade" && idx < levels.length - 1) return levels[idx + 1];
      if (direction === "downgrade" && idx > 0) return levels[idx - 1];
      return null;
    },
    [state.currentQuality],
  );

  return {
    ...state,
    setAutoEnabled,
    applyQuality,
    dismissSuggestion,
    getNextQuality,
  };
}
