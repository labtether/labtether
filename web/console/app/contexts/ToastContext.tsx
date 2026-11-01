"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

type ToastType = "success" | "error" | "info" | "warning";

export type ToastActionOptions = {
  label: string;
  onClick: () => void;
};

type Toast = {
  id: string;
  type: ToastType;
  message: string;
  action?: ToastActionOptions;
  secondaryAction?: ToastActionOptions;
};

type ToastContextValue = {
  toasts: Toast[];
  addToast: (type: ToastType, message: string, durationMs?: number, action?: ToastActionOptions, secondaryAction?: ToastActionOptions) => void;
  removeToast: (id: string) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextIdRef = useRef(0);
  const timersRef = useRef<Map<string, number>>(new Map());

  useEffect(() => {
    const timers = timersRef.current;
    return () => {
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    };
  }, []);

  const removeToast = useCallback((id: string) => {
    const timer = timersRef.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timersRef.current.delete(id);
    }
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const addToast = useCallback(
    (type: ToastType, message: string, durationMs = 4000, action?: ToastActionOptions, secondaryAction?: ToastActionOptions) => {
      const id = `toast-${++nextIdRef.current}`;
      setToasts((prev) => [...prev, { id, type, message, action, secondaryAction }]);
      if (durationMs > 0) {
        const timer = window.setTimeout(() => {
          timersRef.current.delete(id);
          removeToast(id);
        }, durationMs);
        timersRef.current.set(id, timer);
      }
    },
    [removeToast]
  );

  const value = useMemo(() => ({ toasts, addToast, removeToast }), [toasts, addToast, removeToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <ToastContainer toasts={toasts} onDismiss={removeToast} />
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
}

/** 2.0 toasts: a surface card bottom right (full width on a phone), an icon in the tone colour, the message, its
 *  action, and a quiet dismiss. Failures are announced assertively; everything else politely. */
function ToastContainer({ toasts, onDismiss }: { toasts: Toast[]; onDismiss: (id: string) => void }) {
  if (toasts.length === 0) return null;

  return (
    <div className="kt-toasts">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`kt-toast toast-${toast.type}`}
          role={toast.type === "error" ? "alert" : "status"}
          aria-live={toast.type === "error" ? "assertive" : "polite"}
          aria-atomic="true"
        >
          {toastIcon(toast.type)}
          <span className="kt-toast-msg">{toast.message}</span>
          {toast.action ? (
            <button
              type="button"
              className="kt-toast-act"
              onClick={() => {
                toast.action?.onClick();
                onDismiss(toast.id);
              }}
            >
              {toast.action.label}
            </button>
          ) : null}
          {toast.secondaryAction ? (
            <button
              type="button"
              className="kt-toast-act alt"
              onClick={() => {
                toast.secondaryAction?.onClick();
                onDismiss(toast.id);
              }}
            >
              {toast.secondaryAction.label}
            </button>
          ) : null}
          <button type="button" className="kt-toast-x" onClick={() => onDismiss(toast.id)} aria-label="Dismiss">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </button>
        </div>
      ))}
    </div>
  );
}

const ICON_PROPS = {
  width: 14,
  height: 14,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 2.2,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
} as const;

function toastIcon(type: ToastType): ReactNode {
  switch (type) {
    case "success":
      return (
        <svg {...ICON_PROPS}>
          <path d="m5 12 5 5 9-10" />
        </svg>
      );
    case "error":
      return (
        <svg {...ICON_PROPS}>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 8v4.5M12 16h.01" />
        </svg>
      );
    case "warning":
      return (
        <svg {...ICON_PROPS}>
          <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
          <path d="M12 9v4M12 17h.01" />
        </svg>
      );
    case "info":
      return (
        <svg {...ICON_PROPS}>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 16v-4.5M12 8h.01" />
        </svg>
      );
  }
}
