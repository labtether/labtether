"use client";

import { Check, Pencil, ChevronLeft } from "lucide-react";
import { Input } from "../../../components/ui/Input";
import { PROTOCOL_DOT_COLOR, type RemoteViewProtocol } from "./types";

export function StepIndicator({ step }: { step: 1 | 2 }) {
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

type AuthenticationStepProps = {
  protocol: RemoteViewProtocol;
  bookmarkNickname: string;
  host: string;
  uriString: string;
  username: string;
  password: string;
  allowInsecureVNC: boolean;
  ignoreCertificate: boolean;
  allowLegacySecurity: boolean;
  certificateFingerprints: string;
  spiceSecurityMode: "tls" | "cleartext";
  spiceCAPEM: string;
  canSaveBookmark: boolean;
  onBack: () => void;
  handleConnect: (withCredentials: boolean) => void;
  setUsername: (value: string) => void;
  setPassword: (value: string) => void;
  setAllowInsecureVNC: (value: boolean) => void;
  setIgnoreCertificate: (value: boolean) => void;
  setAllowLegacySecurity: (value: boolean) => void;
  setCertificateFingerprints: (value: string) => void;
  setSPICESecurityMode: (value: "tls" | "cleartext") => void;
  setSPICECAPEM: (value: string) => void;
  setSaveBookmark: (value: boolean) => void;
};

export function AuthenticationStep({
  protocol,
  bookmarkNickname,
  host,
  uriString,
  username,
  password,
  allowInsecureVNC,
  ignoreCertificate,
  allowLegacySecurity,
  certificateFingerprints,
  spiceSecurityMode,
  spiceCAPEM,
  canSaveBookmark,
  onBack,
  handleConnect,
  setUsername,
  setPassword,
  setAllowInsecureVNC,
  setIgnoreCertificate,
  setAllowLegacySecurity,
  setCertificateFingerprints,
  setSPICESecurityMode,
  setSPICECAPEM,
  setSaveBookmark,
}: AuthenticationStepProps) {
  return (
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
                onClick={() => onBack()}
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

            {protocol === "rdp" && (
              <div className="space-y-3 rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
                <div>
                  <label className="mb-1 block text-[11px] font-medium text-[var(--warn)]">
                    Trusted certificate fingerprint
                  </label>
                  <Input
                    type="text"
                    value={certificateFingerprints}
                    onChange={(e) => {
                      setCertificateFingerprints(e.target.value);
                      if (e.target.value.trim()) {
                        setSaveBookmark(false);
                        setIgnoreCertificate(false);
                        setAllowLegacySecurity(false);
                      }
                    }}
                    placeholder="sha256:AA:BB:..."
                    autoComplete="off"
                  />
                  <p className="mt-1 text-[10px] text-[var(--muted)]">
                    Use this for a self-signed certificate or a DNS name pinned
                    to an IP.
                  </p>
                </div>
                <label className="flex items-start gap-2 text-xs text-[var(--warn)]">
                  <input
                    type="checkbox"
                    checked={ignoreCertificate}
                    onChange={(e) => {
                      setIgnoreCertificate(e.target.checked);
                      if (e.target.checked) {
                        setSaveBookmark(false);
                        setAllowLegacySecurity(false);
                        setCertificateFingerprints("");
                      }
                    }}
                    className="mt-0.5 h-3.5 w-3.5 accent-[var(--warn)]"
                  />
                  <span>
                    Allow an untrusted RDP certificate for this session only.
                    This can expose the password and screen to an attacker.
                  </span>
                </label>
                <label className="flex items-start gap-2 text-xs text-[var(--warn)]">
                  <input
                    type="checkbox"
                    checked={allowLegacySecurity}
                    onChange={(e) => {
                      setAllowLegacySecurity(e.target.checked);
                      if (e.target.checked) {
                        setSaveBookmark(false);
                        setIgnoreCertificate(false);
                        setCertificateFingerprints("");
                      }
                    }}
                    className="mt-0.5 h-3.5 w-3.5 accent-[var(--warn)]"
                  />
                  <span>
                    Allow old RDP security for this session only. It has no
                    server certificate and can be intercepted.
                  </span>
                </label>
                {!canSaveBookmark && (
                  <p className="text-[10px] text-[var(--warn)]">
                    This choice applies to this session only. Save as bookmark
                    was turned off.
                  </p>
                )}
              </div>
            )}

            {(protocol === "vnc" || protocol === "ard") && (
              <div className="space-y-2 rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
                <label className="flex items-start gap-2 text-xs text-[var(--warn)]">
                  <input
                    type="checkbox"
                    checked={allowInsecureVNC}
                    onChange={(event) => setAllowInsecureVNC(event.target.checked)}
                    className="mt-0.5 h-3.5 w-3.5 accent-[var(--warn)]"
                  />
                  <span>
                    Allow plain VNC. Screen, keyboard, and login data can be read
                    or changed on the network. The Hub must also allow unsafe
                    transport.
                  </span>
                </label>
                {!allowInsecureVNC && (
                  <p className="text-[10px] text-[var(--warn)]">
                    This choice is required for a direct VNC connection.
                  </p>
                )}
              </div>
            )}

            {protocol === "spice" && (
              <div className="space-y-3 rounded-lg border border-[var(--warn)]/30 bg-[var(--warn-glow)] px-3 py-2">
                <label className="flex items-start gap-2 text-xs text-[var(--warn)]">
                  <input
                    type="checkbox"
                    checked={spiceSecurityMode === "cleartext"}
                    onChange={(e) => {
                      setSPICESecurityMode(
                        e.target.checked ? "cleartext" : "tls",
                      );
                      if (e.target.checked) setSPICECAPEM("");
                    }}
                    className="mt-0.5 h-3.5 w-3.5 accent-[var(--warn)]"
                  />
                  <span>
                    Allow plain-text SPICE. This can expose the password,
                    screen, and keyboard to an attacker. The Hub must also
                    allow unsafe transport.
                  </span>
                </label>
                {spiceSecurityMode === "tls" && (
                  <div>
                    <label className="mb-1 block text-[11px] font-medium text-[var(--warn)]">
                      Custom CA certificate (optional)
                    </label>
                    <textarea
                      value={spiceCAPEM}
                      onChange={(e) => setSPICECAPEM(e.target.value)}
                      maxLength={16 * 1024}
                      rows={4}
                      spellCheck={false}
                      placeholder="-----BEGIN CERTIFICATE-----"
                      className="w-full resize-y rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1.5 font-mono text-[10px] text-[var(--text)] outline-none focus:border-[var(--accent)]"
                    />
                    <p className="mt-1 text-[10px] text-[var(--muted)]">
                      Leave this blank for a certificate trusted by the Hub.
                    </p>
                  </div>
                )}
              </div>
            )}

            {/* Connect button */}
            <button
              type="button"
              onClick={() => handleConnect(true)}
              disabled={
                (protocol === "vnc" || protocol === "ard") && !allowInsecureVNC
              }
              className="w-full py-2.5 rounded-lg text-white text-sm font-semibold shadow-[0_2px_12px_rgba(255,0,128,0.15)] hover:-translate-y-px hover:shadow-[0_4px_20px_rgba(255,0,128,0.25)] transition-all duration-[var(--dur-fast)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:translate-y-0"
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
              disabled={
                (protocol === "vnc" || protocol === "ard") && !allowInsecureVNC
              }
              className="w-full py-2 rounded-lg border border-[var(--panel-border)] text-sm text-[var(--muted)] hover:border-[var(--line)] hover:text-[var(--text-secondary)] transition-all duration-[var(--dur-fast)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              Connect without credentials
            </button>

            {/* Back */}
            <button
              type="button"
              onClick={() => onBack()}
              className="flex items-center justify-center gap-1 py-1.5 text-xs text-[var(--muted)]/60 hover:text-[var(--muted)] transition-colors"
            >
              <ChevronLeft size={12} />
              Back
            </button>
          </div>
  );
}
