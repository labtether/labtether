"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Bar, KV, ListEmpty, ListError, SectionHead } from "../../../../../components/containers/PanelParts";
import { usePortainerOverview } from "./usePortainerData";

type Props = {
  assetId: string;
  bar?: ReactNode;
};

/** The endpoint's Portainer version and address, and how many containers, stacks, images, volumes and networks it has. */
export function PortainerOverviewTab({ assetId, bar }: Props) {
  const t = useTranslations("devices-docker.portainer");
  const { data, loading, error, refresh } = usePortainerOverview(assetId);

  let body: ReactNode;
  if (loading && !data) body = <ListEmpty text={t("loading")} />;
  else if (error && !data) body = <ListError text={t("loadFailed")} onRetry={refresh} />;
  else if (!data) body = <ListEmpty text={t("noOverview")} />;
  else body = (
    <div className="ct-two">
      <section aria-labelledby="pt-srv">
        <SectionHead id="pt-srv" title={t("server")} />
        <dl className="ct-sheet ct-kvs">
          <KV k={t("version")}>{data.server_version || "—"}</KV>
          <KV k={t("endpoint")}>{data.endpoint_name || "—"}</KV>
          <KV k={t("url")} mono><span className="ct-wrap">{data.endpoint_url || "—"}</span></KV>
        </dl>
      </section>
      <section aria-labelledby="pt-res">
        <SectionHead id="pt-res" title={t("resources")} />
        <dl className="ct-sheet ct-kvs">
          <KV k={t("containers")}>{t("runStop", { running: data.containers.running, stopped: data.containers.stopped })}</KV>
          <KV k={t("stacks")}>{t("runStop", { running: data.stacks.running, stopped: data.stacks.stopped })}</KV>
          <KV k={t("images")}>{data.images.count}</KV>
          <KV k={t("volumes")}>{data.volumes.count}</KV>
          <KV k={t("networks")}>{data.networks.count}</KV>
        </dl>
      </section>
    </div>
  );

  return (
    <>
      <Bar left={bar} />
      {body}
    </>
  );
}
