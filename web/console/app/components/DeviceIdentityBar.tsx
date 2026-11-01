"use client";

import type { ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { Asset } from "../console/models";
import { deviceIcon } from "../[locale]/(console)/nodes/deviceListModel";
import { deviceAddress, deviceOS, deviceState, shortWhen } from "../[locale]/(console)/nodes/[id]/deviceStatusModel";

type DeviceIdentityBarProps = {
  asset: Asset;
  /** The connect buttons and the ⋯ menu, right of the name. */
  actions?: ReactNode;
};

/**
 * The device page's title row: the device's kind in a quiet tile, its name, its state as a word and a time only when
 * something is wrong ("Offline since 21:12"), its address and OS underneath, and the actions on the right.
 */
export function DeviceIdentityBar({ asset, actions }: DeviceIdentityBarProps) {
  const t = useTranslations("devices.page.state");
  const locale = useLocale();
  const Icon = deviceIcon(asset);
  const state = deviceState(asset);
  const when = state.since ? shortWhen(state.since, locale) : "";
  const word = state.word === "online" ? "" : when && state.word !== "noCheckIn" ? t(`${state.word}Since`, { time: when }) : t(state.word);
  const address = deviceAddress(asset);
  const os = deviceOS(asset);

  return (
    <div className="dv-head">
      <div className="dv-id">
        <span className="ld-tile" aria-hidden="true">
          <Icon strokeWidth={1.75} />
        </span>
        <div className="dv-id-txt">
          <div className="dv-title">
            <h1>{asset.name}</h1>
            {word ? (
              <span className={`dv-status ${state.tone}`}>
                <i className={`dv-dot ${state.tone}`} aria-hidden="true" />
                {word}
              </span>
            ) : null}
          </div>
          {address || os ? (
            <p className="dv-sub">
              {address ? <span className="mono">{address}</span> : null}
              {address && os ? <span aria-hidden="true"> · </span> : null}
              {os ? <span>{os}</span> : null}
            </p>
          ) : null}
        </div>
      </div>
      {actions ? <div className="dv-acts">{actions}</div> : null}
    </div>
  );
}
