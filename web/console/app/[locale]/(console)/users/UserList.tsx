"use client";

import { useMemo, useState } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, ChevronUp, MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import type { HubUser } from "../../../hooks/useHubUsers";
import { normalizeHubRole } from "../../../lib/roles";
import { hasTwoFactorState, isLocalUser, nextSort, sortUsers, type UserSort, type UserSortKey } from "./usersModel";

export type UserAction = "role" | "password" | "signOut" | "remove";

type UserListProps = {
  users: HubUser[];
  loading: boolean;
  currentUserId: string;
  canManage: (user: HubUser) => boolean;
  onAction: (action: UserAction, user: HubUser) => void;
};

/** Users as one 2.0 list: the name over "You · Local account", the role word, 2FA when the hub reports it, and a ⋯ menu. */
export function UserList({ users, loading, currentUserId, canManage, onAction }: UserListProps) {
  const t = useTranslations("users");
  const tc = useTranslations("common");
  const [sort, setSort] = useState<UserSort>({ key: "name", desc: false });
  const rows = useMemo(() => sortUsers(users, sort), [users, sort]);
  const twoFactor = hasTwoFactorState(users);
  const grid = `pp-u${twoFactor ? " tfa" : ""}`;

  const head = (key: UserSortKey, label: string) => {
    const on = sort.key === key;
    const Arrow = sort.desc ? ChevronDown : ChevronUp;
    return (
      <span role="columnheader" aria-sort={on ? (sort.desc ? "descending" : "ascending") : "none"}>
        <button type="button" className={`pp-sort${on ? " on" : ""}`} onClick={() => setSort(nextSort(sort, key))}>
          {label}
          {on ? <Arrow aria-hidden="true" /> : null}
        </button>
      </span>
    );
  };

  return (
    <div className="pp-list pp-hang" role="table" aria-label={t("title")}>
      <div className={`pp-cols ${grid}`} role="row">
        {head("name", t("list.user"))}
        {head("role", t("list.role"))}
        {twoFactor ? <span role="columnheader">{t("list.twoFactor")}</span> : null}
        <span role="columnheader"><span className="sr-only">{t("list.actions")}</span></span>
      </div>
      {loading && users.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{tc("loading")}</p></div> : null}
      {!loading && users.length === 0 ? <div role="row"><p role="cell" className="pp-empty">{t("list.empty")}</p></div> : null}
      {rows.map((user) => {
        const you = user.id === currentUserId;
        const kind = isLocalUser(user) ? t("list.local") : t("list.sso");
        return (
          <div key={user.id} className={`pp-row ${grid}`} role="row">
            <span className="pp-in" role="cell">
              <span className="pp-nm">{user.username}</span>
              <span className="pp-grey">{you ? t("list.youAnd", { kind }) : kind}</span>
            </span>
            <span className="pp-c" role="cell">{t(`roles.${normalizeHubRole(user.role)}`)}</span>
            {twoFactor ? (
              <span className={`pp-c${user.totp_enabled ? "" : " pp-off"}`} role="cell">{user.totp_enabled ? t("list.on") : t("list.off")}</span>
            ) : null}
            <span className="pp-end" role="cell">
              {canManage(user) ? <UserMenu user={user} you={you} onAction={onAction} /> : null}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function UserMenu({ user, you, onAction }: { user: HubUser; you: boolean; onAction: (action: UserAction, user: HubUser) => void }) {
  const t = useTranslations("users");
  const owner = normalizeHubRole(user.role) === "owner";
  return (
    <Menu.Root>
      <Menu.Trigger className="pp-more" aria-label={t("list.actionsFor", { name: user.username })}>
        <MoreHorizontal aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="end" sideOffset={4}>
          {!owner ? <Menu.Item className="lt-menu-item" onSelect={() => onAction("role", user)}>{t("menu.role")}</Menu.Item> : null}
          {isLocalUser(user) ? <Menu.Item className="lt-menu-item" onSelect={() => onAction("password", user)}>{t("menu.password")}</Menu.Item> : null}
          <Menu.Item className="lt-menu-item" onSelect={() => onAction("signOut", user)}>{t("menu.signOut")}</Menu.Item>
          {!owner && !you ? (
            <>
              <Menu.Separator className="lt-menu-sep" />
              <Menu.Item className="lt-menu-item pp-danger" onSelect={() => onAction("remove", user)}>{t("menu.remove")}</Menu.Item>
            </>
          ) : null}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
