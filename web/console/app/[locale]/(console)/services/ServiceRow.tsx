"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { GripVertical, MoreHorizontal } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { memo, useRef, type KeyboardEvent, type MouseEvent } from "react";
import { useRouter } from "../../../../i18n/navigation";
import { ServiceIcon, serviceIconKey } from "../../../components/ServiceIcon";
import type { WebService } from "../../../hooks/useWebServices";
import { safeHttpURL } from "../../../../lib/safeExternalURL";
import { ServiceDetailPanel } from "./ServiceDetailPanel";
import { formatResponseTime } from "./servicesLayoutHelpers";
import { formatCheckTime, isDown, isHidden, isManual, serviceLine, SLOW_MS } from "./servicesListModel";
import { useCanWrite } from "../../../hooks/useCanWrite";

/** What a row can ask the page to do. The service is passed back so the handlers stay stable across rows. */
export type ServiceRowActions = {
  onSelect: (service: WebService) => void;
  onToggleDetails: (service: WebService) => void;
  onEdit: (service: WebService) => void;
  onToggleHidden: (service: WebService) => void;
  onRemove: (service: WebService) => void;
  onCopy: (service: WebService) => void;
  onDragStart: (service: WebService) => void;
  onDragOver: (service: WebService) => void;
  onDrop: (service: WebService) => void;
  onDragEnd: () => void;
};

type ServiceRowProps = {
  service: WebService;
  hostName: string;
  selected: boolean;
  open: boolean;
  /** The open row's service as loaded in full (recent checks, other addresses). */
  detail?: WebService;
  arranging: boolean;
  dragging: boolean;
  dragOver: boolean;
  actions: ServiceRowActions;
};

/** The grey text after the name: the problem in red, else the device and the address (and Hidden). */
function useServiceLine(service: WebService, hostName: string) {
  const t = useTranslations("services.list");
  const locale = useLocale();
  if (isDown(service)) {
    const since = formatCheckTime(service.health?.last_change_at, locale);
    return { tone: "bad", text: since ? t("downSince", { time: since }) : t("down") };
  }
  const parts = [serviceLine(service.url, hostName)];
  if (service.status !== "up") parts.unshift(t("unknown"));
  if (isHidden(service)) parts.push(t("hidden"));
  return { tone: "", text: parts.join(" · ") };
}

function ServiceRowView({ service, hostName, selected, open, detail, arranging, dragging, dragOver, actions }: ServiceRowProps) {
  const t = useTranslations("services.list");
  const router = useRouter();
  const canWrite = useCanWrite();
  const rowRef = useRef<HTMLDivElement>(null);
  const line = useServiceLine(service, hostName);
  const href = safeHttpURL(service.url);
  const down = isDown(service);
  const tags = (service.metadata?.user_tags ?? "").split(",").map((tag) => tag.trim()).filter(Boolean);
  const ms = !down && service.response_ms > 0 ? service.response_ms : 0;
  const label = hostName ? t("rowLabel", { name: service.name, host: hostName }) : service.name;

  const onRowClick = (event: MouseEvent<HTMLDivElement>) => {
    if (arranging || (event.target as HTMLElement).closest("a, button, input, [role='menu']")) return;
    actions.onToggleDetails(service);
  };
  const onRowKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return;
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      actions.onToggleDetails(service);
    } else if (event.key === "o" && href) {
      window.open(href, "_blank", "noopener,noreferrer");
    }
  };

  return (
    <>
      <div
        ref={rowRef}
        className={`sv-row${selected ? " sel" : ""}${open ? " open" : ""}${dragging ? " drag" : ""}${dragOver ? " over" : ""}`}
        tabIndex={0}
        role="group"
        aria-label={label}
        data-service-row={service.name}
        draggable={arranging}
        onClick={onRowClick}
        onKeyDown={onRowKey}
        onDragStart={(event) => {
          event.dataTransfer.effectAllowed = "move";
          event.dataTransfer.setData("text/plain", service.id);
          actions.onDragStart(service);
        }}
        onDragOver={(event) => {
          if (!arranging) return;
          event.preventDefault();
          actions.onDragOver(service);
        }}
        onDrop={(event) => {
          if (!arranging) return;
          event.preventDefault();
          actions.onDrop(service);
        }}
        onDragEnd={actions.onDragEnd}
      >
        <span className="sv-lead">
          {arranging ? (
            <GripVertical className="sv-grip" aria-hidden="true" />
          ) : (
            <>
              {down ? <i className="sv-dot bad" /> : service.status !== "up" ? <i className="sv-dot unk" /> : null}
              <input type="checkbox" className="sv-cbx" checked={selected} onChange={() => actions.onSelect(service)} aria-label={t("select", { name: label })} />
            </>
          )}
        </span>
        <span className="sv-ic" aria-hidden="true"><ServiceIcon iconKey={serviceIconKey(service)} size={16} /></span>
        <span className="sv-main">
          {href ? (
            <a className="sv-nm" href={href} target="_blank" rel="noopener noreferrer" data-service-name={service.name} draggable={false}>{service.name}</a>
          ) : (
            <span className="sv-nm" data-service-name={service.name}>{service.name}</span>
          )}
          {tags.length > 0 && <span className="sv-tags">{tags.map((tag) => <span key={tag} className="sv-tag">{tag}</span>)}</span>}
          <span className={`sv-grey${line.tone ? ` ${line.tone}` : ""}`}>{line.text}</span>
        </span>
        <span className={`sv-ms${ms ? (ms > SLOW_MS ? " hot" : "") : " none"}`}>{ms ? formatResponseTime(ms) : "—"}</span>
        <Menu.Root>
          <Menu.Trigger className="sv-more" aria-label={t("actionsFor", { name: label })}>
            <MoreHorizontal aria-hidden="true" strokeWidth={2} />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content
              className="lt-menu sv-menu"
              align="end"
              sideOffset={4}
              onCloseAutoFocus={(event) => {
                // Back to the row (not its ⋯), so the row keeps its keys and the button no longer shows a ring.
                event.preventDefault();
                rowRef.current?.focus({ preventScroll: true });
              }}
            >
              {href && (
                <Menu.Item className="lt-menu-item" onSelect={() => window.open(href, "_blank", "noopener,noreferrer")}>{t("open")}</Menu.Item>
              )}
              <Menu.Item className="lt-menu-item" onSelect={() => actions.onCopy(service)}>{t("copyAddress")}</Menu.Item>
              {service.host_asset_id && (
                <Menu.Item className="lt-menu-item" onSelect={() => router.push(`/nodes/${encodeURIComponent(service.host_asset_id)}`)}>
                  {t("goTo", { host: hostName || service.host_asset_id })}
                </Menu.Item>
              )}
              <Menu.Separator className="lt-menu-sep" />
              <Menu.Item className="lt-menu-item" onSelect={() => actions.onToggleDetails(service)}>{open ? t("hideDetails") : t("details")}</Menu.Item>
              {canWrite ? <Menu.Item className="lt-menu-item" onSelect={() => actions.onEdit(service)}>{t("edit")}</Menu.Item> : null}
              {canWrite ? <Menu.Item className="lt-menu-item" onSelect={() => actions.onToggleHidden(service)}>{isHidden(service) ? t("unhide") : t("hide")}</Menu.Item> : null}
              {canWrite && isManual(service) && (
                <>
                  <Menu.Separator className="lt-menu-sep" />
                  <Menu.Item className="lt-menu-item sv-danger" onSelect={() => actions.onRemove(service)}>{t("remove")}</Menu.Item>
                </>
              )}
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
      {open && !arranging && <ServiceDetailPanel service={detail ?? service} hostName={hostName} />}
    </>
  );
}

export const ServiceRow = memo(ServiceRowView);
