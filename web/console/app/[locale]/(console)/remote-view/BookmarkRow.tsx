"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Ellipsis } from "lucide-react";
import { useTranslations } from "next-intl";
import { bookmarkBlocked } from "./newTabModel";
import type { RemoteBookmark } from "./remoteBookmarksClient";

/**
 * A bookmark: its name opens it; the grey text is how it is reached. A VNC bookmark without plain VNC allowed says so
 * and opens only after Edit. ⋯ holds Edit and Remove.
 */
export function BookmarkRow({
  bookmark,
  onOpen,
  onEdit,
  onRemove,
}: {
  bookmark: RemoteBookmark;
  onOpen: () => void;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const t = useTranslations("desktop.page");
  const blocked = bookmarkBlocked(bookmark);
  return (
    <div className={`rv-row${blocked ? " off" : ""}`}>
      <span className="rv-lead" />
      <span className="rv-main">
        <button type="button" className="rv-open" onClick={onOpen} disabled={blocked}>
          {bookmark.label}
        </button>
        {blocked ? <span className="rv-prob warn">{t("plainVncMissing")}</span> : null}
        <span className="rv-g">
          {`${bookmark.protocol.toUpperCase()} · ${bookmark.host}:${bookmark.port}`}
        </span>
      </span>
      <Menu.Root modal={false}>
        <Menu.Trigger className="rv-ib rv-more" aria-label={t("bookmarkActions", { name: bookmark.label })}>
          <Ellipsis aria-hidden="true" />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            <Menu.Item className="lt-menu-item" onSelect={onEdit}>
              {t("edit")}
            </Menu.Item>
            <Menu.Separator className="lt-menu-sep" />
            <Menu.Item className="lt-menu-item rv-danger" onSelect={onRemove}>
              {t("removeItem")}
            </Menu.Item>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
