"use client";

import { ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
import type { AddDeviceCompatPrefill } from "./types";

export type SourceType = "agent" | "proxmox" | "pbs" | "docker" | "portainer" | "truenas" | "homeassistant" | "manual";

/** The systems, grouped the way they connect: the agent, an API, or by hand. */
const GROUPS: Array<{ id: "agent" | "api" | "other"; sources: SourceType[] }> = [
  { id: "agent", sources: ["agent"] },
  { id: "api", sources: ["proxmox", "pbs", "truenas", "docker", "portainer", "homeassistant"] },
  { id: "other", sources: ["manual"] },
];

type SourcePickerProps = {
  onSelect: (source: SourceType) => void;
  prefillBySource?: Partial<Record<SourceType, AddDeviceCompatPrefill>>;
};

/** One plain row per system: its name over one grey line, and where LabTether already found one. */
export function SourcePicker({ onSelect, prefillBySource = {} }: SourcePickerProps) {
  const t = useTranslations("auth.addDevice");
  return (
    <div className="si-ad-pick">
      {GROUPS.map((group) => (
        <section key={group.id} className="si-ad-grp" aria-labelledby={`si-ad-g-${group.id}`}>
          <h3 id={`si-ad-g-${group.id}`} className="si-ad-gh">{t(`groups.${group.id}`)}</h3>
          {group.sources.map((source) => {
            const found = prefillBySource[source];
            return (
              <button key={source} type="button" className="si-ad-src" onClick={() => onSelect(source)}>
                <span className="si-ad-tx">
                  <span className="si-ad-nm">{t(`sources.${source}.name`)}</span>
                  <span className="si-ad-sub">
                    {found
                      ? t("found", { name: found.serviceName || found.baseURL })
                      : t(`sources.${source}.sub`)}
                  </span>
                </span>
                <ChevronRight aria-hidden="true" strokeWidth={2} />
              </button>
            );
          })}
        </section>
      ))}
    </div>
  );
}
