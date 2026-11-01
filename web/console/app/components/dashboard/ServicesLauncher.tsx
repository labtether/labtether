"use client";

import { useMemo } from "react";
import type { Asset } from "../../console/models";
import type { WebService } from "../../hooks/useWebServices";
import { ServiceIcon } from "../ServiceIcon";

/** Every web service's icon and name, A–Z, one click from the Dashboard. Two of the same app say which machine each is on. */
export function ServicesLauncher({ services, assets }: { services: WebService[]; assets: Asset[] }) {
  const list = useMemo(() => {
    const hostName = new Map(assets.map((a) => [a.id, a.name]));
    const web = services
      .filter((s) => /^https?:\/\//i.test(s.url))
      .map((s) => ({ s, host: hostName.get(s.host_asset_id) ?? "" }))
      .sort((a, b) => a.s.name.localeCompare(b.s.name) || a.host.localeCompare(b.host));
    const seen = new Map<string, number>();
    for (const { s } of web) seen.set(s.name, (seen.get(s.name) ?? 0) + 1);
    return web.map((item) => ({ ...item, twice: (seen.get(item.s.name) ?? 0) > 1 }));
  }, [services, assets]);

  return (
    <div className="db-apps">
      {list.map(({ s, host, twice }) => (
        <a key={s.id} href={s.url} target="_blank" rel="noopener noreferrer" className={`db-app${s.status === "down" ? " down" : ""}`} title={host ? `${s.name} · ${host}` : s.name}>
          <ServiceIcon iconKey={s.icon_key} size={20} />
          <span className="db-app-n">
            {s.name}
            {twice && host && <small>{host}</small>}
          </span>
          {s.status === "down" && <i className="db-dot bad" aria-hidden="true" />}
        </a>
      ))}
    </div>
  );
}
