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
  /** A short label after the OS, such as "Monitoring only" for the agent's monitoring edition. */
  note?: string;
};

/**
 * The device page's title row: the device's kind in a quiet tile, its name, its state as a word and a time only when
 * something is wrong ("Offline since 21:12"), its address and OS underneath, and the actions on the right.
 */
export function DeviceIdentityBar({ asset, actions, note }: DeviceIdentityBarProps) {
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
          {address || os || note ? (
            <p className="dv-sub">
              {[address ? <span key="a" className="mono">{address}</span> : null, os ? <span key="o">{os}</span> : null, note ? <span key="n">{note}</span> : null]
                .filter((part) => part !== null)
                .flatMap((part, index) => (index === 0 ? [part] : [<span key={`s${index}`} aria-hidden="true"> · </span>, part]))}
            </p>
          ) : null}
        </div>
      </div>
      {actions ? <div className="dv-acts">{actions}</div> : null}
    </div>
  );
}
