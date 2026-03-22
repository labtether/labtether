"use client";

import { X, CheckCircle2, AlertCircle, Loader2 } from "lucide-react";
import type { FileTransfer } from "./fileTransferClient";
import { formatSize } from "./fileWorkspaceUtils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface TransferProgressBarProps {
  transfers: FileTransfer[];
  onCancel: (transferId: string) => void;
  onClearCompleted: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function progressPercent(t: FileTransfer): number {
  if (!t.file_size || t.file_size === 0) return 0;
  return Math.min(100, Math.round((t.bytes_transferred / t.file_size) * 100));
}

function isActive(t: FileTransfer): boolean {
  return t.status === "pending" || t.status === "in_progress";
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function TransferRow({
  transfer,
  onCancel,
}: {
  transfer: FileTransfer;
  onCancel: (id: string) => void;
}) {
  const pct = progressPercent(transfer);
  const active = isActive(transfer);
  const failed = transfer.status === "failed";
  const completed = transfer.status === "completed";

  return (
    <div className="flex items-center gap-3 px-3 py-2 min-w-0">
      {/* Status icon */}
      <div className="flex-shrink-0 w-4 h-4">
        {active && (
          <Loader2
            className="w-4 h-4 text-[var(--accent)]"
            style={{ animation: "spin 1s linear infinite" }}
          />
        )}
        {completed && <CheckCircle2 className="w-4 h-4 text-emerald-400" />}
        {failed && <AlertCircle className="w-4 h-4 text-red-400" />}
      </div>

      {/* File name + progress */}
      <div className="flex-1 min-w-0 flex flex-col gap-0.5">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-xs text-[var(--text)] truncate">
            {transfer.file_name}
          </span>
          {active && transfer.file_size != null && transfer.file_size > 0 && (
            <span className="text-[10px] text-[var(--muted)] tabular-nums flex-shrink-0">
              {formatSize(transfer.bytes_transferred)} / {formatSize(transfer.file_size)}
            </span>
          )}
          {active && (
            <span className="text-[10px] text-[var(--accent)] tabular-nums flex-shrink-0">
              {pct}%
            </span>
          )}
          {failed && transfer.error && (
            <span className="text-[10px] text-red-400 truncate flex-shrink-0">
              {transfer.error}
            </span>
          )}
        </div>

        {/* Progress bar for active transfers */}
        {active && (
          <div className="h-1 w-full rounded-full bg-[var(--surface)] overflow-hidden">
            <div
              className="h-full rounded-full bg-[var(--accent)] transition-[width] duration-300 ease-out"
              style={{ width: `${pct}%` }}
            />
          </div>
        )}
      </div>

      {/* Cancel button for active transfers */}
      {active && (
        <button
          className="flex-shrink-0 p-1 rounded text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors cursor-pointer bg-transparent border-none"
          onClick={() => onCancel(transfer.id)}
          title="Cancel transfer"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function TransferProgressBar({
  transfers,
  onCancel,
  onClearCompleted,
}: TransferProgressBarProps) {
  if (transfers.length === 0) return null;

  const hasCompleted = transfers.some(
    (t) => t.status === "completed" || t.status === "failed",
  );

  return (
    <div className="sticky bottom-0 z-10 border-t border-[var(--line)] bg-[var(--panel)] backdrop-blur-sm">
      {/* Header row */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-[var(--line)]">
        <span className="text-[10px] font-medium text-[var(--muted)] uppercase tracking-wider">
          Transfers ({transfers.length})
        </span>
        {hasCompleted && (
          <button
            className="text-[10px] text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer bg-transparent border-none"
            onClick={onClearCompleted}
          >
            Clear completed
          </button>
        )}
      </div>

      {/* Transfer list */}
      <div className="max-h-40 overflow-y-auto divide-y divide-[var(--line)]">
        {transfers.map((transfer) => (
          <TransferRow
            key={transfer.id}
            transfer={transfer}
            onCancel={onCancel}
          />
        ))}
      </div>
    </div>
  );
}
