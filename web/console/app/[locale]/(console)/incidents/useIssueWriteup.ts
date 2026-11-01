import { useCallback, useState } from "react";
import type { Incident } from "../../../console/models";

export type WriteupFields = { rootCause: string; followUps: string; lessons: string };

const fieldsOf = (issue: Incident): WriteupFields => ({
  rootCause: issue.root_cause ?? "",
  followUps: (issue.action_items ?? []).join("\n"),
  lessons: issue.lessons_learned ?? "",
});

/** Follow-ups are typed one per line; a leading "-", "*" or "•" is dropped, blank lines are not kept. */
export const followUpList = (text: string) =>
  text.split("\n").map((line) => line.replace(/^\s*[-*•]\s*/, "").trim()).filter(Boolean);

/** The write-up form: root cause, follow-ups and lessons learned, as the hub stores them on the issue. */
export function useIssueWriteup() {
  const [fields, setFields] = useState<WriteupFields>({ rootCause: "", followUps: "", lessons: "" });
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  const load = useCallback((issue: Incident) => {
    setFields(fieldsOf(issue));
    setDirty(false);
  }, []);

  const change = useCallback((key: keyof WriteupFields, value: string) => {
    setFields((current) => ({ ...current, [key]: value }));
    setDirty(true);
  }, []);

  /** Saves through onSave (which reports success) and clears the dirty mark when the hub took it. */
  const save = useCallback(async (onSave: (req: Record<string, unknown>) => Promise<boolean>) => {
    setSaving(true);
    try {
      // Empty values are sent too, so a cleared field is cleared on the hub.
      const ok = await onSave({
        root_cause: fields.rootCause.trim(),
        action_items: followUpList(fields.followUps),
        lessons_learned: fields.lessons.trim(),
      });
      if (ok) setDirty(false);
      return ok;
    } finally {
      setSaving(false);
    }
  }, [fields]);

  return { fields, dirty, saving, load, change, save };
}
