"use client";

import { useCallback } from "react";
import { Plus, X } from "lucide-react";
import { Button } from "./ui/Button";
import { Input } from "./ui/Input";
import type { HopConfig } from "../console/models";

type JumpChainEditorProps = {
  value: HopConfig[];
  onChange: (hops: HopConfig[]) => void;
  disabled?: boolean;
};

function emptyHop(): HopConfig {
  return { host: "", port: 22, username: "", credential_profile_id: "" };
}

export function JumpChainEditor({
  value,
  onChange,
  disabled = false,
}: JumpChainEditorProps) {
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
      const next = value.map((hop, i) =>
        i === index ? { ...hop, [field]: fieldValue } : hop,
      );
      onChange(next);
    },
    [value, onChange],
  );

  return (
    <div className="space-y-2">
      {value.length === 0 ? (
        <p className="text-[11px] text-[var(--muted)] py-1">
          Assets in this group are accessed directly (no jump hosts).
        </p>
      ) : (
        <div className="space-y-2">
          {value.map((hop, index) => (
            <div
              key={index}
              className="flex items-start gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-2"
            >
              <span className="shrink-0 mt-2 text-[10px] font-mono text-[var(--muted)] w-4 text-right">
                {index + 1}.
              </span>
              <div className="flex-1 grid grid-cols-[1fr_70px_1fr_1fr] gap-2">
                <Input
                  placeholder="Host"
                  value={hop.host}
                  onChange={(e) => updateHop(index, "host", e.target.value)}
                  disabled={disabled}
                  className="!py-1.5 !text-xs"
                />
                <Input
                  type="number"
                  placeholder="Port"
                  value={hop.port}
                  onChange={(e) =>
                    updateHop(index, "port", parseInt(e.target.value, 10) || 22)
                  }
                  disabled={disabled}
                  className="!py-1.5 !text-xs"
                  min={1}
                  max={65535}
                />
                <Input
                  placeholder="Username"
                  value={hop.username}
                  onChange={(e) => updateHop(index, "username", e.target.value)}
                  disabled={disabled}
                  className="!py-1.5 !text-xs"
                />
                <Input
                  placeholder="Credential Profile ID"
                  value={hop.credential_profile_id}
                  onChange={(e) =>
                    updateHop(index, "credential_profile_id", e.target.value)
                  }
                  disabled={disabled}
                  className="!py-1.5 !text-xs"
                />
              </div>
              <button
                type="button"
                className="shrink-0 mt-1.5 p-1 rounded text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer disabled:opacity-40 disabled:pointer-events-none"
                style={{ transitionDuration: "var(--dur-instant)" }}
                onClick={() => removeHop(index)}
                disabled={disabled}
                title="Remove hop"
              >
                <X size={12} />
              </button>
            </div>
          ))}
        </div>
      )}

      <Button
        variant="ghost"
        size="sm"
        onClick={addHop}
        disabled={disabled}
      >
        <Plus size={12} />
        Add hop
      </Button>
    </div>
  );
}
