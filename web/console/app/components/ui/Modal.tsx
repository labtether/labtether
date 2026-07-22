"use client";

import { type ReactNode, type CSSProperties, useEffect, useLayoutEffect, useRef } from "react";
import * as Dialog from "@radix-ui/react-dialog";

type ModalProps = {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  title?: string;
  ariaTitle?: string;
  description?: string;
};

const glassStyle: CSSProperties = {
  backdropFilter: "blur(40px)",
  WebkitBackdropFilter: "blur(40px)",
  boxShadow: "var(--shadow-panel)",
  borderRadius: "var(--radius-lg)",
};

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

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay
          className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-[modal-overlay-in_200ms_ease-out] data-[state=closed]:animate-[modal-overlay-out_150ms_ease-in]"
        />
        <Dialog.Content
          onOpenAutoFocus={() => {
            if (!returnFocusRef.current?.isConnected && document.activeElement instanceof HTMLElement) {
              returnFocusRef.current = document.activeElement;
            }
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            const returnTarget = returnFocusRef.current;
            returnFocusRef.current = null;
            if (!returnTarget?.isConnected) return;
            returnTarget.focus({ preventScroll: true });
          }}
          className={`fixed inset-x-3 top-4 z-50 max-h-[calc(100vh-2rem)] overflow-hidden border border-[var(--panel-border)] bg-[var(--panel-glass)] focus:outline-none data-[state=open]:animate-[modal-content-in_200ms_ease-out] data-[state=closed]:animate-[modal-content-out_150ms_ease-in] sm:inset-x-4 md:left-1/2 md:right-auto md:w-full md:max-w-lg md:-translate-x-1/2 ${className}`}
          style={glassStyle}
        >
          {title && (
            <Dialog.Title className="px-5 py-4 text-sm font-medium text-[var(--text)] border-b border-[var(--panel-border)]">
              {title}
            </Dialog.Title>
          )}
          {!title && <Dialog.Title className="sr-only">{ariaTitle ?? "Dialog"}</Dialog.Title>}
          <Dialog.Description className="sr-only">
            {description ?? "Dialog content"}
          </Dialog.Description>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
