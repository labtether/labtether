"use client";

import { useEffect, useState } from "react";

import { plainFingerprint } from "../lib/fingerprint";

type TLSInfo = { ca_available?: boolean; ca_fingerprint_sha256?: string };

/**
 * The fingerprint of the Hub's own CA certificate as plain hex, from the Hub's public TLS info. "" while loading,
 * when the Hub has no CA of its own (a publicly trusted or uploaded certificate, or plain HTTP), or when the Hub
 * cannot be asked: a fingerprint is only worth showing when it is the Hub's answer.
 */
export function useHubCAFingerprint(): string {
  const [fingerprint, setFingerprint] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      try {
        const response = await fetch("/api/v1/tls/info", { cache: "no-store", signal: controller.signal });
        if (!response.ok) return;
        const info = (await response.json()) as TLSInfo;
        setFingerprint(info.ca_available ? plainFingerprint(info.ca_fingerprint_sha256) : "");
      } catch {
        // No answer: show nothing.
      }
    })();
    return () => controller.abort();
  }, []);

  return fingerprint;
}
