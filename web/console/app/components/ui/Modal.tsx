"use client";

import { type ReactNode, type CSSProperties } from "react";
import * as Dialog from "@radix-ui/react-dialog";

type ModalProps = {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  title?: string;
};

const glassStyle: CSSProperties = {
  backdropFilter: "blur(40px)",
  WebkitBackdropFilter: "blur(40px)",
  boxShadow: "var(--shadow-panel)",
  borderRadius: "var(--radius-lg)",
};

export function Modal({ open, onClose, children, className = "", title }: ModalProps) {
  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay
          className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm data-[state=open]:animate-[modal-overlay-in_200ms_ease-out] data-[state=closed]:animate-[modal-overlay-out_150ms_ease-in]"
        />
        <Dialog.Content
          className={`fixed inset-x-3 top-4 z-50 max-h-[calc(100vh-2rem)] overflow-hidden border border-[var(--panel-border)] bg-[var(--panel-glass)] focus:outline-none data-[state=open]:animate-[modal-content-in_200ms_ease-out] data-[state=closed]:animate-[modal-content-out_150ms_ease-in] sm:inset-x-4 md:left-1/2 md:right-auto md:w-full md:max-w-lg md:-translate-x-1/2 ${className}`}
          style={glassStyle}
        >
          {title && (
            <Dialog.Title className="px-5 py-4 text-sm font-medium text-[var(--text)] border-b border-[var(--panel-border)]">
              {title}
            </Dialog.Title>
          )}
          {!title && <Dialog.Title className="sr-only">Dialog</Dialog.Title>}
          <Dialog.Description className="sr-only">Dialog content</Dialog.Description>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
