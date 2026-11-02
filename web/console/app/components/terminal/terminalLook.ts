// The 2.0 terminal colours for xterm. The saved theme "labtether-dark" (the default, in terminal/themes.ts) carries
// the 2.0 palette; its canvas follows --term-bg, a shade darker on the light app theme. Every other theme in Settings
// keeps its own colours.
import type { ITheme } from "@xterm/xterm";
import { getThemeById, type TerminalThemeDef } from "../../terminal/themes";

export const DEFAULT_TERMINAL_THEME_ID = "labtether-dark";

/** The 2.0 palette on the canvas colour of the app theme ("light" or a dark theme). */
export function labtetherTerminalTheme(appTheme: string): ITheme {
  const background = appTheme === "light" ? "#0e0e10" : "#111114";
  return { ...getThemeById(DEFAULT_TERMINAL_THEME_ID).theme, background, cursorAccent: background };
}

/** The saved theme as xterm needs it: the 2.0 palette for the default, else the theme's own colours. */
export function resolveTerminalTheme(themeId: string, appTheme: string): TerminalThemeDef {
  const def = getThemeById(themeId);
  if (def.id !== DEFAULT_TERMINAL_THEME_ID) return def;
  return { ...def, theme: labtetherTerminalTheme(appTheme) };
}
