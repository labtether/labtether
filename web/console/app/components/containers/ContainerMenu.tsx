"use client";

import { useTranslations } from "next-intl";
import { useCallback, useState } from "react";
import { useAuth } from "../../contexts/AuthContext";
import { useToast } from "../../contexts/ToastContext";
import { canAccessHostData, hasWriteRole } from "../../lib/roles";
import { copyText, runContainerAction, type CtAction, type CtTarget } from "./containerActions";
import { RowMenu, useConfirm, type MenuEntry } from "./RowMenu";

export type ContainerMenuProps = {
  target: CtTarget;
  name: string;
  image: string;
  /** The container's page; Logs and Shell open its panels. */
  href: string;
  running: boolean;
  /** Restarting or removing: no start, stop or restart until it settles. */
  busy?: boolean;
  /** The menu is on the container's own page (no Open item, header-sized button). */
  onPage?: boolean;
  /** More items after Open, Logs and Shell (Portainer's Shell). */
  extra?: MenuEntry[];
  onChanged?: () => void;
};

/** Runs one container action with a toast, and reports which one is in flight. */
export function useContainerAction(name: string, target: CtTarget, onChanged?: () => void) {
  const t = useTranslations("containers.toast");
  const { addToast } = useToast();
  const [pending, setPending] = useState<CtAction | null>(null);
  const run = useCallback(async (action: CtAction) => {
    setPending(action);
    try {
      await runContainerAction(target, action);
      addToast("success", t(`${action}Done`, { name }));
      setTimeout(() => onChanged?.(), 900);
    } catch (err) {
      const why = err instanceof Error && err.message && err.message !== action ? `: ${err.message}` : "";
      addToast("error", `${t(`${action}Failed`, { name })}${why}`);
    } finally {
      setPending(null);
    }
  }, [addToast, name, onChanged, t, target]);
  return { run, pending };
}

/** The ⋯ of one container: Open, Logs, Shell, Restart, Stop… or Start, Copy image. */
export function ContainerMenu({ target, name, image, href, running, busy = false, onPage = false, extra = [], onChanged }: ContainerMenuProps) {
  const t = useTranslations("containers.menu");
  const tc = useTranslations("common");
  const { user } = useAuth();
  const { addToast } = useToast();
  const canWrite = hasWriteRole(user?.role);
  const docker = target.source === "docker";
  const { run } = useContainerAction(name, target, onChanged);
  const [ask, dialog] = useConfirm();

  const items: MenuEntry[] = [];
  if (!onPage) items.push({ label: t("open"), href });
  if (docker && !onPage && canAccessHostData(user?.role)) {
    items.push({ label: t("logs"), href: `${href}?panel=logs` });
    if (running) items.push({ label: t("shell"), href: `${href}?panel=terminal` });
  }
  items.push(...extra);
  if (canWrite && !busy) {
    items.push("sep");
    if (running) {
      items.push({ label: t("restart"), onSelect: () => void run("restart") });
      items.push({
        label: t("stop"),
        danger: true,
        onSelect: () => ask({ title: t("stopTitle", { name }), body: t("stopBody"), confirm: t("stopConfirm"), danger: true, run: () => run("stop") }),
      });
    } else {
      items.push({ label: t("start"), onSelect: () => void run("start") });
    }
  }
  if (image) {
    items.push("sep", {
      label: t("copyImage"),
      onSelect: () => void copyText(image).then((ok) => { if (ok) addToast("success", tc("copied")); }),
    });
  }

  return (
    <>
      <RowMenu label={t("label", { name })} items={items} size={onPage ? "ctl" : "row"} />
      {dialog}
    </>
  );
}
