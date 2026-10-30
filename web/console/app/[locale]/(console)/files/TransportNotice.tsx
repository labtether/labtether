"use client";

import { AlertTriangle } from "lucide-react";
import { useTranslations } from "next-intl";
import type { FileConnection } from "./fileConnectionsClient";

// The Hub lists a transport_notice for a saved WebDAV connection that still
// relies on plain HTTP without the 1.7.1 opt-in. Opening it fails with 409,
// so the list and the edit form say why and how to fix it.
export function TransportNotice({ connection, className = "" }: { connection?: FileConnection; className?: string }) {
  const t = useTranslations("files");
  if (!connection?.transport_notice?.trim()) return null;
  return (
    <p role="note" className={`flex items-start gap-1.5 text-[var(--warn)] ${className}`}>
      <AlertTriangle className="w-3.5 h-3.5 mt-px shrink-0" aria-hidden="true" />
      <span>{t("webdavTransportNotice")}</span>
    </p>
  );
}
