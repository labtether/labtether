"use client";

import { useCallback } from "react";
import { Plus, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "./ui/Button";
import { Input } from "./ui/Input";
import type { HopConfig } from "../console/models";
import { parsePortInput } from "../lib/portParsing";
import {
  CredentialProfileSelect,
  sshCredentialKinds,
} from "./credentials/CredentialProfileSelect";
import { useCredentialProfiles } from "../hooks/useCredentialProfiles";

type JumpChainEditorProps = {
  value: HopConfig[];
  onChange: (hops: HopConfig[]) => void;
  disabled?: boolean;
};

const MAX_HOPS = 8;

function emptyHop(): HopConfig {
  return { host: "", port: 22, username: "", credential_profile_id: "" };
}

/**
 * "Reach through": the SSH jump hosts the hub goes through, in order, to reach a group's devices. Each hop is a host,
 * port, username and a saved SSH login. No hops means the hub reaches the devices directly.
 */
export function JumpChainEditor({
  value,
  onChange,
  disabled = false,
}: JumpChainEditorProps) {
  const t = useTranslations("groups");
  const credentialProfiles = useCredentialProfiles(!disabled || value.length > 0);

  const addHop = useCallback(() => {
    onChange([...value, emptyHop()]);
  }, [value, onChange]);

  const removeHop = useCallback(
    (index: number) => {
      onChange(value.filter((_, i) => i !== index));
    },
    [value, onChange],
  );

  const updateHop = useCallback(
    (index: number, field: keyof HopConfig, fieldValue: string | number) => {
      onChange(value.map((hop, i) => (i === index ? { ...hop, [field]: fieldValue } : hop)));
    },
    [value, onChange],
  );

  return (
    <fieldset className="pp-hops">
      <legend>{t("jump.title")}</legend>
      {value.length === 0 ? (
        <p className="pp-grey">{t("jump.direct")}</p>
      ) : (
        <div className="pp-hop-list">
          {value.map((hop, index) => {
            const n = index + 1;
            return (
              <div key={index} className="pp-hop">
                <span className="pp-hop-n pp-num" aria-hidden="true">{n}</span>
                <Input
                  placeholder="bastion.example.com"
                  aria-label={t("jump.host", { n })}
                  value={hop.host}
                  onChange={(e) => updateHop(index, "host", e.target.value)}
                  disabled={disabled}
                  autoComplete="off"
                  spellCheck={false}
                />
                <Input
                  type="number"
                  aria-label={t("jump.port", { n })}
                  value={hop.port}
                  onChange={(e) => updateHop(index, "port", parsePortInput(e.target.value, 22))}
                  disabled={disabled}
                  min={1}
                  max={65535}
                />
                <Input
                  aria-label={t("jump.username", { n })}
                  placeholder={t("jump.usernameHint")}
                  value={hop.username}
                  onChange={(e) => updateHop(index, "username", e.target.value)}
                  disabled={disabled}
                  autoComplete="off"
                  spellCheck={false}
                />
                <CredentialProfileSelect
                  id={`jump-hop-${index}-credential-profile`}
                  ariaLabel={t("jump.login", { n })}
                  value={hop.credential_profile_id}
                  onChange={(next) => updateHop(index, "credential_profile_id", next)}
                  profiles={credentialProfiles.profiles}
                  loading={credentialProfiles.loading}
                  error={index === 0 ? credentialProfiles.error : null}
                  allowedKinds={sshCredentialKinds}
                  disabled={disabled}
                  className="min-w-0"
                  emptyLabel={t("jump.noLogin")}
                />
                <button
                  type="button"
                  className="pp-more pp-shown"
                  onClick={() => removeHop(index)}
                  disabled={disabled}
                  aria-label={t("jump.remove", { n })}
                >
                  <X aria-hidden="true" />
                </button>
              </div>
            );
          })}
        </div>
      )}
      {value.length < MAX_HOPS ? (
        <div>
          <Button type="button" size="sm" onClick={addHop} disabled={disabled}>
            <Plus size={14} aria-hidden="true" />
            {t("jump.add")}
          </Button>
        </div>
      ) : null}
    </fieldset>
  );
}
