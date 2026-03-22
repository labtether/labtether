"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import type { QuickConnectParams } from "../../hooks/useSession";

type AuthMethod = "password" | "private_key";

interface QuickConnectDialogProps {
  open: boolean;
  onClose: () => void;
  onConnect: (params: QuickConnectParams) => void;
}

export default function QuickConnectDialog({
  open,
  onClose,
  onConnect,
}: QuickConnectDialogProps) {
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [username, setUsername] = useState("");
  const [authMethod, setAuthMethod] = useState<AuthMethod>("password");
  const [password, setPassword] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [strictHostKey, setStrictHostKey] = useState(true);

  const hostInputRef = useRef<HTMLInputElement | null>(null);

  // Focus host input when dialog opens.
  useEffect(() => {
    if (open) {
      const timer = window.setTimeout(() => hostInputRef.current?.focus(), 0);
      return () => window.clearTimeout(timer);
    }
  }, [open]);

  // Close on Escape.
  useEffect(() => {
    if (!open) return undefined;
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const handleSubmit = useCallback(
    (event: React.FormEvent) => {
      event.preventDefault();
      const trimmedHost = host.trim();
      const trimmedUsername = username.trim();
      if (!trimmedHost || !trimmedUsername) return;

      const parsedPort = parseInt(port, 10);
      const params: QuickConnectParams = {
        host: trimmedHost,
        port: Number.isFinite(parsedPort) && parsedPort > 0 ? parsedPort : 22,
        username: trimmedUsername,
        auth_method: authMethod,
        strict_host_key: strictHostKey,
      };

      if (authMethod === "password") {
        params.password = password;
      } else {
        params.private_key = privateKey;
        if (passphrase) {
          params.passphrase = passphrase;
        }
      }

      onConnect(params);
    },
    [host, port, username, authMethod, password, privateKey, passphrase, strictHostKey, onConnect],
  );

  if (!open) return null;

  const canSubmit = host.trim() !== "" && username.trim() !== "";

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Quick Connect"
      className="fixed inset-0 z-[70] flex items-start justify-center p-4 pt-[8vh]"
    >
      <button
        type="button"
        aria-label="Close quick connect"
        onClick={onClose}
        className="absolute inset-0 bg-black/76"
      />

      <div className="relative z-10 w-full max-w-md overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--panel)] shadow-[var(--shadow-lg)]">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[var(--line)] px-4 py-3">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
              Quick Connect
            </p>
            <p className="mt-0.5 text-sm font-semibold text-[var(--text)]">
              SSH into any host
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-6 w-6 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--surface)] text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)]"
          >
            <X size={13} />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="space-y-4 p-4">
          {/* Host + Port */}
          <div className="grid grid-cols-[1fr_5rem] gap-2">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                Host <span className="text-[var(--bad)]">*</span>
              </span>
              <input
                ref={hostInputRef}
                type="text"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="192.168.1.10 or hostname"
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
                autoComplete="off"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                Port
              </span>
              <input
                type="text"
                inputMode="numeric"
                value={port}
                onChange={(e) => setPort(e.target.value)}
                placeholder="22"
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
              />
            </label>
          </div>

          {/* Username */}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
              Username <span className="text-[var(--bad)]">*</span>
            </span>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="root"
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
              autoComplete="off"
            />
          </label>

          {/* Auth Method Toggle */}
          <div>
            <span className="mb-1.5 block text-xs font-medium text-[var(--muted)]">
              Authentication
            </span>
            <div className="inline-flex rounded-lg border border-[var(--line)] bg-[var(--surface)] p-0.5">
              <button
                type="button"
                onClick={() => setAuthMethod("password")}
                className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                  authMethod === "password"
                    ? "bg-[var(--panel)] text-[var(--text)] shadow-sm"
                    : "text-[var(--muted)] hover:text-[var(--text)]"
                }`}
              >
                Password
              </button>
              <button
                type="button"
                onClick={() => setAuthMethod("private_key")}
                className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                  authMethod === "private_key"
                    ? "bg-[var(--panel)] text-[var(--text)] shadow-sm"
                    : "text-[var(--muted)] hover:text-[var(--text)]"
                }`}
              >
                Private Key
              </button>
            </div>
          </div>

          {/* Credential Fields */}
          {authMethod === "password" ? (
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                Password
              </span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="SSH password"
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
                autoComplete="off"
              />
            </label>
          ) : (
            <div className="space-y-3">
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                  Private Key (PEM)
                </span>
                <textarea
                  value={privateKey}
                  onChange={(e) => setPrivateKey(e.target.value)}
                  placeholder={"-----BEGIN OPENSSH PRIVATE KEY-----\n..."}
                  rows={4}
                  className="w-full resize-y rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 font-mono text-xs text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
                />
              </label>
              <label className="block">
                <span className="mb-1 block text-xs font-medium text-[var(--muted)]">
                  Passphrase <span className="text-xs font-normal text-[var(--muted)]">(optional)</span>
                </span>
                <input
                  type="password"
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  placeholder="Key passphrase"
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] outline-none transition-colors placeholder:text-[var(--muted)]/50 focus:border-[var(--accent)]"
                  autoComplete="off"
                />
              </label>
            </div>
          )}

          {/* Strict Host Key Checking */}
          <label className="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              checked={strictHostKey}
              onChange={(e) => setStrictHostKey(e.target.checked)}
              className="h-3.5 w-3.5 rounded border-[var(--line)] accent-[var(--accent)]"
            />
            <span className="text-xs text-[var(--text)]">
              Strict host key checking
            </span>
          </label>

          {/* Actions */}
          <div className="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-sm text-[var(--text)] transition-colors hover:bg-[var(--hover)]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
                canSubmit
                  ? "bg-[var(--accent)] text-white hover:bg-[var(--accent-hover)]"
                  : "cursor-not-allowed bg-[var(--surface)] text-[var(--muted)] opacity-60"
              }`}
            >
              Connect
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
