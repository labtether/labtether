"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Link, usePathname } from "../../i18n/navigation";
import { Menu, X } from "lucide-react";
import { navGroups } from "../lib/navigation";
import { useAuth } from "../contexts/AuthContext";
import { meetsMinimumRole } from "../lib/roles";

const MOBILE_NAV_DRAWER_ID = "mobile-navigation-drawer";
const MOBILE_NAV_TOGGLE_ID = "mobile-navigation-toggle";

export function MobileNavToggle({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  return (
    <button
      id={MOBILE_NAV_TOGGLE_ID}
      className="fixed top-3 right-3 z-40 p-2 rounded-lg bg-[var(--panel)] border border-[var(--line)] md:hidden cursor-pointer"
      onClick={onToggle}
      aria-label="Toggle navigation"
      aria-controls={MOBILE_NAV_DRAWER_ID}
      aria-expanded={open}
    >
      <Menu className="w-4 h-4" strokeWidth={1.5} />
    </button>
  );
}

export function MobileNavOverlay({ open, onClose }: { open: boolean; onClose: () => void }) {
  const pathname = usePathname();
  const { user } = useAuth();
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

  return (
    <div
      className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm md:hidden"
      onClick={onClose}
    >
      <div
        id={MOBILE_NAV_DRAWER_ID}
        role="dialog"
        aria-modal="true"
        aria-label="Navigation menu"
        className="absolute top-0 right-0 bottom-0 w-64 bg-[rgba(255,255,255,0.02)] border-l border-[var(--line)] p-5 space-y-6"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span
              className="flex items-center justify-center w-7 h-7 rounded-lg bg-[var(--accent)] text-white text-xs font-medium font-mono"
              style={{ boxShadow: "0 0 12px var(--accent-glow)" }}
            >
              LT
            </span>
            <span className="text-sm font-medium text-[var(--text)]">LabTether</span>
          </div>
          <button
            ref={closeButtonRef}
            className="p-1 rounded-lg hover:bg-[var(--hover)] cursor-pointer"
            onClick={onClose}
            aria-label="Close navigation"
          >
            <X className="w-4 h-4" strokeWidth={1.5} />
          </button>
        </div>

        {/* Nav links */}
        <nav className="space-y-4" aria-label="Primary navigation">
          {navGroups.map((group) => (
            <div key={group.category}>
              <span className="block mb-1.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--muted)]">
                {group.category}
              </span>
              <div className="space-y-0.5">
                {group.items
                  .filter((item) => meetsMinimumRole(user?.role, item.minimumRole))
                  .map((item) => {
                  const Icon = item.icon;
                  const isActive = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      aria-current={isActive ? "page" : undefined}
                      className={`group flex items-center gap-2.5 px-2 h-[34px] rounded-r-lg text-sm transition-colors duration-[var(--dur-instant)] ${
                        isActive
                          ? "border-l-2 border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--text)] font-medium"
                          : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--hover)]"
                      }`}
                      onClick={onClose}
                    >
                      <Icon
                        className={`w-4 h-4 transition-opacity duration-[var(--dur-instant)] ${
                          isActive
                            ? "text-[var(--accent-text)] opacity-100"
                            : "opacity-50 group-hover:opacity-90"
                        }`}
                        strokeWidth={1.5}
                      />
                      <span>{item.label}</span>
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
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
