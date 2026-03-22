"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { Density, Theme } from "../console/models";
import { densityOptions, themeOptions } from "../console/models";

export type AccentOption = "neon-rose" | "deep-signal";

export const accentOptions = [
  { id: "neon-rose" as const, label: "Neon Rose" },
  { id: "deep-signal" as const, label: "Deep Signal" },
];

type ThemeContextValue = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  density: Density;
  setDensity: (density: Density) => void;
  accent: AccentOption;
  setAccent: (accent: AccentOption) => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function detectSystemTheme(): Theme {
  if (typeof window === "undefined") return "oled";
  return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "oled";
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  // Read localStorage synchronously in initializers so the first render already
  // has the correct values — prevents the accent/theme effects from clobbering
  // the blocking script's body attributes with wrong defaults.
  const [theme, setThemeState] = useState<Theme>(() => {
    if (typeof window === "undefined") return "oled";
    const stored = window.localStorage.getItem("labtether.theme") as Theme | null;
    if (stored && themeOptions.some((o) => o.id === stored)) return stored;
    return detectSystemTheme();
  });
  const [density, setDensityState] = useState<Density>(() => {
    if (typeof window === "undefined") return "minimal";
    const stored = window.localStorage.getItem("labtether.density") as Density | null;
    if (stored && densityOptions.some((o) => o.id === stored)) return stored;
    return "minimal";
  });
  const [accent, setAccentState] = useState<AccentOption>(() => {
    if (typeof window === "undefined") return "neon-rose";
    const stored = window.localStorage.getItem("lt-accent") as AccentOption | null;
    if (stored && accentOptions.some((o) => o.id === stored)) return stored;
    return "neon-rose";
  });
  const userChoseTheme = useRef(false);

  // On mount: mark whether the user explicitly chose a theme (for OS preference tracking)
  useEffect(() => {
    const storedTheme = window.localStorage.getItem("labtether.theme") as Theme | null;
    if (storedTheme && themeOptions.some((option) => option.id === storedTheme)) {
      userChoseTheme.current = true;
    }
  }, []);

  // Listen for OS theme changes — only follow when user hasn't explicitly chosen
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const handler = (e: MediaQueryListEvent) => {
      if (!userChoseTheme.current) {
        setThemeState(e.matches ? "light" : "oled");
      }
    };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  // Apply theme to DOM; only persist to localStorage if user explicitly chose
  useEffect(() => {
    document.body.dataset.theme = theme;
    if (userChoseTheme.current) {
      window.localStorage.setItem("labtether.theme", theme);
    }
  }, [theme]);

  useEffect(() => {
    document.body.dataset.density = density;
    window.localStorage.setItem("labtether.density", density);
  }, [density]);

  useEffect(() => {
    if (accent === "neon-rose") {
      delete document.body.dataset.accent;
    } else {
      document.body.dataset.accent = accent;
    }
    window.localStorage.setItem("lt-accent", accent);
  }, [accent]);

  const setTheme = useCallback((value: Theme) => {
    userChoseTheme.current = true;
    window.localStorage.setItem("labtether.theme", value);
    setThemeState(value);
  }, []);
  const setDensity = useCallback((value: Density) => setDensityState(value), []);
  const setAccent = useCallback((value: AccentOption) => setAccentState(value), []);

  const value = useMemo(
    () => ({ theme, setTheme, density, setDensity, accent, setAccent }),
    [theme, setTheme, density, setDensity, accent, setAccent]
  );

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
}
