"use client";

import { useCallback, useState } from "react";
import { Copy, Check } from "lucide-react";
import { copyText } from "../../lib/clipboard";

/** A copy icon button; `label` is its full accessible name, such as "Copy enrollment token". */
export function CopyButton({
  text,
  label,
  onCopy,
}: {
  text: string;
  label: string;
  onCopy?: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    void copyText(text).then(() => {
      onCopy?.();
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }, () => setCopied(false));
  }, [onCopy, text]);
  return (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={label}
      className="p-1 rounded hover:bg-[var(--hover)] transition-colors duration-150"
      title={label}
    >
      {copied ? (
        <Check size={14} className="text-[var(--ok)]" />
      ) : (
        <Copy size={14} className="text-[var(--muted)]" />
      )}
    </button>
  );
}
