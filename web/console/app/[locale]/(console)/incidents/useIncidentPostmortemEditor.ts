import { useCallback, useState } from "react";
import type { Incident } from "../../../console/models";

type UseIncidentPostmortemEditorOptions = {
  updateIncident: (id: string, req: Record<string, unknown>) => Promise<Incident>;
};

export function useIncidentPostmortemEditor({ updateIncident }: UseIncidentPostmortemEditorOptions) {
  const [pmRootCause, setPmRootCause] = useState("");
  const [pmActionItems, setPmActionItems] = useState<string[]>([""]);
  const [pmLessonsLearned, setPmLessonsLearned] = useState("");
  const [pmSaving, setPmSaving] = useState(false);
  const [pmDirty, setPmDirty] = useState(false);

  const loadPostmortemFields = useCallback((inc: Incident) => {
    setPmRootCause(inc.root_cause ?? "");
    setPmActionItems(inc.action_items && inc.action_items.length > 0 ? [...inc.action_items] : [""]);
    setPmLessonsLearned(inc.lessons_learned ?? "");
    setPmDirty(false);
  }, []);

  const handleSavePostmortem = useCallback(async (id: string) => {
    setPmSaving(true);
    try {
      const items = pmActionItems
        .map((item) => item.trim())
        .filter((item) => item !== "");
      const updatedIncident = await updateIncident(id, {
        root_cause: pmRootCause.trim() || undefined,
        action_items: items.length > 0 ? items : undefined,
        lessons_learned: pmLessonsLearned.trim() || undefined,
      });
      setPmDirty(false);
      return updatedIncident;
    } finally {
      setPmSaving(false);
    }
  }, [pmActionItems, pmLessonsLearned, pmRootCause, updateIncident]);

  const handleAddActionItem = useCallback(() => {
    setPmActionItems([...pmActionItems, ""]);
    setPmDirty(true);
  }, [pmActionItems]);

  const handleRemoveActionItem = useCallback((index: number) => {
    setPmActionItems(pmActionItems.filter((_, i) => i !== index));
    setPmDirty(true);
  }, [pmActionItems]);

  const handleUpdateActionItem = useCallback((index: number, value: string) => {
    const next = [...pmActionItems];
    next[index] = value;
    setPmActionItems(next);
    setPmDirty(true);
  }, [pmActionItems]);

  const handleRootCauseChange = useCallback((value: string) => {
    setPmRootCause(value);
    setPmDirty(true);
  }, []);

  const handleLessonsLearnedChange = useCallback((value: string) => {
    setPmLessonsLearned(value);
    setPmDirty(true);
  }, []);

  const resetPmDirty = useCallback(() => {
    setPmDirty(false);
  }, []);

  return {
    pmRootCause,
    pmActionItems,
    pmLessonsLearned,
    pmSaving,
    pmDirty,
    loadPostmortemFields,
    resetPmDirty,
    handleSavePostmortem,
    handleAddActionItem,
    handleRemoveActionItem,
    handleUpdateActionItem,
    handleRootCauseChange,
    handleLessonsLearnedChange,
  };
}
