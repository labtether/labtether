"use client";

import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Seg } from "../../../../components/containers/ContainersToolbar";
import { nodeHref, parseDockerStatus, stateTone } from "../../../../components/containers/containerModel";
import { KV, ListEmpty, SectionHead } from "../../../../components/containers/PanelParts";
import { useStateWord } from "../../../../components/containers/useContainerWords";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useAuth } from "../../../../contexts/AuthContext";
import { canAccessHostData } from "../../../../lib/roles";
import type { Asset } from "../../../../console/models";
import { usePortainerCapabilities } from "./useDeviceCapabilities";
import { PortainerOverviewTab } from "./portainer/PortainerOverviewTab";
import { PortainerContainersTab } from "./portainer/PortainerContainersTab";
import { PortainerStacksTab } from "./portainer/PortainerStacksTab";
import { PortainerImagesTab } from "./portainer/PortainerImagesTab";
import { PortainerVolumesTab } from "./portainer/PortainerVolumesTab";
import { PortainerNetworksTab } from "./portainer/PortainerNetworksTab";

type PortainerPanelProps = {
  asset: Asset;
};

const HOST_VIEWS = ["overview", "containers", "stacks", "images", "volumes", "networks"] as const;
type HostView = (typeof HOST_VIEWS)[number];

function parseLabels(raw?: string): Array<[string, string]> {
  try {
    const parsed = JSON.parse(raw?.trim() || "{}") as Record<string, unknown>;
    return Object.entries(parsed)
      .filter((e): e is [string, string] => typeof e[1] === "string")
      .sort(([a], [b]) => a.localeCompare(b));
  } catch {
    return [];
  }
}

/** A Portainer endpoint: Overview · Containers · Stacks · Images · Volumes · Networks, as far as the endpoint allows. */
function PortainerHostPanel({ asset }: { asset: Asset }) {
  const t = useTranslations("devices-docker.views");
  const [view, setView] = useState<HostView>("overview");
  const { data: caps } = usePortainerCapabilities(asset.id);
  const { user } = useAuth();
  // Container shells are interactive sessions, which the Hub keeps for operators.
  const canExec = caps?.can_exec === true && canAccessHostData(user?.role);
  // All views while the endpoint's capabilities load, so the switch does not jump.
  const views = useMemo(() => (caps ? HOST_VIEWS.filter((v) => caps.tabs.includes(v)) : [...HOST_VIEWS]), [caps]);
  const current: HostView = views.includes(view) ? view : views[0] ?? "overview";
  const bar = views.length > 1 ? <Seg<HostView> value={current} items={views.map((v) => [v, t(v)])} onChange={setView} label={t("portainer")} /> : null;

  return (
    <div className="ct-tab">
      {current === "overview" && <PortainerOverviewTab assetId={asset.id} bar={bar} />}
      {current === "containers" && <PortainerContainersTab assetId={asset.id} hostName={asset.name} canExec={canExec} bar={bar} />}
      {current === "stacks" && <PortainerStacksTab assetId={asset.id} bar={bar} />}
      {current === "images" && <PortainerImagesTab assetId={asset.id} bar={bar} />}
      {current === "volumes" && <PortainerVolumesTab assetId={asset.id} bar={bar} />}
      {current === "networks" && <PortainerNetworksTab assetId={asset.id} bar={bar} />}
    </div>
  );
}

/** The Portainer endpoint and stack an asset belongs to (by endpoint id and stack name). */
function usePortainerRelations(endpointID: string, stackName: string) {
  const assets = useFastStatus()?.assets;
  return useMemo(() => {
    const list = (assets ?? []).filter((a) => a.source === "portainer" && a.metadata?.endpoint_id === endpointID);
    const host = list.find((a) => a.type === "container-host") ?? null;
    const name = stackName.trim().toLowerCase();
    const stack = name ? list.find((a) => (a.type === "stack" || a.type === "compose-stack") && a.name.toLowerCase() === name) ?? null : null;
    return { list, host, stack };
  }, [assets, endpointID, stackName]);
}

function PortainerContainerPanel({ asset }: { asset: Asset }) {
  const t = useTranslations("devices-docker.portainer");
  const word = useStateWord();
  const meta = asset.metadata ?? {};
  const { host, stack } = usePortainerRelations(meta.endpoint_id ?? "", meta.stack ?? "");
  const labels = useMemo(() => parseLabels(meta.labels_json), [meta.labels_json]);
  const st = parseDockerStatus(meta.state || "", meta.status || "");
  const tone = stateTone(st);
  const created = meta.created_at ? new Date(meta.created_at) : null;

  return (
    <div className="ct-tab">
      <section aria-labelledby="pt-det">
        <SectionHead id="pt-det" title={t("details")} />
        <dl className="ct-sheet ct-kvs">
          <KV k={t("image")} mono><span className="ct-wrap">{meta.image || "—"}</span></KV>
          <KV k={t("state")}>
            {tone ? <i className={`ct-dot ${tone}`} aria-hidden="true" /> : null}
            <span>{word(st, true)}</span>
            {meta.status ? <span className="ct-quiet">{meta.status}</span> : null}
          </KV>
          <KV k={t("ports")} mono>{meta.ports || <span className="ct-quiet">{t("noPorts")}</span>}</KV>
          <KV k={t("endpoint")}>
            {host ? <Link href={nodeHref(host.id)} className="ct-lnk">{host.name}</Link> : <span className="ct-quiet">{meta.endpoint_id || "—"}</span>}
          </KV>
          <KV k={t("stack")}>
            {stack ? <Link href={nodeHref(stack.id)} className="ct-lnk">{stack.name}</Link> : <span className="ct-quiet">{meta.stack || t("standalone")}</span>}
          </KV>
          {created && Number.isFinite(created.getTime()) ? <KV k={t("created")}>{created.toLocaleString()}</KV> : null}
          {meta.container_id ? <KV k={t("containerId")} mono><span className="ct-wrap" title={meta.container_id}>{meta.container_id.slice(0, 12)}</span></KV> : null}
        </dl>
      </section>
      {labels.length ? (
        <section className="ct-sec" aria-labelledby="pt-lbl">
          <SectionHead id="pt-lbl" title={t("labels")} />
          <dl className="ct-sheet ct-pairs">
            {labels.map(([k, v]) => <div key={k}><dt className="mono">{k}</dt><dd className="mono">{v}</dd></div>)}
          </dl>
        </section>
      ) : null}
    </div>
  );
}

function PortainerStackPanel({ asset }: { asset: Asset }) {
  const t = useTranslations("devices-docker.portainer");
  const word = useStateWord();
  const meta = asset.metadata ?? {};
  const { list, host } = usePortainerRelations(meta.endpoint_id ?? "", "");
  const members = useMemo(() => list
    .filter((a) => a.type === "container" && (a.metadata?.stack ?? "").trim().toLowerCase() === asset.name.trim().toLowerCase())
    .sort((a, b) => a.name.localeCompare(b.name)), [list, asset.name]);
  const active = (meta.status || asset.status || "").toLowerCase();

  return (
    <div className="ct-tab">
      <section aria-labelledby="pt-sd">
        <SectionHead id="pt-sd" title={t("details")} />
        <dl className="ct-sheet ct-kvs">
          <KV k={t("state")}>{active === "active" || active === "online" ? t("active") : active === "inactive" ? t("inactive") : (meta.status || asset.status || "—")}</KV>
          {meta.type ? <KV k={t("type")}>{meta.type}</KV> : null}
          <KV k={t("endpoint")}>{host ? <Link href={nodeHref(host.id)} className="ct-lnk">{host.name}</Link> : <span className="ct-quiet">{meta.endpoint_id || "—"}</span>}</KV>
          {meta.git_url ? <KV k={t("git")} mono><span className="ct-wrap">{meta.git_url}</span></KV> : null}
          {meta.entry_point ? <KV k={t("entryPoint")} mono>{meta.entry_point}</KV> : null}
          {meta.created_by ? <KV k={t("createdBy")}>{meta.created_by}</KV> : null}
        </dl>
      </section>
      <section className="ct-sec" aria-labelledby="pt-sm">
        <SectionHead id="pt-sm" title={t("members")} />
        {members.length ? (
          <div className="ct-list">
            {members.map((c) => {
              const st = parseDockerStatus(c.metadata?.state || "", c.metadata?.status || "");
              const tone = stateTone(st);
              return (
                <div key={c.id} className="ct-row sm">
                  <span className="ct-lead" aria-hidden="true">{tone ? <i className={`ct-dot ${tone}`} /> : null}</span>
                  <span className="ct-main">
                    <Link href={nodeHref(c.id)} className="ct-nm">{c.name}</Link>
                    <span className="ct-grey">{word(st) || c.metadata?.image || ""}</span>
                  </span>
                </div>
              );
            })}
          </div>
        ) : <ListEmpty text={t("noMembers")} />}
      </section>
    </div>
  );
}

export function PortainerPanel({ asset }: PortainerPanelProps) {
  const t = useTranslations("devices-docker.portainer");
  if (asset.type === "container-host") return <PortainerHostPanel asset={asset} />;
  if (asset.type === "container") return <PortainerContainerPanel asset={asset} />;
  if (asset.type === "stack" || asset.type === "compose-stack") return <PortainerStackPanel asset={asset} />;
  return <ListEmpty text={t("noDetails")} />;
}
