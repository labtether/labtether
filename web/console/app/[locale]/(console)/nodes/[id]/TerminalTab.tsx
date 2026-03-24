"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { SessionToolbar } from "../../../../components/SessionToolbar";
import { ErrorBoundary } from "../../../../components/ErrorBoundary";
import { SessionPanel } from "../../../../components/SessionPanel";
import { Card } from "../../../../components/ui/Card";
import { Input, Select } from "../../../../components/ui/Input";
import { useSession } from "../../../../hooks/useSession";
import { useTerminalPreferences } from "../../../../hooks/useTerminalPreferences";
import { useTerminalSnippets } from "../../../../hooks/useTerminalSnippets";
import { getThemeById } from "../../../../terminal/themes";
import { getFontById } from "../../../../terminal/fonts";
import type { XTerminalHandle } from "../../../../components/XTerminal";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../../hooks/useConnectedAgents";
import SearchBar from "../../../../components/terminal/SearchBar";
import SnippetPicker from "../../../../components/terminal/SnippetPicker";
import KeyboardToolbar from "../../../../components/terminal/KeyboardToolbar";
import SettingsPanel from "../../../../components/terminal/SettingsPanel";
import { Settings, Code2, Search } from "lucide-react";

function tmuxInstallCommand(asset: { metadata?: Record<string, string> } | null): string | null {
  if (!asset?.metadata) return "sudo apt install -y tmux";
  const osId = (asset.metadata.os_id ?? "").toLowerCase();
  const osIdLike = (asset.metadata.os_id_like ?? "").toLowerCase();
  const osName = (asset.metadata.os_pretty_name ?? asset.metadata.os_name ?? "").toLowerCase();

  if (osId === "debian" || osId === "ubuntu" || osId === "raspbian" || osIdLike.includes("debian") || osName.includes("debian") || osName.includes("ubuntu")) {
    return "sudo apt install -y tmux";
  }
  if (osId === "fedora" || osId === "rhel" || osId === "centos" || osId === "rocky" || osId === "alma" || osIdLike.includes("fedora") || osIdLike.includes("rhel")) {
    return "sudo dnf install -y tmux";
  }
  if (osId === "arch" || osId === "manjaro" || osIdLike.includes("arch")) {
    return "sudo pacman -S --noconfirm tmux";
  }
  if (osId === "opensuse" || osId === "sles" || osIdLike.includes("suse")) {
    return "sudo zypper install -y tmux";
  }
  if (osId === "alpine") {
    return "sudo apk add tmux";
  }
  if (osId === "freebsd") {
    return "sudo pkg install -y tmux";
  }
  if (osName.includes("macos") || osName.includes("darwin")) {
    return "brew install tmux";
  }
  return "sudo apt install -y tmux";
}

const DOCKER_SHELL_PRESET_KEY_PREFIX = "labtether.docker-terminal.shell-preset.";
const DOCKER_SHELL_CUSTOM_KEY_PREFIX = "labtether.docker-terminal.shell-custom.";

const SHELL_PRESETS: Array<{ value: string; label: string }> = [
  { value: "sh", label: "sh (Recommended)" },
  { value: "bash", label: "bash" },
  { value: "ash", label: "ash" },
  { value: "/bin/sh", label: "/bin/sh" },
  { value: "/bin/bash", label: "/bin/bash" },
  { value: "/bin/ash", label: "/bin/ash" },
  { value: "pwsh", label: "pwsh" },
  { value: "custom", label: "Custom command..." },
];

export function TerminalTab({ nodeId }: { nodeId: string }) {
  const status = useFastStatus();
  const { agentTmuxStatus } = useConnectedAgents();
  const { prefs, updatePrefs } = useTerminalPreferences();
  const session = useSession({ type: "terminal", fixedTarget: nodeId, autoReconnect: prefs.auto_reconnect });
  const termRef = useRef<XTerminalHandle>(null);
  const [shellPreset, setShellPreset] = useState("sh");
  const [customShell, setCustomShell] = useState("");
  const [tmuxHintDismissed, setTmuxHintDismissed] = useState(false);

  // Terminal preferences + snippets
  const { snippets } = useTerminalSnippets(nodeId);

  // UI state for overlays
  const [searchOpen, setSearchOpen] = useState(false);
  const [snippetPickerOpen, setSnippetPickerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  // Derived theme/font from preferences
  const themeDef = useMemo(() => getThemeById(prefs.theme), [prefs.theme]);
  const fontDef = useMemo(() => getFontById(prefs.font_family), [prefs.font_family]);

  const asset = useMemo(
    () => status?.assets?.find((entry) => entry.id === nodeId) ?? null,
    [nodeId, status?.assets],
  );
  const isDockerContainer = asset?.source === "docker" && asset?.type === "docker-container";
  const presetStorageKey = `${DOCKER_SHELL_PRESET_KEY_PREFIX}${nodeId}`;
  const customStorageKey = `${DOCKER_SHELL_CUSTOM_KEY_PREFIX}${nodeId}`;

  useEffect(() => {
    if (!isDockerContainer) return;
    try {
      const storedPreset = window.localStorage.getItem(presetStorageKey)?.trim() ?? "";
      const knownPreset = SHELL_PRESETS.some((entry) => entry.value === storedPreset);
      if (knownPreset) {
        setShellPreset(storedPreset);
      }
      const storedCustom = window.localStorage.getItem(customStorageKey)?.trim() ?? "";
      if (storedCustom) {
        setCustomShell(storedCustom);
      }
    } catch {
      // ignore local preference failures
    }
  }, [customStorageKey, isDockerContainer, presetStorageKey]);

  useEffect(() => {
    if (!isDockerContainer) return;
    try {
      window.localStorage.setItem(presetStorageKey, shellPreset);
      if (shellPreset === "custom") {
        if (customShell.trim()) {
          window.localStorage.setItem(customStorageKey, customShell.trim());
        } else {
          window.localStorage.removeItem(customStorageKey);
        }
      }
    } catch {
      // ignore local preference failures
    }
  }, [customShell, customStorageKey, isDockerContainer, presetStorageKey, shellPreset]);

  const terminalShell = shellPreset === "custom" ? customShell.trim() : shellPreset;

  const connectWithShell = () =>
    void session.connect(undefined, {
      terminalShell: isDockerContainer && terminalShell ? terminalShell : undefined,
    });

  // Snippet selection handler: send command to terminal
  const handleSnippetSelect = useCallback(
    (command: string) => {
      termRef.current?.sendData(command);
      termRef.current?.focus();
    },
    [],
  );

  // Global keyboard shortcuts for search (Ctrl+F) and snippets (Ctrl+;)
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // Ctrl+F: toggle search bar
      if (e.ctrlKey && e.key === "f") {
        e.preventDefault();
        setSearchOpen((v) => !v);
      }
      // Ctrl+;: toggle snippet picker
      if (e.ctrlKey && e.key === ";") {
        e.preventDefault();
        setSnippetPickerOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const hasTmux = agentTmuxStatus.get(nodeId);
  const showTmuxHint = hasTmux === false && !tmuxHintDismissed && !isDockerContainer;
  const installCmd = tmuxInstallCommand(asset);

  return (
    <>
      {showTmuxHint && (
        <Card className="mb-3 border-[var(--warn)]/20">
          <div className="flex items-start gap-3">
            <span className="text-[var(--warn)] text-sm shrink-0">&#x1F4E6;</span>
            <div className="flex-1 space-y-1">
              <p className="text-xs font-medium text-[var(--text)]">
                Session persistence unavailable — tmux not installed
              </p>
              <p className="text-xs text-[var(--muted)]">
                Without tmux, terminal sessions cannot survive disconnects. Environment variables, working directory, and running processes will be lost on reconnect.
              </p>
              {installCmd && (
                <div className="mt-1.5 flex items-center gap-2">
                  <code className="rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-0.5 text-xs text-[var(--text)]">
                    {installCmd}
                  </code>
                  <button
                    type="button"
                    className="rounded border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--surface)]"
                    onClick={() => {
                      void navigator.clipboard.writeText(installCmd);
                    }}
                  >
                    Copy
                  </button>
                </div>
              )}
            </div>
            <button
              type="button"
              className="text-[var(--muted)] hover:text-[var(--text)] text-xs"
              onClick={() => setTmuxHintDismissed(true)}
            >
              Dismiss
            </button>
          </div>
        </Card>
      )}
      {isDockerContainer && (
        <Card className="mb-3">
          <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wider text-[var(--muted)]">Container Shell</p>
              <p className="text-xs text-[var(--muted)]">Saved per container for future terminal sessions.</p>
            </div>
            <div className="flex w-full flex-col gap-2 md:w-auto md:min-w-[260px]">
              <Select
                value={shellPreset}
                onChange={(event) => setShellPreset(event.target.value)}
              >
                {SHELL_PRESETS.map((preset) => (
                  <option key={preset.value} value={preset.value}>
                    {preset.label}
                  </option>
                ))}
              </Select>
              {shellPreset === "custom" && (
                <Input
                  type="text"
                  placeholder="Custom command (for example: bash -lc)"
                  value={customShell}
                  onChange={(event) => setCustomShell(event.target.value)}
                />
              )}
            </div>
          </div>
        </Card>
      )}
      <SessionToolbar
        type="terminal"
        target={session.target}
        setTarget={session.setTarget}
        isFixedTarget
        assets={session.assets}
        connectedAgentIds={session.connectedAgentIds}
        connectionState={session.connectionState}
        activeSessionId={session.activeSessionId}
        isReconnecting={session.isReconnecting}
        onConnect={connectWithShell}
        onDisconnect={session.disconnect}
        compact
        extraActions={
          <div style={{ display: "flex", alignItems: "center", gap: 2 }}>
            {/* Search button */}
            <button
              type="button"
              onClick={() => setSearchOpen((v) => !v)}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                width: 24,
                height: 24,
                border: "none",
                background: searchOpen ? "#333" : "none",
                color: searchOpen ? "#e0e0e0" : "#888",
                cursor: "pointer",
                borderRadius: 4,
              }}
              title="Search terminal (Ctrl+F)"
            >
              <Search size={12} />
            </button>
            {/* Snippets button */}
            <button
              type="button"
              onClick={() => setSnippetPickerOpen((v) => !v)}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                width: 24,
                height: 24,
                border: "none",
                background: snippetPickerOpen ? "#333" : "none",
                color: snippetPickerOpen ? "#e0e0e0" : "#888",
                cursor: "pointer",
                borderRadius: 4,
              }}
              title="Snippets (Ctrl+;)"
            >
              <Code2 size={12} />
            </button>
            {/* Settings button */}
            <button
              type="button"
              onClick={() => setSettingsOpen((v) => !v)}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                width: 24,
                height: 24,
                border: "none",
                background: settingsOpen ? "#333" : "none",
                color: settingsOpen ? "#e0e0e0" : "#888",
                cursor: "pointer",
                borderRadius: 4,
              }}
              title="Terminal settings"
            >
              <Settings size={12} />
            </button>
          </div>
        }
      />

      {/* Terminal panel with search overlay */}
      <div style={{ position: "relative" }}>
        <SearchBar
          termRef={termRef}
          open={searchOpen}
          onClose={() => setSearchOpen(false)}
        />
        <ErrorBoundary>
          <SessionPanel
            type="terminal"
            connectionState={session.connectionState}
            wsUrl={session.wsUrl}
            error={session.error}
            target={session.target}
            connectedAgentIds={session.connectedAgentIds}
            onRetry={connectWithShell}
            termRef={termRef}
            onTerminalConnected={session.handleConnected}
            onTerminalDisconnected={session.handleDisconnected}
            onTerminalError={session.handleError}
            onTerminalStreamReady={session.handleStreamReady}
            onTerminalStreamStatus={session.handleStreamStatus}
            terminalTheme={themeDef.theme}
            terminalFontFamily={fontDef.family}
            terminalFontSize={prefs.font_size}
            terminalCursorStyle={prefs.cursor_style}
            terminalCursorBlink={prefs.cursor_blink}
            terminalScrollback={prefs.scrollback}
          />
        </ErrorBoundary>
      </div>

      {/* Keyboard toolbar */}
      <KeyboardToolbar
        termRef={termRef}
        keys={prefs.toolbar_keys ?? undefined}
      />

      {/* Snippet picker overlay */}
      <SnippetPicker
        snippets={snippets}
        open={snippetPickerOpen}
        onClose={() => setSnippetPickerOpen(false)}
        onSelect={handleSnippetSelect}
      />

      {/* Settings panel overlay */}
      <SettingsPanel
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        prefs={prefs}
        onUpdatePrefs={updatePrefs}
      />
    </>
  );
}
