"use client";

import { AlertTriangle, Download, X, FileText } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";

type DeleteConfirmOverlayProps = {
  name: string | null;
  onCancel: () => void;
  onConfirm: () => void;
};

export function DeleteConfirmOverlay({ name, onCancel, onConfirm }: DeleteConfirmOverlayProps) {
  if (!name) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onCancel}>
      <div onClick={(event) => event.stopPropagation()}>
        <Card className="w-full max-w-md mx-4 space-y-4">
          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-[var(--bad-glow)]">
              <AlertTriangle className="w-5 h-5 text-[var(--bad)]" strokeWidth={1.5} />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="text-sm font-semibold text-[var(--text)] mb-1">Delete &ldquo;{name}&rdquo;?</h3>
              <p className="text-sm text-[var(--muted)] leading-relaxed">This action cannot be undone. The file or directory will be permanently removed from the device.</p>
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <Button size="sm" onClick={onCancel}>Cancel</Button>
            <Button size="sm" variant="danger" onClick={onConfirm}>Delete</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

type BatchDeleteConfirmOverlayProps = {
  open: boolean;
  selectionCount: number;
  onCancel: () => void;
  onConfirm: () => void;
};

export function BatchDeleteConfirmOverlay({
  open,
  selectionCount,
  onCancel,
  onConfirm,
}: BatchDeleteConfirmOverlayProps) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onCancel}>
      <div onClick={(event) => event.stopPropagation()}>
        <Card className="w-full max-w-md mx-4 space-y-4">
          <div className="flex items-start gap-3">
            <div className="p-2 rounded-lg bg-[var(--bad-glow)]">
              <AlertTriangle className="w-5 h-5 text-[var(--bad)]" strokeWidth={1.5} />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="text-sm font-semibold text-[var(--text)] mb-1">
                Delete {selectionCount} item{selectionCount !== 1 ? "s" : ""}?
              </h3>
              <p className="text-sm text-[var(--muted)] leading-relaxed">This action cannot be undone. The selected files and directories will be permanently removed.</p>
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <Button size="sm" onClick={onCancel}>Cancel</Button>
            <Button size="sm" variant="danger" onClick={onConfirm}>
              Delete {selectionCount} item{selectionCount !== 1 ? "s" : ""}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

type PreviewContent = {
  name: string;
  content: string;
};

type TextPreviewOverlayProps = {
  previewContent: PreviewContent | null;
  currentPath: string;
  onClose: () => void;
  onDownload: (fullPath: string) => void;
};

export function TextPreviewOverlay({
  previewContent,
  currentPath,
  onClose,
  onDownload,
}: TextPreviewOverlayProps) {
  if (!previewContent) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div className="w-full max-w-3xl mx-4" onClick={(event) => event.stopPropagation()}>
        <Card variant="flush" className="max-h-[80vh] flex flex-col">
          <div className="flex items-center gap-3 p-4 border-b border-[var(--line)]">
            <FileText className="w-4 h-4 text-[var(--muted)] flex-shrink-0" strokeWidth={1.5} />
            <span className="flex-1 font-medium text-sm truncate text-[var(--text)]">{previewContent.name}</span>
            <div className="flex items-center gap-1.5">
              <Button
                size="sm"
                onClick={() => {
                  const fullPath = currentPath === "/" ? `/${previewContent.name}` : `${currentPath}/${previewContent.name}`;
                  onDownload(fullPath);
                }}
              >
                <Download className="w-3.5 h-3.5" />
                <span>Download</span>
              </Button>
              <button
                className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
                onClick={onClose}
                title="Close"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>
          <pre className="flex-1 overflow-auto p-4 text-xs font-mono text-[var(--text)] whitespace-pre-wrap leading-relaxed">{previewContent.content}</pre>
        </Card>
      </div>
    </div>
  );
}
