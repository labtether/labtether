"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";

import { formatBytes as formatByteTotal } from "../../../../console/formatters";
import type { NetworkInterfaceInfo } from "../../../../hooks/useNetworkInterfaces";
import { interfaceTraffic } from "./systemPanelModel";

/** The four busiest interfaces: name, a state word when not up, the first address, and bytes in and out. */
export function SystemPanelNetworkInterfacePreview({
  interfaces,
  loading,
  error,
}: {
  interfaces: NetworkInterfaceInfo[];
  loading: boolean;
  error: string | null;
}) {
  const t = useTranslations("devices.hardware");
  const topInterfaces = useMemo(() => {
    return [...interfaces]
      .sort((left, right) => interfaceTraffic(right) - interfaceTraffic(left))
      .slice(0, 4);
  }, [interfaces]);

  let body;
  if (loading && interfaces.length === 0) {
    body = <p className="dv-empty">{t("interfacesLoading")}</p>;
  } else if (error) {
    body = (
      <div className="dv-empty">
        <b>{t("interfacesError")}</b>
        <span className="dv-err">{error}</span>
      </div>
    );
  } else if (topInterfaces.length === 0) {
    body = <p className="dv-empty"><b>{t("interfacesNone")}</b></p>;
  } else {
    body = (
      <>
        <div className="dv-cols dmt-if" role="row">
          <span>{t("interface")}</span>
          <span className="r">{t("received")}</span>
          <span className="r">{t("sent")}</span>
        </div>
        {topInterfaces.map((entry) => {
          const state = entry.state.trim().toLowerCase();
          const up = state === "up" || state === "active";
          const address = entry.ips[0] || entry.mac || "";
          return (
            <div key={entry.name} className="dv-row dmt-if">
              <span className="dv-ln">
                <span className="dv-nm">{entry.name || t("unnamed")}</span>
                {up ? null : <span className="dv-prob warn">{entry.state || t("stateUnknown")}</span>}
                {address ? <span className="dv-grey">{address}</span> : null}
              </span>
              <span className="dv-n">{formatByteTotal(Math.max(0, entry.rx_bytes))}</span>
              <span className="dv-n">{formatByteTotal(Math.max(0, entry.tx_bytes))}</span>
            </div>
          );
        })}
      </>
    );
  }

  return (
    <section className="dv-sec" aria-labelledby="dmt-busy">
      <div className="dv-sh"><h2 id="dmt-busy">{t("busiestInterfaces")}</h2></div>
      <div className="dv-list">{body}</div>
    </section>
  );
}
