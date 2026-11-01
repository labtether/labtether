"use client";

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { formatProxmoxValue } from "../proxmoxFormatters";
import { DiFoot, DiKV, DiNote, DiSection, DiSheet } from "./DiList";
import { whenText } from "./diFormat";

type Props = {
  proxmoxDetails: ProxmoxDetails;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
};

const CONFIG_PREVIEW = 36;
const KINDS = new Set(["node", "qemu", "lxc", "storage"]);

/** The resource's details, any partial-data warnings, and its Proxmox configuration as key-value lines. */
export function ProxmoxOverviewTab({ proxmoxDetails, proxmoxNode, proxmoxVMID, effectiveKind }: Props) {
  const t = useTranslations("devices-proxmox.pve");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [showAll, setShowAll] = useState(false);

  const config = useMemo(() => {
    return Object.entries(proxmoxDetails.config ?? {})
      .map(([key, value]) => [key, formatProxmoxValue(value)] as const)
      .filter(([, value]) => value.trim() !== "")
      .sort(([left], [right]) => left.localeCompare(right));
  }, [proxmoxDetails.config]);

  const kind = proxmoxDetails.kind || effectiveKind;
  const kindWord = KINDS.has(kind) ? t(`kinds.${kind}`) : kind;
  const shown = showAll ? config : config.slice(0, CONFIG_PREVIEW);

  return (
    <>
      {proxmoxDetails.warnings?.length ? (
        <div>
          {proxmoxDetails.warnings.map((warning, index) => (
            <DiNote key={`${warning}-${index}`} tone="warn">{t("partial", { warning })}</DiNote>
          ))}
        </div>
      ) : null}

      <DiSection title={t("details")}>
        <DiSheet kind="tight">
          <DiKV
            columns={2}
            items={[
              [t("type"), kindWord || null],
              [t("node"), proxmoxDetails.node || proxmoxNode || null],
              [t("vmid"), proxmoxDetails.vmid || proxmoxVMID || null],
              [t("version"), proxmoxDetails.version || null],
              [t("updated"), whenText(proxmoxDetails.fetched_at, locale) || null],
            ]}
          />
        </DiSheet>
      </DiSection>

      <DiSection title={t("configuration")}>
        <DiSheet kind="tight">
          {config.length > 0 ? (
            <>
              <DiKV columns={2} items={shown.map(([key, value]) => [<span key={key} className="di-mono">{key}</span>, value])} />
              {config.length > CONFIG_PREVIEW ? (
                <DiFoot
                  left={kit("shown", { shown: shown.length, total: config.length })}
                  right={
                    <button type="button" onClick={() => setShowAll((current) => !current)}>
                      {showAll ? kit("showFewer") : kit("showAll")}
                    </button>
                  }
                />
              ) : null}
            </>
          ) : (
            <p className="di-note">{t("noConfig")}</p>
          )}
        </DiSheet>
      </DiSection>
    </>
  );
}
