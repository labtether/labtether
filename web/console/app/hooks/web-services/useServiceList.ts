"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { reportRoutePerfMetric } from "../useRoutePerfTelemetry";
import type { URLGroupingSuggestion, UseWebServicesOptions, WebService, WebServiceDiscoveryHostStat } from "./types";
import { reconcileWebServiceList } from "./serviceModel";
import { areDiscoveryStatsEqual, areSuggestionListsEqual, normalizeDiscoveryHostStatList, normalizeSuggestionList } from "./discoveryModel";
import { fetchServicePayload } from "./serviceRequest";

export function useServiceList(options: UseWebServicesOptions) {
  const { host, includeHidden = false, pollInterval = 30000, detailLevel = "full" } = options;
  const [services, setServices] = useState<WebService[]>([]);
  const [discoveryStats, setDiscoveryStats] = useState<WebServiceDiscoveryHostStat[]>([]);
  const [suggestions, setSuggestions] = useState<URLGroupingSuggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const servicesRef = useRef<WebService[]>([]);
  const discoveryStatsRef = useRef<WebServiceDiscoveryHostStat[]>([]);
  const suggestionsRef = useRef<URLGroupingSuggestion[]>([]);

  const fetchServices = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();

    try {
      const { response: res, payload } = await fetchServicePayload({
        detail: detailLevel,
        hostAssetID: host,
        includeHiddenServices: includeHidden,
        signal: controller.signal,
      });
      const { next: nextServices, reused: reusedServices, changed: changedServices } = reconcileWebServiceList(
        payload?.services, servicesRef.current, includeHidden
      );
      const normalizedDiscoveryStats = normalizeDiscoveryHostStatList(payload?.discovery_stats);
      const nextDiscoveryStats = areDiscoveryStatsEqual(discoveryStatsRef.current, normalizedDiscoveryStats)
        ? discoveryStatsRef.current
        : normalizedDiscoveryStats;
      const normalizedSuggestions = normalizeSuggestionList(payload?.suggestions);
      const nextSuggestions = areSuggestionListsEqual(suggestionsRef.current, normalizedSuggestions)
        ? suggestionsRef.current
        : normalizedSuggestions;

      if (res.ok) {
        if (servicesRef.current !== nextServices) {
          servicesRef.current = nextServices;
          setServices(nextServices);
        }
        if (discoveryStatsRef.current !== nextDiscoveryStats) {
          discoveryStatsRef.current = nextDiscoveryStats;
          setDiscoveryStats(nextDiscoveryStats);
        }
        if (suggestionsRef.current !== nextSuggestions) {
          suggestionsRef.current = nextSuggestions;
          setSuggestions(nextSuggestions);
        }
        reportRoutePerfMetric({
          route: "services",
          metric: "request.services_fetch",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          sampleSize: nextServices.length,
          metadata: {
            host_filtered: Boolean(host),
            include_hidden: includeHidden,
            detail_level: detailLevel,
            discovery_hosts: nextDiscoveryStats.length,
            suggestions: nextSuggestions.length,
            reused_services: reusedServices,
            changed_services: changedServices,
          },
        });
        const scheduleAfterPaint = typeof window.requestAnimationFrame === "function"
          ? window.requestAnimationFrame.bind(window)
          : (callback: FrameRequestCallback) => window.setTimeout(() => callback(performance.now()), 0);
        scheduleAfterPaint(() => {
          reportRoutePerfMetric({
            route: "services",
            metric: "render.services_results",
            durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
            sampleSize: nextServices.length,
            metadata: {
              host_filtered: Boolean(host),
              include_hidden: includeHidden,
              detail_level: detailLevel,
              discovery_hosts: nextDiscoveryStats.length,
              suggestions: nextSuggestions.length,
              reused_services: reusedServices,
              changed_services: changedServices,
            },
          });
        });
        setError(null);
      } else {
        reportRoutePerfMetric({
          route: "services",
          metric: "request.services_fetch",
          durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
          status: "error",
          metadata: {
            http_status: res.status,
            host_filtered: Boolean(host),
            include_hidden: includeHidden,
            detail_level: detailLevel,
          },
        });
        setError(`HTTP ${res.status}`);
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      reportRoutePerfMetric({
        route: "services",
        metric: "request.services_fetch",
        durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
        status: "error",
        metadata: {
          host_filtered: Boolean(host),
          include_hidden: includeHidden,
          detail_level: detailLevel,
        },
      });
      setError(err instanceof Error ? err.message : "Failed to fetch services");
    } finally {
      setLoading(false);
    }
  }, [detailLevel, host, includeHidden]);

  useEffect(() => {
    let interval: ReturnType<typeof setInterval> | null = null;

    const start = () => {
      void fetchServices();
      if (interval === null) {
        interval = setInterval(fetchServices, pollInterval);
      }
    };

    const stop = () => {
      if (interval !== null) {
        clearInterval(interval);
        interval = null;
      }
      abortRef.current?.abort();
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        start();
        return;
      }
      stop();
    };

    if (document.visibilityState === "visible") {
      start();
    }

    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stop();
    };
  }, [fetchServices, pollInterval]);

  return { services, discoveryStats, suggestions, loading, error, refresh: fetchServices, servicesRef, setError };
}
