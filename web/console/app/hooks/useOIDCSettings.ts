"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

type OIDCSettingsPayload = {
  enabled?: boolean;
  issuer_url?: string;
  client_id?: string;
  client_secret?: string;
  scopes?: string;
  role_claim?: string;
  default_role?: string;
  display_name?: string;
  admin_role_values?: string;
  operator_role_values?: string;
  auto_provision?: boolean;
  sources?: Record<string, "env" | "db" | "default">;
  active?: boolean;
  active_issuer?: string;
  error?: string;
  message?: string;
};

export function useOIDCSettings() {
  const [enabled, setEnabled] = useState(false);
  const [issuerURL, setIssuerURL] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [scopes, setScopes] = useState("");
  const [roleClaim, setRoleClaim] = useState("");
  const [defaultRole, setDefaultRole] = useState("viewer");
  const [displayName, setDisplayName] = useState("");
  const [adminRoleValues, setAdminRoleValues] = useState("");
  const [operatorRoleValues, setOperatorRoleValues] = useState("");
  const [autoProvision, setAutoProvision] = useState(false);
  const [sources, setSources] = useState<Record<string, "env" | "db" | "default">>({});
  const [active, setActive] = useState(false);
  const [activeIssuer, setActiveIssuer] = useState("");

  const savingRef = useRef(false);
  const applyingRef = useRef(false);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/oidc", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = (await response.json()) as OIDCSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load oidc settings (${response.status})`);
      }

      setEnabled(payload.enabled ?? false);
      setIssuerURL(payload.issuer_url ?? "");
      setClientID(payload.client_id ?? "");
      setClientSecret(payload.client_secret ?? "");
      setScopes(payload.scopes ?? "");
      setRoleClaim(payload.role_claim ?? "");
      setDefaultRole(payload.default_role ?? "viewer");
      setDisplayName(payload.display_name ?? "");
      setAdminRoleValues(payload.admin_role_values ?? "");
      setOperatorRoleValues(payload.operator_role_values ?? "");
      setAutoProvision(payload.auto_provision ?? false);
      setSources(payload.sources ?? {});
      setActive(payload.active ?? false);
      setActiveIssuer(payload.active_issuer ?? "");
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load oidc settings"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = useCallback(async () => {
    if (savingRef.current) {
      return;
    }
    savingRef.current = true;
    setSaving(true);
    setError("");
    setMessage("");
    try {
      const body: Record<string, unknown> = {
        enabled,
        issuer_url: issuerURL,
        client_id: clientID,
        client_secret: clientSecret,
        scopes,
        role_claim: roleClaim,
        default_role: defaultRole,
        display_name: displayName,
        admin_role_values: adminRoleValues,
        operator_role_values: operatorRoleValues,
        auto_provision: autoProvision,
      };

      const response = await fetch("/api/settings/oidc", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify(body),
      });
      const payload = (await response.json()) as OIDCSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to save oidc settings (${response.status})`);
      }

      await load();
      setMessage("OIDC settings saved.");
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to save oidc settings"));
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  }, [enabled, issuerURL, clientID, clientSecret, scopes, roleClaim, defaultRole, displayName, adminRoleValues, operatorRoleValues, autoProvision, load]);

  const apply = useCallback(async () => {
    if (applyingRef.current) {
      return;
    }
    applyingRef.current = true;
    setApplying(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch("/api/settings/oidc/apply", {
        method: "POST",
        signal: AbortSignal.timeout(25_000),
      });
      const payload = (await response.json()) as OIDCSettingsPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to apply oidc settings (${response.status})`);
      }

      await load();
      setMessage(payload.message || "OIDC settings applied.");
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to apply oidc settings"));
    } finally {
      applyingRef.current = false;
      setApplying(false);
    }
  }, [load]);

  return {
    enabled,
    setEnabled,
    issuerURL,
    setIssuerURL,
    clientID,
    setClientID,
    clientSecret,
    setClientSecret,
    scopes,
    setScopes,
    roleClaim,
    setRoleClaim,
    defaultRole,
    setDefaultRole,
    displayName,
    setDisplayName,
    adminRoleValues,
    setAdminRoleValues,
    operatorRoleValues,
    setOperatorRoleValues,
    autoProvision,
    setAutoProvision,
    sources,
    active,
    activeIssuer,
    loading,
    saving,
    applying,
    error,
    message,
    load,
    save,
    apply,
  };
}
