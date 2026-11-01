"use client";

import * as Popover from "@radix-ui/react-popover";
import { ClipboardList, Download, Folder, Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";
import { useContext, useEffect, useRef, useState } from "react";
import type { RemoteViewToolbarProps } from "./types";
import { ToolButton, ToolItem, ToolMenu, ToolbarPortalContext } from "./ToolbarButtons";

/**
 * State that outlives the menus: the last clipboard sync's result, shown for 1.5 s, and a download path being typed
 * (kept while its popover is closed).
 */
export function useToolbarTransferState(clipboardLastSync: RemoteViewToolbarProps["clipboardLastSync"] = "idle") {
  const [clipboardFlash, setClipboardFlash] = useState<"success" | "error" | null>(null);
  const [downloadPath, setDownloadPath] = useState("");
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const prevLastSync = useRef(clipboardLastSync);
  useEffect(() => {
    if (clipboardLastSync !== prevLastSync.current && clipboardLastSync !== "idle") {
      setClipboardFlash(clipboardLastSync);
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        setClipboardFlash(null);
      }, 1500);
    }
    prevLastSync.current = clipboardLastSync;
  }, [clipboardLastSync]);
  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    [],
  );
  return { clipboardFlash, downloadPath, setDownloadPath };
}

type TransferProps = Pick<
  RemoteViewToolbarProps,
  | "onClipboardPull"
  | "onClipboardPush"
  | "clipboardSyncing"
  | "onFileDrawerToggle"
  | "fileDrawerOpen"
  | "fileDownloading"
  | "onDownloadFile"
> & {
  transfer: ReturnType<typeof useToolbarTransferState>;
  onOpenChange: (open: boolean) => void;
};

/** Clipboard: pull the remote clipboard to this computer, or push this computer's clipboard to the remote one. */
export function ToolbarClipboard({ onClipboardPull, onClipboardPush, clipboardSyncing = false, onOpenChange }: TransferProps) {
  const t = useTranslations("desktop.tools");
  if (!onClipboardPull && !onClipboardPush) return null;
  return (
    <ToolMenu
      icon={clipboardSyncing ? <Loader2 className="rv-spin" aria-hidden="true" /> : <ClipboardList aria-hidden="true" />}
      label={t("clipboard")}
      onOpenChange={onOpenChange}
    >
      {onClipboardPull ? <ToolItem onSelect={onClipboardPull}>{t("pull")}</ToolItem> : null}
      {onClipboardPush ? <ToolItem onSelect={onClipboardPush}>{t("push")}</ToolItem> : null}
    </ToolMenu>
  );
}

/**
 * Files: opens the file drawer when the device's agent can browse files, else a field for one remote path to download,
 * else stays disabled and says why.
 */
export function ToolbarFiles(props: TransferProps) {
  const t = useTranslations("desktop.tools");
  const { onFileDrawerToggle, fileDrawerOpen = false, fileDownloading = false, onDownloadFile } = props;
  const icon = fileDownloading ? <Loader2 className="rv-spin" aria-hidden="true" /> : <Folder aria-hidden="true" />;
  if (onFileDrawerToggle) {
    return (
      <ToolButton label={t("files")} pressed={fileDrawerOpen} onClick={onFileDrawerToggle}>
        {icon}
      </ToolButton>
    );
  }
  if (onDownloadFile) return <DownloadPopover {...props} />;
  return (
    <ToolButton label={t("filesNeedAgent")} disabled>
      <Folder aria-hidden="true" />
    </ToolButton>
  );
}

function DownloadPopover({ onDownloadFile, fileDownloading = false, transfer, onOpenChange }: TransferProps) {
  const t = useTranslations("desktop.tools");
  const container = useContext(ToolbarPortalContext);
  const [open, setOpen] = useState(false);
  const setBoth = (next: boolean) => {
    setOpen(next);
    onOpenChange(next);
  };
  const submit = () => {
    const path = transfer.downloadPath.trim();
    if (!path) return;
    onDownloadFile?.(path);
    transfer.setDownloadPath("");
    setBoth(false);
  };
  return (
    <Popover.Root open={open} onOpenChange={setBoth}>
      <Popover.Trigger asChild>
        <ToolButton label={t("downloadFile")} pressed={open}>
          {fileDownloading ? <Loader2 className="rv-spin" aria-hidden="true" /> : <Download aria-hidden="true" />}
        </ToolButton>
      </Popover.Trigger>
      <Popover.Portal container={container ?? undefined}>
        <Popover.Content className="rv-pop" align="end" sideOffset={6} collisionPadding={8}>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            <label className="rv-field-l" htmlFor="rv-download-path">
              {t("downloadPath")}
            </label>
            <input
              id="rv-download-path"
              className="rv-input mono"
              placeholder="/path/to/file"
              value={transfer.downloadPath}
              autoComplete="off"
              spellCheck={false}
              onChange={(event) => transfer.setDownloadPath(event.target.value)}
              onKeyDown={(event) => event.stopPropagation()}
            />
            <div className="rv-pop-foot">
              <button type="submit" className="rv-btn" disabled={!transfer.downloadPath.trim()}>
                {t("download")}
              </button>
            </div>
          </form>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
