"use client";

import { useTranslations } from "next-intl";
import { useEffect, useState, type ReactNode } from "react";
import { Link } from "../../../i18n/navigation";
import { fetchDockerHosts } from "../../../lib/docker";
import { dockerHostAssetId, hostKey, nodeHref } from "./containerModel";

export type HostOption = { key: string; label: string };

/** Docker hosts (agents) that take new containers and stacks. The route's host stays listed while the list loads. */
export function useDeployHosts(routeKey: string): HostOption[] {
  const [hosts, setHosts] = useState<HostOption[]>([]);
  useEffect(() => {
    const controller = new AbortController();
    fetchDockerHosts(controller.signal)
      .then((list) => setHosts(list.map((h) => ({ key: hostKey(h.normalized_id || h.agent_id), label: h.agent_id || h.normalized_id }))))
      .catch(() => { /* the route's host stays the only choice */ });
    return () => controller.abort();
  }, []);
  if (routeKey && !hosts.some((h) => h.key === routeKey)) return [{ key: routeKey, label: routeKey }, ...hosts];
  return hosts;
}

/** The Docker tab of a host's page, at one of its views. */
export const dockerTabHref = (key: string, sub: "containers" | "stacks") => `${nodeHref(dockerHostAssetId(key))}?panel=docker&sub=${sub}`;

/** Containers / host, above a deploy page's title. */
export function DeployCrumbs({ host }: { host: HostOption | undefined }) {
  const t = useTranslations("containers");
  return (
    <nav className="ct-crumb" aria-label={t("deploy.crumbs")}>
      <Link href="/containers">{t("title")}</Link>
      {host ? (
        <>
          <span aria-hidden="true">/</span>
          <Link href={nodeHref(dockerHostAssetId(host.key))}>{host.label}</Link>
        </>
      ) : null}
    </nav>
  );
}

/** A labelled field with its error under it. */
export function Field({ id, label, error, hint, wide = false, children }: {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={`ct-field${wide ? " wide" : ""}`}>
      <label htmlFor={id}>{label}</label>
      {children}
      {error ? <p className="ct-err" id={`${id}-err`}>{error}</p> : hint ? <p className="ct-hint">{hint}</p> : null}
    </div>
  );
}

/** Props that tie an input to its Field's error. */
export const invalid = (id: string, error?: string) => (error ? { "aria-invalid": true as const, "aria-describedby": `${id}-err` } : {});

export function HostSelect({ id, hosts, value, onChange }: { id: string; hosts: HostOption[]; value: string; onChange: (key: string) => void }) {
  return (
    <select className="ct-input" id={id} value={value} onChange={(e) => onChange(e.target.value)}>
      {hosts.map((h) => <option key={h.key} value={h.key}>{h.label}</option>)}
    </select>
  );
}
