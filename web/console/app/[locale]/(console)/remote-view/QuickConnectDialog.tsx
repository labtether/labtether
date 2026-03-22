"use client";

import { useState, useCallback, useEffect } from "react";
import { X, ChevronRight, ChevronLeft, Pencil, Check } from "lucide-react";
import { Input } from "../../../components/ui/Input";
import {
  type RemoteViewProtocol,
  defaultPort,
  PROTOCOLS,
  PROTOCOL_DOT_COLOR,
  PROTOCOL_SELECTOR_STYLE,
  PROTOCOL_NAME_COLOR,
} from "./types";

// ── Props ──

interface QuickConnectDialogProps {
  open: boolean;
  onClose: () => void;
  onConnect: (params: {
    protocol: RemoteViewProtocol;
    host: string;
    port: number;
    username?: string;
    password?: string;
    saveBookmark?: { label: string };
  }) => void;
}

// ── URI parser ──

const URI_REGEX = /^(vnc|rdp|spice|ard):\/\/([^:/]+)(?::(\d+))?$/i;

function parseURI(input: string): {
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
} | null {
  const match = input.trim().match(URI_REGEX);
  if (!match) return null;
  const protocol = match[1].toLowerCase() as RemoteViewProtocol;
  return {
    protocol,
    host: match[2],
    port: match[3] ? parseInt(match[3], 10) : defaultPort(protocol),
  };
}

// ── Step indicator ──

function StepIndicator({ step }: { step: 1 | 2 }) {
  return (
    <div className="flex items-center justify-center gap-0 mb-6">
      {/* Step 1 */}
      <div className="flex items-center gap-1.5">
        {step === 1 ? (
          <span
            className="w-5 h-5 rounded-full border border-[var(--accent)] flex items-center justify-center text-[10px] font-bold text-[var(--accent)]"
            style={{ boxShadow: "0 0 12px rgba(var(--accent-rgb), 0.15)" }}
          >
            1
          </span>
        ) : (
          <span className="w-5 h-5 rounded-full bg-green-500 border border-green-500 flex items-center justify-center">
            <Check size={11} className="text-white" strokeWidth={3} />
          </span>
        )}
        <span
          className={`text-[11px] font-medium ${
            step === 1 ? "text-[var(--text)]" : "text-green-500/80"
          }`}
        >
          Connection
        </span>
      </div>

      {/* Connecting line */}
      <div className="w-10 h-px mx-2">
        <div
          className="h-full w-full"
          style={{
            background:
              step === 2
                ? "linear-gradient(90deg, rgba(0,230,138,0.2), rgba(var(--accent-rgb),0.2))"
                : "var(--line)",
          }}
        />
      </div>

      {/* Step 2 */}
      <div className="flex items-center gap-1.5">
        {step === 2 ? (
          <span
            className="w-5 h-5 rounded-full border border-[var(--accent)] flex items-center justify-center text-[10px] font-bold text-[var(--accent)]"
            style={{ boxShadow: "0 0 12px rgba(var(--accent-rgb), 0.15)" }}
          >
            2
          </span>
        ) : (
          <span className="w-5 h-5 rounded-full border border-[var(--line)] flex items-center justify-center text-[10px] font-medium text-[var(--muted)]">
            2
          </span>
        )}
        <span
          className={`text-[11px] font-medium ${
            step === 2 ? "text-[var(--text)]" : "text-[var(--muted)]"
          }`}
        >
          Auth
        </span>
      </div>
    </div>
  );
}

// ── QuickConnectDialog ──

export default function QuickConnectDialog({
  open,
  onClose,
  onConnect,
}: QuickConnectDialogProps) {
  const [step, setStep] = useState<1 | 2>(1);
  const [protocol, setProtocol] = useState<RemoteViewProtocol>("vnc");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(String(defaultPort("vnc")));
  const [saveBookmark, setSaveBookmark] = useState(false);
  const [bookmarkNickname, setBookmarkNickname] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  const [prevProtocolDefault, setPrevProtocolDefault] = useState(
    String(defaultPort("vnc")),
  );

  // Reset all state when dialog opens
  useEffect(() => {
    if (open) {
      setStep(1);
      setProtocol("vnc");
      setHost("");
      setPort(String(defaultPort("vnc")));
      setSaveBookmark(false);
      setBookmarkNickname("");
      setUsername("");
      setPassword("");
      setPrevProtocolDefault(String(defaultPort("vnc")));
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  // Protocol change: update port only if it still matches the previous protocol's default
  const handleProtocolChange = useCallback(
    (newProtocol: RemoteViewProtocol) => {
      setProtocol(newProtocol);
      if (port === prevProtocolDefault || port === "") {
        setPort(String(defaultPort(newProtocol)));
      }
      setPrevProtocolDefault(String(defaultPort(newProtocol)));
    },
    [port, prevProtocolDefault],
  );

  // URI auto-detection in host field
  const handleHostChange = useCallback((value: string) => {
    setHost(value);
    const parsed = parseURI(value);
    if (parsed) {
      setProtocol(parsed.protocol);
      setHost(parsed.host);
      setPort(String(parsed.port));
      setPrevProtocolDefault(String(defaultPort(parsed.protocol)));
    }
  }, []);

  const canAdvance = host.trim() !== "";

  const handleNext = useCallback(() => {
    if (canAdvance) setStep(2);
  }, [canAdvance]);

  const handleConnect = useCallback(
    (withCredentials: boolean) => {
      const parsedPort = parseInt(port, 10);
      onConnect({
        protocol,
        host: host.trim(),
        port: Number.isFinite(parsedPort) && parsedPort > 0 ? parsedPort : defaultPort(protocol),
        ...(withCredentials && username.trim()
          ? { username: username.trim() }
          : {}),
        ...(withCredentials && password ? { password } : {}),
        ...(saveBookmark
          ? { saveBookmark: { label: bookmarkNickname.trim() || host.trim() } }
          : {}),
      });
      onClose();
    },
    [protocol, host, port, username, password, saveBookmark, bookmarkNickname, onConnect, onClose],
  );

  if (!open) return null;

  const uriString = `${protocol}://${host.trim() || "..."}:${port}`;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Quick Connect"
      className="fixed inset-0 z-50 flex items-center justify-center"
    >
      {/* Backdrop */}
      <button
        type="button"
        aria-label="Close dialog"
        onClick={onClose}
        className="absolute inset-0 bg-black/50"
      />

      {/* Dialog container */}
      <div
        className="relative z-10 w-80 border border-[var(--panel-border)] rounded-2xl p-6"
        style={{
          background: "linear-gradient(180deg, var(--panel-glass), var(--panel))",
          boxShadow:
            "0 4px 24px rgba(0,0,0,0.5), 0 12px 48px rgba(0,0,0,0.4)",
        }}
      >
        {/* Top-edge specular highlight */}
        <div
          className="absolute top-0 left-[15%] right-[15%] h-px pointer-events-none"
          style={{
            background:
              "linear-gradient(90deg, transparent, var(--surface), transparent)",
          }}
        />

        {/* Header */}
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-sm font-semibold text-[var(--text)]">
            Quick Connect
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="inline-flex h-6 w-6 items-center justify-center rounded-md border border-[var(--panel-border)] text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)]"
          >
            <X size={13} />
          </button>
        </div>

        {/* Step indicator */}
        <StepIndicator step={step} />

        {/* ── Step 1: Connection ── */}
        {step === 1 && (
          <div className="flex flex-col gap-4">
            {/* Protocol selector — 4-column grid */}
            <div>
              <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                Protocol
              </label>
              <div className="grid grid-cols-4 gap-1.5">
                {PROTOCOLS.map((p) => {
                  const isSelected = protocol === p.value;
                  const selectorStyle = PROTOCOL_SELECTOR_STYLE[p.value];
                  const nameColor = PROTOCOL_NAME_COLOR[p.value];
                  return (
                    <button
                      key={p.value}
                      type="button"
                      onClick={() => handleProtocolChange(p.value)}
                      className={`flex flex-col items-center gap-0.5 py-2 rounded-lg border text-center transition-all duration-[var(--dur-fast)] ${
                        isSelected
                          ? selectorStyle.selected
                          : selectorStyle.unselected
                      }`}
                    >
                      <span
                        className={`text-[11px] font-semibold ${
                          isSelected ? nameColor.selected : nameColor.unselected
                        }`}
                      >
                        {p.label}
                      </span>
                      <span className="text-[9px] text-[var(--muted)]">
                        :{defaultPort(p.value)}
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Host */}
            <div>
              <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                Host
              </label>
              <Input
                type="text"
                value={host}
                onChange={(e) => handleHostChange(e.target.value)}
                placeholder="192.168.1.10 or hostname"
                autoComplete="off"
                autoFocus
              />
            </div>

            {/* Port */}
            <div>
              <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                Port
              </label>
              <Input
                type="text"
                inputMode="numeric"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                placeholder={String(defaultPort(protocol))}
              />
            </div>

            {/* Save as bookmark */}
            <label className="flex items-center gap-2 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={saveBookmark}
                onChange={(e) => setSaveBookmark(e.target.checked)}
                className="h-3.5 w-3.5 rounded border-[var(--line)] accent-[var(--accent)]"
              />
              <span className="text-xs text-[var(--text)]">
                Save as bookmark
              </span>
            </label>

            {saveBookmark && (
              <div>
                <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                  Nickname
                </label>
                <Input
                  type="text"
                  value={bookmarkNickname}
                  onChange={(e) => setBookmarkNickname(e.target.value)}
                  placeholder={host.trim() || "My Server"}
                />
              </div>
            )}

            {/* Next button */}
            <button
              type="button"
              disabled={!canAdvance}
              onClick={handleNext}
              className={`w-full py-2.5 rounded-lg text-sm font-semibold transition-all duration-[var(--dur-fast)] ${
                canAdvance
                  ? "text-white shadow-[0_2px_12px_rgba(255,0,128,0.15)] hover:-translate-y-px hover:shadow-[0_4px_20px_rgba(255,0,128,0.25)]"
                  : "text-white/40 cursor-not-allowed"
              }`}
              style={{
                background: "linear-gradient(135deg, var(--accent), #d4006a)",
                opacity: canAdvance ? 1 : 0.4,
              }}
            >
              <span className="flex items-center justify-center gap-1">
                Next <ChevronRight size={14} />
              </span>
            </button>

            {/* Hint */}
            <p className="text-[10px] text-[var(--muted)] text-center">
              Paste a URI — vnc://, rdp://, spice://, ard://
            </p>
          </div>
        )}

        {/* ── Step 2: Authentication ── */}
        {step === 2 && (
          <div className="flex flex-col gap-4">
            {/* Connection summary card */}
            <div className="flex items-center gap-2.5 p-3 rounded-lg border border-[var(--panel-border)] bg-[var(--surface)]">
              <span
                className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${PROTOCOL_DOT_COLOR[protocol]}`}
              />
              <div className="flex-1 min-w-0">
                <div className="text-xs font-medium text-[var(--text)] truncate">
                  {bookmarkNickname.trim() || host.trim()}
                </div>
                <div className="text-[10px] font-mono text-[var(--muted)] truncate">
                  {uriString}
                </div>
              </div>
              <button
                type="button"
                onClick={() => setStep(1)}
                aria-label="Edit connection"
                className="p-1 rounded text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
              >
                <Pencil size={12} />
              </button>
            </div>

            {/* Username */}
            <div>
              <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                Username
              </label>
              <Input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                autoComplete="username"
                autoFocus
              />
            </div>

            {/* Password */}
            <div>
              <label className="block text-[11px] font-medium text-[var(--muted)] mb-1.5">
                Password
              </label>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Password"
                autoComplete="current-password"
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleConnect(true);
                }}
              />
            </div>

            {/* Connect button */}
            <button
              type="button"
              onClick={() => handleConnect(true)}
              className="w-full py-2.5 rounded-lg text-white text-sm font-semibold shadow-[0_2px_12px_rgba(255,0,128,0.15)] hover:-translate-y-px hover:shadow-[0_4px_20px_rgba(255,0,128,0.25)] transition-all duration-[var(--dur-fast)]"
              style={{
                background: "linear-gradient(135deg, var(--accent), #d4006a)",
              }}
            >
              Connect
            </button>

            {/* Connect without credentials */}
            <button
              type="button"
              onClick={() => handleConnect(false)}
              className="w-full py-2 rounded-lg border border-[var(--panel-border)] text-sm text-[var(--muted)] hover:border-[var(--line)] hover:text-[var(--text-secondary)] transition-all duration-[var(--dur-fast)]"
            >
              Connect without credentials
            </button>

            {/* Back */}
            <button
              type="button"
              onClick={() => setStep(1)}
              className="flex items-center justify-center gap-1 py-1.5 text-xs text-[var(--muted)]/60 hover:text-[var(--muted)] transition-colors"
            >
              <ChevronLeft size={12} />
              Back
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
