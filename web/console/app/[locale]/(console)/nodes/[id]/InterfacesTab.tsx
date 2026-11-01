"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo } from "react";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { useNetworkInterfaces } from "../../../../hooks/useNetworkInterfaces";
import { copyText, ListState, RefreshButton, RowMenu, SizeCell } from "./systemViewParts";

const isUp = (entry: NetworkInterfaceInfo) => {
  const state = entry.state.trim().toLowerCase();
  return state === "up" || state === "active";
};
const isDown = (entry: NetworkInterfaceInfo) => {
  const state = entry.state.trim().toLowerCase();
  return state === "down" || state === "inactive";
};

/** Interfaces that are up first, then by name. */
function sortInterfaces(entries: NetworkInterfaceInfo[]): NetworkInterfaceInfo[] {
  return [...entries].sort((left, right) => Number(isUp(right)) - Number(isUp(left)) || left.name.localeCompare(right.name));
}

/** The device's network interfaces: state, MAC and MTU, addresses, and bytes received and sent since they came up. */
export function InterfacesTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const { interfaces, loading, error, refresh } = useNetworkInterfaces(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const rows = useMemo(() => sortInterfaces(interfaces), [interfaces]);

  return (
    <div className="dsy">
      <div className="dv-bar">
        <span className="dv-sp" />
        <RefreshButton loading={loading} onClick={() => void refresh()} />
      </div>
      <ListState
        loading={loading}
        hasRows={rows.length > 0}
        error={error}
        loadError={t("interfaces.loadError")}
        empty={t("interfaces.empty")}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-if" role="table" aria-label={t("interfaces.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("interfaces.cols.interface")}</span>
            <span role="columnheader" className="c-addr">{t("interfaces.cols.address")}</span>
            <span role="columnheader" className="r">{t("interfaces.cols.received")}</span>
            <span role="columnheader" className="r c-tx">{t("interfaces.cols.sent")}</span>
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((entry) => {
            const grey = [entry.mac, Number.isFinite(entry.mtu) && entry.mtu > 0 ? t("interfaces.mtu", { mtu: entry.mtu }) : ""].filter(Boolean).join(" · ");
            const addresses = entry.ips?.length ? entry.ips.join(", ") : "";
            const state = entry.state.trim();
            return (
              <div key={entry.name} className="dv-row" role="row" tabIndex={0}>
                <span role="cell" className="dv-ln">
                  <span className="dv-nm">{entry.name || "—"}</span>
                  {isDown(entry) ? <span className="dv-prob bad">{t("interfaces.down")}</span>
                    : !isUp(entry) && state ? <span className="dv-prob busy">{state}</span> : null}
                  {grey ? <span className="dv-grey">{grey}</span> : null}
                </span>
                <span role="cell" className="dv-txt c-addr" title={addresses || undefined}>{addresses || "—"}</span>
                <SizeCell bytes={entry.rx_bytes} title={t("interfaces.packets", { count: entry.rx_packets ?? 0 })} />
                <SizeCell bytes={entry.tx_bytes} className="c-tx" title={t("interfaces.packets", { count: entry.tx_packets ?? 0 })} />
                <RowMenu
                  name={entry.name}
                  items={[
                    ...(addresses ? [{ label: t("interfaces.copyAddress"), onSelect: () => copyText(entry.ips[0] ?? "") }] : []),
                    ...(entry.mac ? [{ label: t("interfaces.copyMac"), onSelect: () => copyText(entry.mac) }] : []),
                    { label: t("interfaces.copyName"), onSelect: () => copyText(entry.name) },
                  ]}
                />
              </div>
            );
          })}
        </div>
      </ListState>
    </div>
  );
}
