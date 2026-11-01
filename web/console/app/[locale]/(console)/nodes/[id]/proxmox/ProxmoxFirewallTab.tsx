"use client";

import { useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiRow } from "./DiList";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

/** Firewall rules in Proxmox order: position, action and direction, then what the rule matches; off rules say so. */
export function ProxmoxFirewallTab({ proxmoxDetails }: Props) {
  const t = useTranslations("devices-proxmox.pve.firewall");
  const rules = [...(proxmoxDetails.firewall_rules ?? [])].sort((left, right) => left.pos - right.pos);

  return (
    <DiList cols="32px minmax(0, 1fr) 48px" colsS="32px minmax(0, 1fr) auto" label={t("label")}>
      <DiCols>
        <span className="r">#</span>
        <span>{t("rule")}</span>
        <span className="r">{t("state")}</span>
      </DiCols>
      {rules.length === 0 ? <DiEmpty title={t("none")} /> : null}
      {rules.map((rule, index) => {
        const ports = [rule.dport ? t("toPort", { port: rule.dport }) : null, rule.sport ? t("fromPort", { port: rule.sport }) : null];
        return (
          <DiRow key={`${rule.pos}-${index}`}>
            <span className="di-num none">{rule.pos}</span>
            <DiMain
              name={[rule.action, rule.type].filter(Boolean).join(" ")}
              grey={
                <DiDots
                  parts={[
                    rule.macro || null,
                    rule.proto || null,
                    rule.source || rule.dest ? `${rule.source || t("any")} → ${rule.dest || t("any")}` : null,
                    ...ports,
                    rule.iface ? t("iface", { iface: rule.iface }) : null,
                    rule.comment || null,
                  ]}
                />
              }
            />
            <span className={`di-num${rule.enable ? "" : " none"}`}>{rule.enable ? t("on") : t("off")}</span>
          </DiRow>
        );
      })}
    </DiList>
  );
}
