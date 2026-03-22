"use client";

import { useCallback, useState } from "react";
import { ArrowLeft, CheckCircle2, XCircle, Loader2, Trash2 } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import type { FileConnection, CreateFileConnectionRequest, TestResult } from "./fileConnectionsClient";
import { createFileConnection, updateFileConnection, deleteFileConnection, testFileConnectionStateless } from "./fileConnectionsClient";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ConnectionFormProps {
  protocol: string;
  /** When provided, form operates in edit mode and pre-fills fields. */
  existingConnection?: FileConnection;
  onConnect: (connection: FileConnection) => void;
  onCancel: () => void;
  /** Called after a successful delete so the parent can clean up. */
  onDeleted?: (id: string) => void;
}

// ---------------------------------------------------------------------------
// Protocol defaults
// ---------------------------------------------------------------------------

const DEFAULT_PORTS: Record<string, number> = {
  sftp: 22,
  smb: 445,
  ftp: 21,
  webdav: 443,
};

const PROTOCOL_LABELS: Record<string, string> = {
  sftp: "SFTP",
  smb: "SMB",
  ftp: "FTP",
  webdav: "WebDAV",
};

const PROTOCOL_ACCENT: Record<string, string> = {
  sftp: "bg-blue-500",
  smb: "bg-orange-500",
  ftp: "bg-teal-500",
  webdav: "bg-cyan-500",
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ConnectionForm({ protocol, existingConnection, onConnect, onCancel, onDeleted }: ConnectionFormProps) {
  const isEdit = !!existingConnection;
  const extra = existingConnection?.extra_config ?? {};

  // Common fields — pre-fill from existing connection when editing
  const [name, setName] = useState(existingConnection?.name ?? "");
  const [host, setHost] = useState(existingConnection?.host ?? "");
  const [port, setPort] = useState<number>(existingConnection?.port ?? DEFAULT_PORTS[protocol] ?? 22);
  const [initialPath, setInitialPath] = useState(existingConnection?.initial_path ?? "");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  // SFTP-specific
  const [authMethod, setAuthMethod] = useState<"password" | "private_key">("password");
  const [privateKey, setPrivateKey] = useState("");
  const [passphrase, setPassphrase] = useState("");

  // SMB-specific
  const [domain, setDomain] = useState((extra.domain as string) ?? "");
  const [shareName, setShareName] = useState((extra.smb_share as string) ?? "");

  // FTP-specific (fall back to old keys for connections saved before the rename)
  const [passiveMode, setPassiveMode] = useState((extra.ftp_passive as boolean) ?? (extra.passive_mode as boolean) ?? true);
  const [useTLS, setUseTLS] = useState((extra.ftp_tls as boolean) ?? (extra.use_tls as boolean) ?? false);

  // WebDAV-specific (fall back to old scheme key)
  const [webdavTLS, setWebdavTLS] = useState((extra.webdav_tls as boolean) ?? (extra.scheme === "https" || extra.scheme === undefined));

  // Form state
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [formError, setFormError] = useState("");
  const [testResult, setTestResult] = useState<TestResult | null>(null);

  // ---------------------------------------------------------------------------
  // Build request from form state
  // ---------------------------------------------------------------------------

  const buildRequest = useCallback((): CreateFileConnectionRequest => {
    const extraCfg: Record<string, unknown> = {};

    if (protocol === "smb") {
      if (domain.trim()) extraCfg.domain = domain.trim();
      if (shareName.trim()) extraCfg.smb_share = shareName.trim();
    }
    if (protocol === "ftp") {
      extraCfg.ftp_passive = passiveMode;
      extraCfg.ftp_tls = useTLS;
    }
    if (protocol === "webdav") {
      extraCfg.webdav_tls = webdavTLS;
    }

    const secret =
      protocol === "sftp" && authMethod === "private_key"
        ? privateKey
        : password;

    return {
      name: name.trim(),
      protocol,
      host: host.trim(),
      port,
      initial_path: initialPath.trim() || undefined,
      username: username.trim(),
      secret,
      passphrase: protocol === "sftp" && authMethod === "private_key" && passphrase.trim()
        ? passphrase.trim()
        : undefined,
      auth_method: protocol === "sftp" ? authMethod : undefined,
      extra_config: Object.keys(extraCfg).length > 0 ? extraCfg : undefined,
    };
  }, [
    protocol, name, host, port, initialPath, username, password,
    authMethod, privateKey, passphrase, domain, shareName, passiveMode, useTLS, webdavTLS,
  ]);

  // ---------------------------------------------------------------------------
  // Validation
  // ---------------------------------------------------------------------------

  const validate = useCallback((requireSecret: boolean): string | null => {
    if (!name.trim()) return "Connection name is required.";
    if (!host.trim()) return "Host is required.";
    if (protocol === "smb" && !shareName.trim()) return "Share name is required for SMB.";
    if (requireSecret) {
      const secret = protocol === "sftp" && authMethod === "private_key" ? privateKey : password;
      if (!secret.trim()) return "Credentials are required.";
    }
    return null;
  }, [name, host, protocol, shareName, authMethod, privateKey, password]);

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  const handleTest = useCallback(async () => {
    const err = validate(true);
    if (err) {
      setFormError(err);
      return;
    }
    setFormError("");
    setTestResult(null);
    setTesting(true);
    try {
      const result = await testFileConnectionStateless(buildRequest());
      setTestResult(result);
    } catch (e) {
      setTestResult({
        success: false,
        error: e instanceof Error ? e.message : "Test failed.",
      });
    } finally {
      setTesting(false);
    }
  }, [validate, buildRequest]);

  const handleSave = useCallback(async () => {
    // In edit mode, credentials are optional (only update if user entered new ones)
    const err = validate(!isEdit);
    if (err) {
      setFormError(err);
      return;
    }
    setFormError("");
    setSaving(true);
    try {
      if (isEdit) {
        const req = buildRequest();
        // Only send secret if user entered a new one
        const secret = req.secret?.trim() ? req.secret : undefined;
        const connection = await updateFileConnection(existingConnection.id, {
          name: req.name,
          protocol: req.protocol,
          host: req.host,
          port: req.port,
          initial_path: req.initial_path,
          username: req.username?.trim() ? req.username : undefined,
          secret,
          auth_method: req.auth_method,
          extra_config: req.extra_config,
        });
        onConnect(connection);
      } else {
        const connection = await createFileConnection(buildRequest());
        onConnect(connection);
      }
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Failed to save connection.");
    } finally {
      setSaving(false);
    }
  }, [validate, buildRequest, onConnect, isEdit, existingConnection]);

  const handleDelete = useCallback(async () => {
    if (!existingConnection) return;
    setFormError("");
    setDeleting(true);
    try {
      await deleteFileConnection(existingConnection.id);
      onDeleted?.(existingConnection.id);
    } catch (e) {
      setFormError(e instanceof Error ? e.message : "Failed to delete connection.");
    } finally {
      setDeleting(false);
    }
  }, [existingConnection, onDeleted]);

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const label = PROTOCOL_LABELS[protocol] ?? protocol.toUpperCase();
  const accent = PROTOCOL_ACCENT[protocol] ?? "bg-blue-500";

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button
          className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
          onClick={onCancel}
          title="Back"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${accent}`} />
        <h2 className="text-base font-semibold text-[var(--text)]">
          {isEdit ? `Edit ${label} Connection` : `New ${label} Connection`}
        </h2>
        {isEdit && (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto text-[var(--bad)] hover:bg-[var(--bad-glow)]"
            onClick={handleDelete}
            loading={deleting}
            title="Delete connection"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
        )}
      </div>

      {/* Form grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {/* Name -- full width */}
        <div className="md:col-span-2">
          <label className="block text-xs font-medium text-[var(--muted)] mb-1">
            Connection Name *
          </label>
          <Input
            placeholder={`My ${label} server`}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        {/* Host */}
        <div>
          <label className="block text-xs font-medium text-[var(--muted)] mb-1">
            Host *
          </label>
          <Input
            placeholder="192.168.1.100 or hostname"
            value={host}
            onChange={(e) => setHost(e.target.value)}
          />
        </div>

        {/* Port */}
        <div>
          <label className="block text-xs font-medium text-[var(--muted)] mb-1">
            Port
          </label>
          <Input
            type="number"
            value={port}
            onChange={(e) => setPort(Number(e.target.value) || 0)}
          />
        </div>

        {/* Initial path */}
        <div className="md:col-span-2">
          <label className="block text-xs font-medium text-[var(--muted)] mb-1">
            Initial Path
          </label>
          <Input
            placeholder={protocol === "smb" ? "/" : "/home/user"}
            value={initialPath}
            onChange={(e) => setInitialPath(e.target.value)}
          />
        </div>

        {/* Username */}
        <div>
          <label className="block text-xs font-medium text-[var(--muted)] mb-1">
            Username{isEdit ? "" : ""}
          </label>
          <Input
            placeholder={isEdit ? "(unchanged)" : "user"}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />
        </div>

        {/* --- SFTP-specific fields --- */}
        {protocol === "sftp" && (
          <>
            <div>
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Auth Method
              </label>
              <Select
                value={authMethod}
                onChange={(e) => setAuthMethod(e.target.value as "password" | "private_key")}
              >
                <option value="password">Password</option>
                <option value="private_key">Private Key</option>
              </Select>
            </div>

            {authMethod === "password" ? (
              <div className="md:col-span-2">
                <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                  Password
                </label>
                <Input
                  type="password"
                  placeholder={isEdit ? "(unchanged)" : "Password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
            ) : (
              <>
                <div className="md:col-span-2">
                  <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                    Private Key
                  </label>
                  <textarea
                    className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-xs text-[var(--text)] placeholder:text-[var(--muted)] font-mono resize-y min-h-[100px] outline-none focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_var(--accent-subtle)] transition-[border-color,box-shadow] duration-[var(--dur-fast)]"
                    placeholder={isEdit ? "(unchanged)" : "-----BEGIN OPENSSH PRIVATE KEY-----\n..."}
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                    rows={5}
                  />
                </div>
                <div className="md:col-span-2">
                  <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                    Passphrase (optional)
                  </label>
                  <Input
                    type="password"
                    placeholder="Key passphrase"
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                  />
                </div>
              </>
            )}
          </>
        )}

        {/* --- SMB-specific fields --- */}
        {protocol === "smb" && (
          <>
            <div>
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Password
              </label>
              <Input
                type="password"
                placeholder={isEdit ? "(unchanged)" : "Password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Domain (optional)
              </label>
              <Input
                placeholder="WORKGROUP"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
              />
            </div>
            <div className="md:col-span-2">
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Share Name *
              </label>
              <Input
                placeholder="shared"
                value={shareName}
                onChange={(e) => setShareName(e.target.value)}
              />
            </div>
          </>
        )}

        {/* --- FTP-specific fields --- */}
        {protocol === "ftp" && (
          <>
            <div>
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Password
              </label>
              <Input
                type="password"
                placeholder={isEdit ? "(unchanged)" : "Password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            <div className="flex items-center gap-4 md:col-span-2 py-1">
              <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={passiveMode}
                  onChange={(e) => setPassiveMode(e.target.checked)}
                  className="accent-[var(--accent)]"
                />
                Passive Mode
              </label>
              <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={useTLS}
                  onChange={(e) => setUseTLS(e.target.checked)}
                  className="accent-[var(--accent)]"
                />
                Use TLS (FTPS)
              </label>
            </div>
          </>
        )}

        {/* --- WebDAV-specific fields --- */}
        {protocol === "webdav" && (
          <>
            <div>
              <label className="block text-xs font-medium text-[var(--muted)] mb-1">
                Password
              </label>
              <Input
                type="password"
                placeholder={isEdit ? "(unchanged)" : "Password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
              />
            </div>
            <div className="flex items-center gap-4 py-1">
              <label className="flex items-center gap-2 text-xs text-[var(--text)] cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={webdavTLS}
                  onChange={(e) => setWebdavTLS(e.target.checked)}
                  className="accent-[var(--accent)]"
                />
                Use HTTPS
              </label>
            </div>
          </>
        )}
      </div>

      {/* Error */}
      {formError && (
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-[var(--bad-glow)] text-[var(--bad)] text-xs">
          <XCircle className="w-3.5 h-3.5 flex-shrink-0" />
          {formError}
        </div>
      )}

      {/* Test result */}
      {testResult && (
        <div
          className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs ${
            testResult.success
              ? "bg-[var(--ok-glow)] text-[var(--ok)]"
              : "bg-[var(--bad-glow)] text-[var(--bad)]"
          }`}
        >
          {testResult.success ? (
            <>
              <CheckCircle2 className="w-3.5 h-3.5 flex-shrink-0" />
              Connection successful
              {testResult.latency_ms != null && (
                <span className="text-[var(--muted)] ml-1">
                  ({testResult.latency_ms}ms)
                </span>
              )}
            </>
          ) : (
            <>
              <XCircle className="w-3.5 h-3.5 flex-shrink-0" />
              {testResult.error || "Connection failed."}
            </>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 pt-1">
        <Button variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleTest}
          disabled={testing}
        >
          {testing ? (
            <>
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
              Testing...
            </>
          ) : (
            "Test Connection"
          )}
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={handleSave}
          loading={saving}
        >
          {isEdit ? "Save Changes" : "Save & Connect"}
        </Button>
      </div>
    </div>
  );
}
