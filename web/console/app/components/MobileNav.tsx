"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Menu, X } from "lucide-react";
import { SidebarNav } from "./shell/SidebarNav";

const MOBILE_NAV_DRAWER_ID = "mobile-navigation-drawer";
const MOBILE_NAV_TOGGLE_ID = "mobile-navigation-toggle";

/** The top bar's menu button; shown under 900 px, where the sidebar folds into a drawer. */
export function MobileNavToggle({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const t = useTranslations("nav");
  return (
    <button
      id={MOBILE_NAV_TOGGLE_ID}
      type="button"
      className="lt-icon-btn lt-menu-btn"
      onClick={onToggle}
      aria-label={t("openNav")}
      aria-controls={MOBILE_NAV_DRAWER_ID}
      aria-expanded={open}
    >
      <Menu aria-hidden="true" strokeWidth={2} />
    </button>
  );
}

/** The sidebar as a drawer from the left. Escape or the scrim closes it, and focus goes back to the menu button. */
export function MobileNavOverlay({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useTranslations("nav");
  const closeButtonRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (!open) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      onClose();
    };

    document.addEventListener("keydown", handleKeyDown);
    closeButtonRef.current?.focus({ preventScroll: true });

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.getElementById(MOBILE_NAV_TOGGLE_ID)?.focus({ preventScroll: true });
    };
  }, [onClose, open]);

  if (!open) return null;

  const close = (
    <button ref={closeButtonRef} type="button" className="lt-icon-btn lt-drawer-x" onClick={onClose} aria-label={t("closeNav")}>
      <X aria-hidden="true" strokeWidth={2} />
    </button>
  );

  return (
    <div className="lt-drawer-scrim" onClick={onClose}>
      <div
        id={MOBILE_NAV_DRAWER_ID}
        role="dialog"
        aria-modal="true"
        aria-label={t("navMenu")}
        className="lt-drawer"
        onClick={(event) => event.stopPropagation()}
      >
        <nav aria-label={t("primaryNav")} className="lt-drawer-nav">
          <SidebarNav onNavigate={onClose} close={close} />
        </nav>
      </div>
    </div>
  );
}

export function useMobileNav() {
  const [open, setOpen] = useState(false);
  const toggle = useCallback(() => setOpen((prev) => !prev), []);
  const close = useCallback(() => setOpen(false), []);
  return { open, toggle, close };
}
