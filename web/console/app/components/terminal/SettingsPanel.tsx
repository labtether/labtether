"use client";

import { useCallback } from "react";
import { X } from "lucide-react";
import { terminalThemes } from "../../terminal/themes";
import { terminalFonts } from "../../terminal/fonts";
import type { TerminalPreferences } from "../../hooks/useTerminalPreferences";

interface SettingsPanelProps {
  open: boolean;
  onClose: () => void;
  prefs: TerminalPreferences;
  onUpdatePrefs: (updates: Partial<TerminalPreferences>) => void;
}

const SCROLLBACK_PRESETS = [1000, 5000, 10000, 50000];

const CURSOR_STYLES: Array<{ value: TerminalPreferences["cursor_style"]; label: string }> = [
  { value: "block", label: "Block" },
  { value: "underline", label: "Underline" },
  { value: "bar", label: "Bar" },
];

export default function SettingsPanel({
  open,
  onClose,
  prefs,
  onUpdatePrefs,
}: SettingsPanelProps) {
  const handleThemeSelect = useCallback(
    (themeId: string) => onUpdatePrefs({ theme: themeId }),
    [onUpdatePrefs],
  );

  const handleFontChange = useCallback(
    (fontId: string) => onUpdatePrefs({ font_family: fontId }),
    [onUpdatePrefs],
  );

  const handleFontSizeChange = useCallback(
    (size: number) => onUpdatePrefs({ font_size: size }),
    [onUpdatePrefs],
  );

  const handleCursorStyleChange = useCallback(
    (style: TerminalPreferences["cursor_style"]) => onUpdatePrefs({ cursor_style: style }),
    [onUpdatePrefs],
  );

  const handleCursorBlinkToggle = useCallback(
    () => onUpdatePrefs({ cursor_blink: !prefs.cursor_blink }),
    [onUpdatePrefs, prefs.cursor_blink],
  );

  const handleScrollbackChange = useCallback(
    (value: number) => onUpdatePrefs({ scrollback: value }),
    [onUpdatePrefs],
  );

  const handleAutoReconnectToggle = useCallback(
    () => onUpdatePrefs({ auto_reconnect: !prefs.auto_reconnect }),
    [onUpdatePrefs, prefs.auto_reconnect],
  );

  if (!open) return null;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 100,
        display: "flex",
        alignItems: "flex-start",
        justifyContent: "center",
        paddingTop: "8vh",
        backgroundColor: "rgba(0, 0, 0, 0.6)",
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          width: "100%",
          maxWidth: 480,
          maxHeight: "80vh",
          backgroundColor: "#1a1a1a",
          border: "1px solid #333",
          borderRadius: 10,
          boxShadow: "0 20px 60px rgba(0, 0, 0, 0.7)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          animation: "settingsPanelFadeIn 0.15s ease-out",
        }}
      >
        <style>{`
          @keyframes settingsPanelFadeIn {
            from { transform: translateY(-12px); opacity: 0; }
            to { transform: translateY(0); opacity: 1; }
          }
        `}</style>

        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "14px 16px",
            borderBottom: "1px solid #333",
            flexShrink: 0,
          }}
        >
          <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "#e0e0e0" }}>
            Terminal Settings
          </h2>
          <button
            type="button"
            onClick={onClose}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 24,
              height: 24,
              border: "none",
              background: "none",
              color: "#888",
              cursor: "pointer",
              borderRadius: 4,
            }}
            title="Close settings"
          >
            <X size={14} />
          </button>
        </div>

        {/* Scrollable content */}
        <div
          style={{
            flex: 1,
            overflowY: "auto",
            padding: "12px 16px 20px",
          }}
        >
          {/* Theme section */}
          <Section title="Theme">
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(2, 1fr)",
                gap: 8,
              }}
            >
              {terminalThemes.map((t) => {
                const isActive = prefs.theme === t.id;
                const th = t.theme;
                const swatchColors = [
                  th.background ?? "#000",
                  th.red ?? "#f00",
                  th.green ?? "#0f0",
                  th.blue ?? "#00f",
                  th.yellow ?? "#ff0",
                  th.magenta ?? "#f0f",
                ];
                return (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => handleThemeSelect(t.id)}
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 6,
                      padding: "8px 10px",
                      border: isActive ? "2px solid #58a6ff" : "1px solid #444",
                      borderRadius: 6,
                      backgroundColor: isActive ? "rgba(88, 166, 255, 0.08)" : "#222",
                      cursor: "pointer",
                      textAlign: "left",
                      transition: "border-color 0.15s, background-color 0.15s",
                    }}
                  >
                    {/* Color swatch strip */}
                    <div style={{ display: "flex", gap: 2, height: 12, borderRadius: 3, overflow: "hidden" }}>
                      {swatchColors.map((color, i) => (
                        <div
                          key={i}
                          style={{
                            flex: 1,
                            backgroundColor: color,
                          }}
                        />
                      ))}
                    </div>
                    <span
                      style={{
                        fontSize: 11,
                        fontWeight: isActive ? 600 : 400,
                        color: isActive ? "#e0e0e0" : "#aaa",
                      }}
                    >
                      {t.name}
                    </span>
                  </button>
                );
              })}
            </div>
          </Section>

          {/* Font section */}
          <Section title="Font">
            <select
              value={prefs.font_family}
              onChange={(e) => handleFontChange(e.target.value)}
              style={{
                width: "100%",
                padding: "6px 10px",
                fontSize: 13,
                backgroundColor: "#2a2a2a",
                color: "#e0e0e0",
                border: "1px solid #444",
                borderRadius: 6,
                outline: "none",
                cursor: "pointer",
              }}
            >
              {terminalFonts.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.name}
                </option>
              ))}
            </select>
          </Section>

          {/* Font Size section */}
          <Section title="Font Size">
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <input
                type="range"
                min={10}
                max={24}
                step={1}
                value={prefs.font_size}
                onChange={(e) => handleFontSizeChange(Number(e.target.value))}
                style={{
                  flex: 1,
                  height: 4,
                  accentColor: "#58a6ff",
                  cursor: "pointer",
                }}
              />
              <span
                style={{
                  fontSize: 13,
                  fontFamily: "'JetBrains Mono', monospace",
                  color: "#e0e0e0",
                  minWidth: 36,
                  textAlign: "right",
                }}
              >
                {prefs.font_size}px
              </span>
            </div>
          </Section>

          {/* Cursor section */}
          <Section title="Cursor">
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <div style={{ display: "flex", gap: 4, flex: 1 }}>
                {CURSOR_STYLES.map((cs) => (
                  <button
                    key={cs.value}
                    type="button"
                    onClick={() => handleCursorStyleChange(cs.value)}
                    style={{
                      flex: 1,
                      padding: "5px 0",
                      fontSize: 12,
                      border: prefs.cursor_style === cs.value ? "1px solid #58a6ff" : "1px solid #444",
                      borderRadius: 4,
                      backgroundColor: prefs.cursor_style === cs.value ? "rgba(88, 166, 255, 0.15)" : "#2a2a2a",
                      color: prefs.cursor_style === cs.value ? "#e0e0e0" : "#888",
                      cursor: "pointer",
                      transition: "border-color 0.15s, background-color 0.15s, color 0.15s",
                    }}
                  >
                    {cs.label}
                  </button>
                ))}
              </div>

              {/* Blink checkbox */}
              <label
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontSize: 12,
                  color: "#aaa",
                  cursor: "pointer",
                  userSelect: "none",
                  flexShrink: 0,
                }}
              >
                <input
                  type="checkbox"
                  checked={prefs.cursor_blink}
                  onChange={handleCursorBlinkToggle}
                  style={{ accentColor: "#58a6ff", cursor: "pointer" }}
                />
                Blink
              </label>
            </div>
          </Section>

          {/* Scrollback section */}
          <Section title="Scrollback">
            <div style={{ display: "flex", gap: 6 }}>
              {SCROLLBACK_PRESETS.map((val) => (
                <button
                  key={val}
                  type="button"
                  onClick={() => handleScrollbackChange(val)}
                  style={{
                    flex: 1,
                    padding: "5px 0",
                    fontSize: 12,
                    fontFamily: "'JetBrains Mono', monospace",
                    border: prefs.scrollback === val ? "1px solid #58a6ff" : "1px solid #444",
                    borderRadius: 4,
                    backgroundColor: prefs.scrollback === val ? "rgba(88, 166, 255, 0.15)" : "#2a2a2a",
                    color: prefs.scrollback === val ? "#e0e0e0" : "#888",
                    cursor: "pointer",
                    transition: "border-color 0.15s, background-color 0.15s, color 0.15s",
                  }}
                >
                  {val >= 1000 ? `${val / 1000}k` : val}
                </button>
              ))}
            </div>
          </Section>

          {/* Auto-Reconnect section */}
          <Section title="Auto-Reconnect" last>
            <label
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                cursor: "pointer",
                userSelect: "none",
              }}
            >
              <span style={{ fontSize: 12, color: "#aaa" }}>
                Automatically reconnect on disconnect
              </span>
              <ToggleSwitch
                checked={prefs.auto_reconnect}
                onChange={handleAutoReconnectToggle}
              />
            </label>
          </Section>
        </div>
      </div>
    </div>
  );
}

/* ---------- Helpers ---------- */

function Section({
  title,
  children,
  last,
}: {
  title: string;
  children: React.ReactNode;
  last?: boolean;
}) {
  return (
    <div
      style={{
        marginBottom: last ? 0 : 16,
        paddingBottom: last ? 0 : 16,
        borderBottom: last ? "none" : "1px solid #2a2a2a",
      }}
    >
      <h3
        style={{
          margin: "0 0 8px 0",
          fontSize: 11,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.6px",
          color: "#888",
        }}
      >
        {title}
      </h3>
      {children}
    </div>
  );
}

function ToggleSwitch({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={onChange}
      style={{
        position: "relative",
        width: 36,
        height: 20,
        borderRadius: 10,
        border: "none",
        backgroundColor: checked ? "#58a6ff" : "#444",
        cursor: "pointer",
        transition: "background-color 0.2s",
        flexShrink: 0,
        padding: 0,
      }}
    >
      <span
        style={{
          position: "absolute",
          top: 2,
          left: checked ? 18 : 2,
          width: 16,
          height: 16,
          borderRadius: "50%",
          backgroundColor: "#fff",
          transition: "left 0.2s",
          boxShadow: "0 1px 3px rgba(0,0,0,0.3)",
        }}
      />
    </button>
  );
}
