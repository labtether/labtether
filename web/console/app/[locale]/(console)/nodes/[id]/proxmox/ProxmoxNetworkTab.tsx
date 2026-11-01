"use client";

import { useTranslations } from "next-intl";
import type { NetworkInterface } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiRow } from "./DiList";

type Props = {
  networkInterfaces: NetworkInterface[];
};

/** The node's network interfaces as Proxmox configures them: type, address, gateway, bridge ports and method. */
export function ProxmoxNetworkTab({ networkInterfaces }: Props) {
  const t = useTranslations("devices-proxmox.pve.network");
  const list = [...networkInterfaces].sort((left, right) => (left.iface ?? "").localeCompare(right.iface ?? ""));

  return (
    <DiList cols="minmax(0, 1fr) minmax(120px, 200px) 80px" colsS="minmax(0, 1fr) auto" label={t("label")}>
      <DiCols>
        <span>{t("interface")}</span>
        <span className="di-s">{t("address")}</span>
        <span className="r">{t("state")}</span>
      </DiCols>
      {list.length === 0 ? <DiEmpty title={t("none")} /> : null}
      {list.map((iface, index) => (
        <DiRow key={`${iface.iface ?? "iface"}-${index}`}>
          <DiMain
            name={iface.iface || "—"}
            grey={
              <DiDots
                parts={[
                  iface.type || null,
                  iface.bridge_ports ? t("ports", { ports: iface.bridge_ports }) : null,
                  iface.gateway ? t("gateway", { gateway: iface.gateway }) : null,
                  iface.method || null,
                  iface.autostart === 1 ? t("autostart") : null,
                ]}
              />
            }
          />
          <span className="di-num di-s">{iface.cidr || iface.address || "—"}</span>
          <span className={`di-num${iface.active === 1 ? "" : " none"}`}>{iface.active === 1 ? t("active") : t("inactive")}</span>
        </DiRow>
      ))}
    </DiList>
  );
}
