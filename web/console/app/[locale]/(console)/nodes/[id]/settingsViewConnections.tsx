"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import type { CSSProperties } from "react";
import { Button } from "../../../../components/ui/Button";
import type { WebService } from "../../../../hooks/useWebServices";
import { formatLogTime, fullTime } from "./logsViewParts";
import type { ProtocolConfig, ProtocolType } from "./useProtocolConfigs";

export const PROTOCOL_LABELS: Record<ProtocolType, string> = {
  ssh: "SSH",
  telnet: "Telnet",
  vnc: "VNC",
  rdp: "RDP",
  ard: "ARD",
};

/** Columns of a connection row: name and grey line, the connect button, ⋯. */
export const CONNECTION_COLS = { "--cols": "minmax(0, 1fr) auto 28px" } as CSSProperties;

export type RowMenuItem = { label: string; onSelect: () => void; danger?: boolean; disabled?: boolean; sep?: boolean };

/** A row's ⋯: a quiet 28 px button that shows on hover or focus, and a Radix menu. */
export function RowMenu({ label, items }: { label: string; items: RowMenuItem[] }) {
  return (
    <Menu.Root>
      <Menu.Trigger className="dst-more" aria-label={label}>
        <MoreHorizontal aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={4}>
          {items.map((item) => (
            <div key={item.label}>
              {item.sep ? <Menu.Separator className="lt-menu-sep" /> : null}
              <Menu.Item
                className={`lt-menu-item${item.danger ? " dst-danger" : ""}`}
                disabled={item.disabled}
                onSelect={item.onSelect}
              >
                {item.label}
              </Menu.Item>
            </div>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

type ProtocolRowProps = {
  config: ProtocolConfig;
  onConnect?: () => void;
  onTest: () => void;
  onPushKey?: () => void;
  onEdit: () => void;
  onRemove: () => void;
  removing?: boolean;
};

/** One SSH/Telnet/VNC/RDP/ARD connection: where it points, when it was last tested, Connect and ⋯. */
export function ProtocolRow({ config, onConnect, onTest, onPushKey, onEdit, onRemove, removing = false }: ProtocolRowProps) {
  const t = useTranslations("devices.settingsTab.connections");
  const name = PROTOCOL_LABELS[config.protocol] ?? config.protocol;
  const where = config.host
    ? `${config.username ? `${config.username}@` : ""}${config.host}:${config.port}`
    : t("port", { port: config.port });
  const tested = config.last_tested_at ? formatLogTime(config.last_tested_at) : "";
  const failed = config.test_status === "failed";
  const testWord = failed
    ? t("testFailed")
    : config.test_status === "success" && tested
      ? t("tested", { time: tested })
      : t("notTested");
  const items: RowMenuItem[] = [
    { label: t("test"), onSelect: onTest },
    ...(onPushKey ? [{ label: t("pushKey"), onSelect: onPushKey }] : []),
    { label: t("edit"), onSelect: onEdit },
    { label: t("remove"), onSelect: onRemove, danger: true, disabled: removing, sep: true },
  ];
  return (
    <div className="dv-row dst-conn">
      <span className="dv-ln">
        {failed ? <span className="dv-dot bad dst-dot" aria-hidden="true" /> : null}
        <span className="dv-nm">{name}</span>
        <span className="dv-grey">{where}</span>
        <span
          className={`dv-grey${failed ? " bad" : ""}`}
          title={failed && config.test_error ? config.test_error : config.last_tested_at ? fullTime(config.last_tested_at) : undefined}
        >
          {testWord}
        </span>
      </span>
      {onConnect ? <Button size="sm" onClick={onConnect}>{t("connect")}</Button> : <span />}
      <RowMenu label={t("actionsFor", { name })} items={items} />
    </div>
  );
}

type WebServiceRowProps = {
  service: WebService;
  onOpen: () => void;
  onEdit: () => void;
  onRemove: () => void;
};

/** One web address on this device: its name and URL, Open and ⋯. */
export function WebServiceRow({ service, onOpen, onEdit, onRemove }: WebServiceRowProps) {
  const t = useTranslations("devices.settingsTab.connections");
  const down = service.status === "down";
  return (
    <div className="dv-row dst-conn">
      <span className="dv-ln">
        {down ? <span className="dv-dot bad dst-dot" aria-hidden="true" /> : null}
        <span className="dv-nm">{service.name}</span>
        {down ? <span className="dv-prob bad">{t("down")}</span> : null}
        <span className="dv-grey">{service.url}</span>
      </span>
      <Button size="sm" onClick={onOpen}>{t("open")}</Button>
      <RowMenu
        label={t("actionsFor", { name: service.name })}
        items={[
          { label: t("edit"), onSelect: onEdit },
          { label: t("remove"), onSelect: onRemove, danger: true, sep: true },
        ]}
      />
    </div>
  );
}
