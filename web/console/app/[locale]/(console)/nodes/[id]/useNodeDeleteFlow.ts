"use client";

import { useCallback, useState } from "react";
import { useRouter } from "../../../../../i18n/navigation";
import { useStatusControls } from "../../../../contexts/StatusContext";

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
  const { fetchStatus } = useStatusControls();
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
      setShowDeleteConfirm(false);
      setDeleting(false);
      // The Devices list drops the device now instead of at the next poll.
      void fetchStatus();
      router.push("/nodes");
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Delete failed");
      setDeleting(false);
    }
  }, [fetchStatus, nodeId, router]);

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
