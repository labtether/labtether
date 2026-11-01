"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { withCollector } from "../proxmoxFormatters";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection } from "./DiList";
import { useProxmoxList } from "./useProxmoxData";

type UpdatePackage = {
  Package?: string;
  Title?: string;
  OldVersion?: string;
  Version?: string;
  Origin?: string;
  Priority?: string;
  Section?: string;
  Description?: string;
};

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

/** Package updates Proxmox reports for the node: package, current and new version. */
export function ProxmoxUpdatesTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const t = useTranslations("devices-proxmox.pve.updates");
  const kit = useTranslations("devices-proxmox.kit");
  const path = proxmoxNode
    ? withCollector(`/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/updates`, proxmoxCollectorID)
    : null;
  const { data: packages, loading, error, refresh } = useProxmoxList<UpdatePackage>(path);

  return (
    <DiSection
      title={t("title")}
      right={<Button size="sm" onClick={refresh} disabled={loading}>{loading ? kit("loading") : kit("refresh")}</Button>}
    >
      {error ? <DiNote tone="bad">{t("loadFailed", { error })}</DiNote> : null}
      <DiList cols="minmax(0, 1fr) minmax(0, 260px)" colsS="minmax(0, 1fr) minmax(0, 45%)" label={t("title")}>
        <DiCols>
          <span>{t("package")}</span>
          <span className="r">{t("version")}</span>
        </DiCols>
        {packages.length === 0 ? <DiEmpty title={loading ? t("checking") : error ? t("unknown") : t("upToDate")} /> : null}
        {packages.map((pkg, index) => (
          <DiRow key={pkg.Package ?? index}>
            <DiMain name={pkg.Package || "—"} grey={<DiDots parts={[pkg.Title || null, pkg.Section || null, pkg.Origin || null]} />} />
            <span className="di-num di-mono">
              {pkg.OldVersion ? `${pkg.OldVersion} → ` : ""}{pkg.Version || "—"}
            </span>
          </DiRow>
        ))}
      </DiList>
    </DiSection>
  );
}
