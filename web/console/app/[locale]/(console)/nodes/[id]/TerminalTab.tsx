"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { ErrorBoundary } from "../../../../components/ErrorBoundary";
import { SessionPanel } from "../../../../components/SessionPanel";
import { SessionToolbar } from "../../../../components/SessionToolbar";
import KeyboardToolbar from "../../../../components/terminal/KeyboardToolbar";
import SearchBar from "../../../../components/terminal/SearchBar";
import SettingsPanel from "../../../../components/terminal/SettingsPanel";
import SnippetPicker from "../../../../components/terminal/SnippetPicker";
import { shortcut } from "../../../../components/terminal/TerminalMoreMenu";
import { resolveTerminalTheme } from "../../../../components/terminal/terminalLook";
import { Select } from "../../../../components/ui/Input";
import type { XTerminalHandle } from "../../../../components/XTerminal";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useTheme } from "../../../../contexts/ThemeContext";
import { useConnectedAgents } from "../../../../hooks/useConnectedAgents";
import { useSession } from "../../../../hooks/useSession";
import { useTerminalPreferences } from "../../../../hooks/useTerminalPreferences";
import { useTerminalSnippets } from "../../../../hooks/useTerminalSnippets";
import { getFontById } from "../../../../terminal/fonts";
import { copyText } from "../../../../lib/clipboard";

/** The package manager command that installs tmux on the device's OS. */
function tmuxInstallCommand(asset: { metadata?: Record<string, string> } | null): string {
  const osId = (asset?.metadata?.os_id ?? "").toLowerCase();
  const like = (asset?.metadata?.os_id_like ?? "").toLowerCase();
  const name = (asset?.metadata?.os_pretty_name ?? asset?.metadata?.os_name ?? "").toLowerCase();
  if (["fedora", "rhel", "centos", "rocky", "alma"].includes(osId) || like.includes("fedora") || like.includes("rhel")) return "sudo dnf install -y tmux";
  if (["arch", "manjaro"].includes(osId) || like.includes("arch")) return "sudo pacman -S --noconfirm tmux";
  if (["opensuse", "sles"].includes(osId) || like.includes("suse")) return "sudo zypper install -y tmux";
  if (osId === "alpine") return "sudo apk add tmux";
  if (osId === "freebsd") return "sudo pkg install -y tmux";
  if (name.includes("macos") || name.includes("darwin")) return "brew install tmux";
  return "sudo apt install -y tmux";
}

const DOCKER_SHELL_PRESET_KEY_PREFIX = "labtether.docker-terminal.shell-preset.";
const DOCKER_SHELL_CUSTOM_KEY_PREFIX = "labtether.docker-terminal.shell-custom.";
const SHELL_PRESETS = ["sh", "bash", "ash", "/bin/sh", "/bin/bash", "/bin/ash", "pwsh", "custom"];

/** A device's Terminal tab: one shell on this device, its status bar and ⋯ menu, in the 2.0 terminal colours. */
export function TerminalTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("terminal.device");
  const status = useFastStatus();
  const { theme: appTheme } = useTheme();
  const { agentTmuxStatus } = useConnectedAgents();
  const { prefs, updatePrefs } = useTerminalPreferences();
  const session = useSession({ type: "terminal", fixedTarget: nodeId, autoReconnect: prefs.auto_reconnect });
  const termRef = useRef<XTerminalHandle>(null);
  const [shellPreset, setShellPreset] = useState("sh");
  const [customShell, setCustomShell] = useState("");
  const [tmuxHintDismissed, setTmuxHintDismissed] = useState(false);
  const [copied, setCopied] = useState(false);
  const { snippets } = useTerminalSnippets(nodeId);
  const [searchOpen, setSearchOpen] = useState(false);
  const [snippetPickerOpen, setSnippetPickerOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const themeDef = useMemo(() => resolveTerminalTheme(prefs.theme, appTheme), [prefs.theme, appTheme]);
  const fontDef = useMemo(() => getFontById(prefs.font_family), [prefs.font_family]);
  const asset = useMemo(() => status?.assets?.find((entry) => entry.id === nodeId) ?? null, [nodeId, status?.assets]);
  const isDockerContainer = asset?.source === "docker" && asset?.type === "docker-container";
  const presetStorageKey = `${DOCKER_SHELL_PRESET_KEY_PREFIX}${nodeId}`;
  const customStorageKey = `${DOCKER_SHELL_CUSTOM_KEY_PREFIX}${nodeId}`;

  useEffect(() => {
    if (!isDockerContainer) return;
    try {
      const storedPreset = window.localStorage.getItem(presetStorageKey)?.trim() ?? "";
      if (SHELL_PRESETS.includes(storedPreset)) setShellPreset(storedPreset);
      const storedCustom = window.localStorage.getItem(customStorageKey)?.trim() ?? "";
      if (storedCustom) setCustomShell(storedCustom);
    } catch {
      // Storage blocked: the default shell stays.
    }
  }, [customStorageKey, isDockerContainer, presetStorageKey]);

  useEffect(() => {
    if (!isDockerContainer) return;
    try {
      window.localStorage.setItem(presetStorageKey, shellPreset);
      if (shellPreset === "custom") {
        if (customShell.trim()) window.localStorage.setItem(customStorageKey, customShell.trim());
        else window.localStorage.removeItem(customStorageKey);
      }
    } catch {
      // Storage blocked: the choice lasts until the page reloads.
    }
  }, [customShell, customStorageKey, isDockerContainer, presetStorageKey, shellPreset]);

  const terminalShell = shellPreset === "custom" ? customShell.trim() : shellPreset;
  const connectWithShell = () =>
    void session.connect(undefined, { terminalShell: isDockerContainer && terminalShell ? terminalShell : undefined });

  const handleSnippetSelect = useCallback((command: string) => {
    termRef.current?.sendData(command);
    termRef.current?.focus();
  }, []);

  // Ctrl+F: find in the output. Ctrl+;: snippets.
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.ctrlKey && !event.shiftKey && event.key === "f") {
        event.preventDefault();
        setSearchOpen((open) => !open);
      }
      if (event.ctrlKey && event.key === ";") {
        event.preventDefault();
        setSnippetPickerOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const name = asset?.name || nodeId;
  const showTmuxHint = agentTmuxStatus.get(nodeId) === false && !tmuxHintDismissed && !isDockerContainer;
  const installCmd = tmuxInstallCommand(asset);

  return (
    <div className="tm-devtab">
      {showTmuxHint ? (
        <div className="tm-note">
          <span>{t("noTmux", { name })}</span>
          <code>{installCmd}</code>
          <button
            type="button"
            className="tm-qbtn"
            onClick={() => {
              void copyText(installCmd).then(() => setCopied(true), () => setCopied(false));
            }}
          >
            {copied ? t("copied") : t("copy")}
          </button>
          <button type="button" className="tm-qbtn" onClick={() => setTmuxHintDismissed(true)}>{t("dismiss")}</button>
        </div>
      ) : null}
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
          <>
            {isDockerContainer ? (
              <span className="tm-shell">
                <Select aria-label={t("shell")} value={shellPreset} onChange={(event) => setShellPreset(event.target.value)}>
                  {SHELL_PRESETS.map((preset) => (
                    <option key={preset} value={preset}>{preset === "custom" ? t("customShell") : preset}</option>
                  ))}
                </Select>
                {shellPreset === "custom" ? (
                  <input
                    className="kt-input tm-mono"
                    aria-label={t("customShell")}
                    placeholder="bash -l"
                    value={customShell}
                    onChange={(event) => setCustomShell(event.target.value)}
                  />
                ) : null}
              </span>
            ) : null}
            <Menu.Root>
              <Menu.Trigger className="tm-icon" aria-label={t("more")} title={t("more")}>
                <MoreHorizontal aria-hidden="true" />
              </Menu.Trigger>
              <Menu.Portal>
                <Menu.Content className="lt-menu tm-menu" align="end" sideOffset={6}>
                  <Menu.Item className="lt-menu-item" onSelect={() => setSearchOpen(true)}>
                    {t("find")}
                    <span className="lt-menu-hint">{shortcut("F")}</span>
                  </Menu.Item>
                  <Menu.Item className="lt-menu-item" onSelect={() => setSnippetPickerOpen(true)}>
                    {t("snippets")}
                    <span className="lt-menu-hint">{shortcut(";")}</span>
                  </Menu.Item>
                  <Menu.Item className="lt-menu-item" onSelect={() => setSettingsOpen(true)}>{t("settings")}</Menu.Item>
                  <Menu.Separator className="lt-menu-sep" />
                  <Menu.Item className="lt-menu-item" asChild>
                    <Link href={`/terminal?target=${encodeURIComponent(nodeId)}`}>{t("openInTerminal")}</Link>
                  </Menu.Item>
                </Menu.Content>
              </Menu.Portal>
            </Menu.Root>
          </>
        }
      />

      <div className="tm-devterm">
        <SearchBar termRef={termRef} open={searchOpen} onClose={() => setSearchOpen(false)} />
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

      <KeyboardToolbar termRef={termRef} keys={prefs.toolbar_keys ?? undefined} />
      <SnippetPicker snippets={snippets} open={snippetPickerOpen} onClose={() => setSnippetPickerOpen(false)} onSelect={handleSnippetSelect} />
      <SettingsPanel open={settingsOpen} onClose={() => setSettingsOpen(false)} prefs={prefs} onUpdatePrefs={updatePrefs} />
    </div>
  );
}
