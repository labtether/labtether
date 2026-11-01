// The 2.0 terminal colours for xterm. The saved theme "labtether-dark" (the default) shows the 2.0 palette; every
// other theme in Settings keeps its own colours. The canvas matches --term-bg: a shade darker on the light app theme.
import type { ITheme } from "@xterm/xterm";
import { getThemeById, type TerminalThemeDef } from "../../terminal/themes";

export const DEFAULT_TERMINAL_THEME_ID = "labtether-dark";

const PALETTE: ITheme = {
  foreground: "#e4e4e8",
  cursor: "#ff79c6",
  selectionBackground: "rgba(228, 228, 232, 0.22)",
  black: "#3a3a42",
  red: "#ff6b70",
  green: "#3dd68c",
  yellow: "#f5c451",
  blue: "#7aa2f7",
  magenta: "#ff79c6",
  cyan: "#5fd3e0",
  white: "#e4e4e8",
  brightBlack: "#8b8b94",
  brightRed: "#ff8a8e",
  brightGreen: "#62e3a4",
  brightYellow: "#ffd479",
  brightBlue: "#9ab8ff",
  brightMagenta: "#ff9ad6",
  brightCyan: "#86e3ee",
  brightWhite: "#ffffff",
};

/** The 2.0 palette on the canvas colour of the app theme ("light" or a dark theme). */
export function labtetherTerminalTheme(appTheme: string): ITheme {
  const background = appTheme === "light" ? "#0e0e10" : "#111114";
  return { ...PALETTE, background, cursorAccent: background };
}

/** The saved theme as xterm needs it: the 2.0 palette for the default, else the theme's own colours. */
export function resolveTerminalTheme(themeId: string, appTheme: string): TerminalThemeDef {
  const def = getThemeById(themeId);
  if (def.id !== DEFAULT_TERMINAL_THEME_ID) return def;
  return { ...def, theme: labtetherTerminalTheme(appTheme) };
}
