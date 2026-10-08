"use client";

import { Input, Select } from "../../../components/ui/Input";

type ConnectionProtocolFieldsProps = {
  protocol: string;
  isEdit: boolean;
  authMethod: "password" | "private_key";
  setAuthMethod: (value: "password" | "private_key") => void;
  password: string;
  setPassword: (value: string) => void;
  privateKey: string;
  setPrivateKey: (value: string) => void;
  passphrase: string;
  setPassphrase: (value: string) => void;
  domain: string;
  setDomain: (value: string) => void;
  shareName: string;
  setShareName: (value: string) => void;
  passiveMode: boolean;
  setPassiveMode: (value: boolean) => void;
  useTLS: boolean;
  setUseTLS: (value: boolean) => void;
  allowCleartextFTP: boolean;
  setAllowCleartextFTP: (value: boolean) => void;
  webdavTLS: boolean;
  setWebdavTLS: (value: boolean) => void;
};

export function ConnectionProtocolFields({
  protocol,
  isEdit,
  authMethod,
  setAuthMethod,
  password,
  setPassword,
  privateKey,
  setPrivateKey,
  passphrase,
  setPassphrase,
  domain,
  setDomain,
  shareName,
  setShareName,
  passiveMode,
  setPassiveMode,
  useTLS,
  setUseTLS,
  allowCleartextFTP,
  setAllowCleartextFTP,
  webdavTLS,
  setWebdavTLS,
}: ConnectionProtocolFieldsProps) {
  return (
    <>
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
                  onChange={(e) => {
                    setUseTLS(e.target.checked);
                    if (e.target.checked) setAllowCleartextFTP(false);
                  }}
                  className="accent-[var(--accent)]"
                />
                Use TLS (FTPS)
              </label>
            </div>
            {!useTLS && (
              <div className="md:col-span-2 rounded-lg border border-[var(--bad)]/30 bg-[var(--bad-glow)] px-3 py-2">
                <label className="flex items-start gap-2 text-xs text-[var(--bad)] cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={allowCleartextFTP}
                    onChange={(e) => setAllowCleartextFTP(e.target.checked)}
                    className="mt-0.5 accent-[var(--bad)]"
                  />
                  <span>
                    Allow cleartext FTP. Passwords and files can be read or
                    changed on the network. The Hub must also enable insecure
                    transport.
                  </span>
                </label>
              </div>
            )}
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
    </>
  );
}
