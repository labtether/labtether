"use client";

import { useCallback, useState } from "react";
import { Copy, Check } from "lucide-react";

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
    void navigator.clipboard.writeText(text).then(() => {
      onCopy?.();
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [onCopy, text]);
  return (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={`Copy ${label}`}
      className="p-1 rounded hover:bg-[var(--hover)] transition-colors duration-150"
      title={`Copy ${label}`}
    >
      {copied ? (
        <Check size={14} className="text-[var(--ok)]" />
      ) : (
        <Copy size={14} className="text-[var(--muted)]" />
      )}
    </button>
  );
}
