"use client";

import { type ReactNode, useEffect, useLayoutEffect, useRef } from "react";
import * as Dialog from "@radix-ui/react-dialog";

const FIRST_FIELD = [
  'input:not([type="hidden"]):not([type="checkbox"]):not([type="radio"]):not([disabled]):not([readonly])',
  "textarea:not([disabled]):not([readonly])",
  "select:not([disabled])",
].join(", ");

type ModalProps = {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  title?: string;
  ariaTitle?: string;
  description?: string;
};

/**
 * 2.0 dialog (kit.css .kt-dialog): a scrim, then a raised panel with a 16/24 title. Escape and a click on the scrim
 * close it. Focus starts in the first field, stays inside while it is open and returns to the control that opened
 * it. The scrim holds the panel and scrolls when the panel is taller than the window; on a phone the panel sits at
 * the bottom.
 */
export function Modal({
  open,
  onClose,
  children,
  className = "",
  title,
  ariaTitle,
  description,
}: ModalProps) {
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const contentRef = useRef<HTMLDivElement | null>(null);
  const wasOpenRef = useRef(open);

  useEffect(() => {
    if (open) return;

    const recordFocus = (event: FocusEvent) => {
      if (event.target instanceof HTMLElement && event.target !== document.body) {
        returnFocusRef.current = event.target;
      }
    };
    const activeElement = document.activeElement;
    if (activeElement instanceof HTMLElement && activeElement !== document.body) {
      returnFocusRef.current = activeElement;
    }
    document.addEventListener("focusin", recordFocus);
    return () => document.removeEventListener("focusin", recordFocus);
  }, [open]);

  useLayoutEffect(() => {
    const wasOpen = wasOpenRef.current;
    wasOpenRef.current = open;
    if (!wasOpen || open) return;

    const returnTarget = returnFocusRef.current;
    returnFocusRef.current = null;
    if (!returnTarget?.isConnected) return;

    const timer = window.setTimeout(() => {
      if (returnTarget.isConnected) {
        returnTarget.focus({ preventScroll: true });
      }
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open]);

  // Without a description, drop Radix's aria-describedby rather than announce filler text.
  const describedBy = description ? {} : { "aria-describedby": undefined };

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="kt-scrim">
          <Dialog.Content
            {...describedBy}
            ref={contentRef}
            onOpenAutoFocus={(event) => {
              if (!returnFocusRef.current?.isConnected && document.activeElement instanceof HTMLElement) {
                returnFocusRef.current = document.activeElement;
              }
              // Start in the first field, so a form dialog takes typing at once; otherwise Radix picks the first control.
              const field = contentRef.current?.querySelector<HTMLElement>(FIRST_FIELD);
              if (field) {
                event.preventDefault();
                field.focus({ preventScroll: true });
              }
            }}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              const returnTarget = returnFocusRef.current;
              returnFocusRef.current = null;
              if (!returnTarget?.isConnected) return;
              returnTarget.focus({ preventScroll: true });
            }}
            className={`kt-dialog${className ? ` ${className}` : ""}`}
          >
            {title ? (
              <Dialog.Title className="kt-dlg-title">{title}</Dialog.Title>
            ) : (
              <Dialog.Title className="sr-only">{ariaTitle ?? "Dialog"}</Dialog.Title>
            )}
            {description ? <Dialog.Description className="sr-only">{description}</Dialog.Description> : null}
            {children}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
