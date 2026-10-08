"use client";

import { useState, useCallback, useEffect } from "react";
import { X, ChevronRight } from "lucide-react";
import { Input } from "../../../components/ui/Input";
import { AuthenticationStep, StepIndicator } from "./QuickConnectSteps";
import { parsePortInput } from "../../../lib/portParsing";
import {
  type RemoteViewProtocol,
  defaultPort,
  PROTOCOLS,
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
    allowInsecureVNC?: boolean;
    ignoreCertificate?: boolean;
    allowLegacySecurity?: boolean;
    certificateFingerprints?: string;
    spiceSecurityMode?: "tls" | "cleartext";
    spiceCAPEM?: string;
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
    port: match[3] ? parsePortInput(match[3], defaultPort(protocol)) : defaultPort(protocol),
  };
}

export function canSaveQuickConnectBookmark(
  protocol: RemoteViewProtocol,
  ignoreCertificate: boolean,
  allowLegacySecurity: boolean,
  certificateFingerprints: string,
): boolean {
  return (
    protocol !== "rdp" ||
    (!ignoreCertificate &&
      !allowLegacySecurity &&
      certificateFingerprints.trim() === "")
  );
}

// ── Step indicator ──

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
  const [allowInsecureVNC, setAllowInsecureVNC] = useState(false);
  const [ignoreCertificate, setIgnoreCertificate] = useState(false);
  const [allowLegacySecurity, setAllowLegacySecurity] = useState(false);
  const [certificateFingerprints, setCertificateFingerprints] = useState("");
  const [spiceSecurityMode, setSPICESecurityMode] = useState<
    "tls" | "cleartext"
  >("tls");
  const [spiceCAPEM, setSPICECAPEM] = useState("");
  const canSaveBookmark = canSaveQuickConnectBookmark(
    protocol,
    ignoreCertificate,
    allowLegacySecurity,
    certificateFingerprints,
  );

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
      setAllowInsecureVNC(false);
      setIgnoreCertificate(false);
      setAllowLegacySecurity(false);
      setCertificateFingerprints("");
      setSPICESecurityMode("tls");
      setSPICECAPEM("");
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
      if (newProtocol !== "rdp") {
        setIgnoreCertificate(false);
        setAllowLegacySecurity(false);
        setCertificateFingerprints("");
      }
      if (newProtocol !== "vnc" && newProtocol !== "ard") {
        setAllowInsecureVNC(false);
      }
      if (newProtocol !== "spice") {
        setSPICESecurityMode("tls");
        setSPICECAPEM("");
      }
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
      if (parsed.protocol !== "rdp") {
        setIgnoreCertificate(false);
        setAllowLegacySecurity(false);
        setCertificateFingerprints("");
      }
      if (parsed.protocol !== "vnc" && parsed.protocol !== "ard") {
        setAllowInsecureVNC(false);
      }
      if (parsed.protocol !== "spice") {
        setSPICESecurityMode("tls");
        setSPICECAPEM("");
      }
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
      onConnect({
        protocol,
        host: host.trim(),
        port: parsePortInput(port, defaultPort(protocol)),
        ...(withCredentials && username.trim()
          ? { username: username.trim() }
          : {}),
        ...(withCredentials && password ? { password } : {}),
        ...((protocol === "vnc" || protocol === "ard") && allowInsecureVNC
          ? { allowInsecureVNC: true }
          : {}),
        ...(protocol === "rdp" && ignoreCertificate
          ? { ignoreCertificate: true }
          : {}),
        ...(protocol === "rdp" && allowLegacySecurity
          ? { allowLegacySecurity: true }
          : {}),
        ...(protocol === "rdp" && certificateFingerprints.trim()
          ? { certificateFingerprints: certificateFingerprints.trim() }
          : {}),
        ...(protocol === "spice"
          ? {
              spiceSecurityMode,
              ...(spiceSecurityMode === "tls" && spiceCAPEM.trim()
                ? { spiceCAPEM: spiceCAPEM.trim() }
                : {}),
            }
          : {}),
        ...(saveBookmark && canSaveBookmark
          ? { saveBookmark: { label: bookmarkNickname.trim() || host.trim() } }
          : {}),
      });
      onClose();
    },
    [
      protocol,
      host,
      port,
      username,
      password,
      allowInsecureVNC,
      ignoreCertificate,
      allowLegacySecurity,
      certificateFingerprints,
      spiceSecurityMode,
      spiceCAPEM,
      saveBookmark,
      canSaveBookmark,
      bookmarkNickname,
      onConnect,
      onClose,
    ],
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
            <label className={`flex items-center gap-2 select-none ${canSaveBookmark ? "cursor-pointer" : "cursor-not-allowed"}`}>
              <input
                type="checkbox"
                checked={saveBookmark}
                onChange={(e) => setSaveBookmark(e.target.checked)}
                disabled={!canSaveBookmark}
                className="h-3.5 w-3.5 rounded border-[var(--line)] accent-[var(--accent)]"
              />
              <span className="text-xs text-[var(--text)]">
                {canSaveBookmark
                  ? "Save as bookmark"
                  : "Session-only RDP security choices cannot be bookmarked"}
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
          <AuthenticationStep
            protocol={protocol}
            bookmarkNickname={bookmarkNickname}
            host={host}
            uriString={uriString}
            username={username}
            password={password}
            allowInsecureVNC={allowInsecureVNC}
            ignoreCertificate={ignoreCertificate}
            allowLegacySecurity={allowLegacySecurity}
            certificateFingerprints={certificateFingerprints}
            spiceSecurityMode={spiceSecurityMode}
            spiceCAPEM={spiceCAPEM}
            canSaveBookmark={canSaveBookmark}
            onBack={() => setStep(1)}
            handleConnect={handleConnect}
            setUsername={setUsername}
            setPassword={setPassword}
            setAllowInsecureVNC={setAllowInsecureVNC}
            setIgnoreCertificate={setIgnoreCertificate}
            setAllowLegacySecurity={setAllowLegacySecurity}
            setCertificateFingerprints={setCertificateFingerprints}
            setSPICESecurityMode={setSPICESecurityMode}
            setSPICECAPEM={setSPICECAPEM}
            setSaveBookmark={setSaveBookmark}
          />
        )}
      </div>
    </div>
  );
}
