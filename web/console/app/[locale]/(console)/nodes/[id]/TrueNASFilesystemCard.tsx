"use client";

import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input } from "../../../../components/ui/Input";
import { formatBytes } from "./truenasTabModel";
import type { TrueNASFilesystemResponse } from "./truenasTabModel";

type TrueNASFilesystemCardProps = {
  pathInput: string;
  filesystem: TrueNASFilesystemResponse | null;
  loading: boolean;
  error: string | null;
  onPathInputChange: (path: string) => void;
  onNavigateUp: () => void;
  onRefresh: () => void;
  onSubmitPath: () => void;
  onOpenDirectory: (path: string) => void;
};

export function TrueNASFilesystemCard({
  pathInput,
  filesystem,
  loading,
  error,
  onPathInputChange,
  onNavigateUp,
  onRefresh,
  onSubmitPath,
  onOpenDirectory,
}: TrueNASFilesystemCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Filesystem Browser</h2>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            onClick={onNavigateUp}
            disabled={!filesystem?.parent_path || loading}
          >
            Up
          </Button>
          <Button size="sm" onClick={onRefresh} disabled={loading}>Refresh</Button>
        </div>
      </div>

      <form
        className="flex items-center gap-2 mb-3"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmitPath();
        }}
      >
        <Input value={pathInput} onChange={(event) => onPathInputChange(event.target.value)} placeholder="/mnt" />
        <Button size="sm" type="submit">Go</Button>
      </form>

      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : loading ? (
        <p className="text-sm text-[var(--muted)]">Loading directory...</p>
      ) : filesystem?.entries && filesystem.entries.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {filesystem.entries.map((entry) => (
            <li key={entry.path} className="py-2.5 flex items-center justify-between gap-3">
              <button
                className={`text-left ${entry.is_directory ? "text-[var(--accent)] hover:underline" : "text-[var(--text)]"}`}
                onClick={() => {
                  if (entry.is_directory) {
                    onOpenDirectory(entry.path);
                  }
                }}
              >
                {entry.is_directory ? `${entry.name}/` : entry.name}
              </button>
              <div className="text-xs text-[var(--muted)] shrink-0 flex items-center gap-3">
                <span>{entry.type}</span>
                <span>{typeof entry.size_bytes === "number" ? formatBytes(entry.size_bytes) : "-"}</span>
                <span>{entry.modified_at ? new Date(entry.modified_at).toLocaleString() : "-"}</span>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-[var(--muted)]">Directory is empty.</p>
      )}
    </Card>
  );
}
