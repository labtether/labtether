"use client";

import { useFormatter, useTranslations } from "next-intl";
import { terminalFonts } from "../../terminal/fonts";
import { terminalThemes } from "../../terminal/themes";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";
import { useTheme } from "../../contexts/ThemeContext";
import { Button } from "../ui/Button";
import { Select } from "../ui/Input";
import { Modal } from "../ui/Modal";
import { Switch } from "../ui/Switch";
import { DEFAULT_TERMINAL_THEME_ID, resolveTerminalTheme } from "./terminalLook";

interface SettingsPanelProps {
  open: boolean;
  onClose: () => void;
  prefs: TerminalPreferences;
  onUpdatePrefs: (updates: Partial<TerminalPreferences>) => void;
}

const SCROLLBACK_PRESETS = [1000, 5000, 10000, 50000];
const CURSOR_STYLES: Array<TerminalPreferences["cursor_style"]> = ["block", "underline", "bar"];
const MIN_FONT = 10;
const MAX_FONT = 24;

/** Terminal settings, saved on the hub for this account: colours, font, cursor, scrollback and reconnecting. */
export default function SettingsPanel({ open, onClose, prefs, onUpdatePrefs }: SettingsPanelProps) {
  const t = useTranslations("terminal.settings");
  const format = useFormatter();
  const { theme: appTheme } = useTheme();
  const size = Math.max(MIN_FONT, Math.min(MAX_FONT, prefs.font_size));

  return (
    <Modal open={open} onClose={onClose} title={t("title")}>
      <div className="kt-dlg-body tm-set">
        <div className="tm-set-row tm-set-top">
          <span className="kt-label" id="tm-set-theme">{t("colours")}</span>
          <div className="tm-themes" role="radiogroup" aria-labelledby="tm-set-theme">
            {terminalThemes.map((def) => {
              const on = prefs.theme === def.id;
              const colours = resolveTerminalTheme(def.id, appTheme).theme;
              return (
                <button
                  key={def.id}
                  type="button"
                  role="radio"
                  aria-checked={on}
                  className={`tm-theme${on ? " on" : ""}`}
                  onClick={() => onUpdatePrefs({ theme: def.id })}
                >
                  <span className="tm-swatch" aria-hidden="true" style={{ background: colours.background, color: colours.foreground }}>
                    <span style={{ color: colours.green }}>$</span> ls
                  </span>
                  <span className="tm-theme-n">{def.id === DEFAULT_TERMINAL_THEME_ID ? t("labtether") : def.name}</span>
                </button>
              );
            })}
          </div>
        </div>
        <div className="tm-set-row">
          <label className="kt-label" htmlFor="tm-set-font">{t("font")}</label>
          <Select id="tm-set-font" value={prefs.font_family} onChange={(event) => onUpdatePrefs({ font_family: event.target.value })}>
            {terminalFonts.map((font) => (
              <option key={font.id} value={font.id}>{font.name}</option>
            ))}
          </Select>
        </div>
        <div className="tm-set-row">
          <span className="kt-label" id="tm-set-size">{t("fontSize")}</span>
          <div className="tm-step" role="group" aria-labelledby="tm-set-size">
            <Button size="sm" aria-label={t("smaller")} disabled={size <= MIN_FONT} onClick={() => onUpdatePrefs({ font_size: size - 1 })}>−</Button>
            <span className="tm-step-v">{t("px", { size })}</span>
            <Button size="sm" aria-label={t("larger")} disabled={size >= MAX_FONT} onClick={() => onUpdatePrefs({ font_size: size + 1 })}>+</Button>
          </div>
        </div>
        <div className="tm-set-row">
          <span className="kt-label" id="tm-set-cursor">{t("cursor")}</span>
          <div className="tm-set-pair">
            <div className="kt-seg sm" role="group" aria-labelledby="tm-set-cursor">
              {CURSOR_STYLES.map((style) => (
                <button key={style} type="button" aria-pressed={prefs.cursor_style === style} onClick={() => onUpdatePrefs({ cursor_style: style })}>
                  {t(`cursor_${style}`)}
                </button>
              ))}
            </div>
            <label className="tm-cb">
              <input type="checkbox" checked={prefs.cursor_blink} onChange={() => onUpdatePrefs({ cursor_blink: !prefs.cursor_blink })} />
              {t("blink")}
            </label>
          </div>
        </div>
        <div className="tm-set-row">
          <span className="kt-label" id="tm-set-scroll">{t("scrollback")}</span>
          <div className="kt-seg sm" role="group" aria-labelledby="tm-set-scroll">
            {SCROLLBACK_PRESETS.map((lines) => (
              <button key={lines} type="button" aria-pressed={prefs.scrollback === lines} onClick={() => onUpdatePrefs({ scrollback: lines })}>
                {format.number(lines, { notation: "compact" })}
              </button>
            ))}
          </div>
        </div>
        <div className="tm-set-row">
          <label className="kt-label" htmlFor="tm-set-reconnect">{t("reconnect")}</label>
          <Switch id="tm-set-reconnect" checked={prefs.auto_reconnect} onChange={(next) => onUpdatePrefs({ auto_reconnect: next })} />
        </div>
        <div className="kt-dlg-acts">
          <Button variant="secondary" onClick={onClose}>{t("done")}</Button>
        </div>
      </div>
    </Modal>
  );
}
