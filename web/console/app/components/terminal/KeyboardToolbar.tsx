"use client";

import { useState, useCallback, type RefObject } from "react";
import { useTranslations } from "next-intl";
import type { XTerminalHandle } from "../XTerminal";

const DEFAULT_KEYS = ["Esc", "Ctrl", "Alt", "Tab", "\u2191", "\u2193", "\u2190", "\u2192", "|", "~", "$", "/"];

const MODIFIER_KEYS = new Set(["Ctrl", "Alt"]);

/** Map display label to the escape sequence or character to send. */
function keyToSequence(key: string): string {
  switch (key) {
    case "Esc":
      return "\x1b";
    case "Tab":
      return "\t";
    case "\u2191":
      return "\x1b[A";
    case "\u2193":
      return "\x1b[B";
    case "\u2192":
      return "\x1b[C";
    case "\u2190":
      return "\x1b[D";
    default:
      return key;
  }
}

interface KeyboardToolbarProps {
  termRef: RefObject<XTerminalHandle | null>;
  keys?: string[];
  visible?: boolean;
}

export default function KeyboardToolbar({
  termRef,
  keys = DEFAULT_KEYS,
  visible,
}: KeyboardToolbarProps) {
  const t = useTranslations("terminal.keys");
  const [latchedModifiers, setLatchedModifiers] = useState<Set<string>>(new Set());

  // Default visibility: true on mobile (touch devices), false on desktop.
  const isVisible = visible ?? (typeof window !== "undefined" && "ontouchstart" in window);

  const handleKeyPress = useCallback(
    (key: string) => {
      if (MODIFIER_KEYS.has(key)) {
        setLatchedModifiers((prev) => {
          const next = new Set(prev);
          if (next.has(key)) {
            next.delete(key);
          } else {
            next.add(key);
          }
          return next;
        });
        return;
      }

      let sequence = keyToSequence(key);

      if (latchedModifiers.has("Ctrl") && sequence.length === 1) {
        const code = sequence.toLowerCase().charCodeAt(0);
        if (code >= 97 && code <= 122) {
          sequence = String.fromCharCode(code - 96);
        }
      }

      if (latchedModifiers.has("Alt") && sequence.length === 1) {
        sequence = "\x1b" + sequence;
      }

      termRef.current?.sendData(sequence);
      termRef.current?.focus();
      setLatchedModifiers(new Set());
    },
    [termRef, latchedModifiers],
  );

  if (!isVisible) return null;

  return (
    <div className="tm-keys" role="toolbar" aria-label={t("label")}>
      {keys.map((key) => {
        const isModifier = MODIFIER_KEYS.has(key);
        const isLatched = latchedModifiers.has(key);
        return (
          <button
            key={key}
            type="button"
            onClick={() => handleKeyPress(key)}
            className="tm-key"
            aria-pressed={isModifier ? isLatched : undefined}
            title={isModifier ? t("hold", { key }) : t("send", { key })}
          >
            {key}
          </button>
        );
      })}
    </div>
  );
}
