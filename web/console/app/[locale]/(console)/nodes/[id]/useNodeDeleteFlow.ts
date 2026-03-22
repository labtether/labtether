"use client";

import { useCallback, useState } from "react";
import { useRouter } from "../../../../../i18n/navigation";

type UseNodeDeleteFlowArgs = {
  nodeId: string;
};

type UseNodeDeleteFlowResult = {
  showDeleteConfirm: boolean;
  deleteConfirmInput: string;
  deleting: boolean;
  deleteError: string | null;
  openDeleteConfirm: () => void;
  setDeleteConfirmInput: (value: string) => void;
  cancelDeleteConfirm: () => void;
  confirmDelete: () => Promise<void>;
};

export function useNodeDeleteFlow({ nodeId }: UseNodeDeleteFlowArgs): UseNodeDeleteFlowResult {
  const router = useRouter();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteConfirmInput, setDeleteConfirmInput] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const openDeleteConfirm = useCallback(() => {
    setDeleteError(null);
    setShowDeleteConfirm(true);
  }, []);

  const cancelDeleteConfirm = useCallback(() => {
    setShowDeleteConfirm(false);
    setDeleteConfirmInput("");
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!nodeId) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(nodeId)}`, { method: "DELETE" });
      const data = (await res.json()) as { deleted?: boolean; error?: string };
      if (!res.ok || !data.deleted) {
        throw new Error(data.error || `Delete failed (${res.status})`);
      }
      router.push("/nodes");
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Delete failed");
      setDeleting(false);
    }
  }, [nodeId, router]);

  return {
    showDeleteConfirm,
    deleteConfirmInput,
    deleting,
    deleteError,
    openDeleteConfirm,
    setDeleteConfirmInput,
    cancelDeleteConfirm,
    confirmDelete,
  };
}
