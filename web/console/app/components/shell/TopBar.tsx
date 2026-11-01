"use client";

import { Moon, Search, Sun } from "lucide-react";
import { useTranslations } from "next-intl";
import { usePalette } from "../../contexts/PaletteContext";
import { useTheme } from "../../contexts/ThemeContext";
import { MobileNavToggle } from "../MobileNav";

/**
 * Above every page: the search button (opens ⌘K) and the theme switch. Under 900 px the menu button and the mascot
 * join it, since the sidebar has folded away.
 */
export function TopBar({ navOpen, onToggleNav }: { navOpen: boolean; onToggleNav: () => void }) {
  const t = useTranslations("nav");
  const { setOpen } = usePalette();
  const { theme, setTheme } = useTheme();
  const light = theme === "light";

  return (
    <div className="lt-bar">
      <MobileNavToggle open={navOpen} onToggle={onToggleNav} />
      <img src="/logo.svg" alt="" width={24} height={24} aria-hidden="true" className="lt-bar-logo" />
      <button type="button" className="lt-search" onClick={() => setOpen(true)}>
        <Search aria-hidden="true" strokeWidth={2} />
        <span>{t("search")}</span>
        <kbd>⌘K</kbd>
      </button>
      <span className="lt-sp" />
      <button
        type="button"
        className="lt-icon-btn"
        onClick={() => setTheme(light ? "oled" : "light")}
        aria-label={t(light ? "darkTheme" : "lightTheme")}
      >
        {light ? <Moon aria-hidden="true" strokeWidth={2} /> : <Sun aria-hidden="true" strokeWidth={2} />}
      </button>
    </div>
  );
}
