import { useState, type KeyboardEvent } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import type { Issue } from "./issuesModel";

/**
 * The issue's notes. The hub keeps them as one text, a note per added line, so a new note is added at its end.
 * ⌘ Enter (Ctrl Enter) adds the note from the box.
 */
export function IssueNotes({ issue, canWrite, onSave }: { issue: Issue; canWrite: boolean; onSave: (req: Record<string, unknown>) => Promise<boolean> }) {
  const t = useTranslations("incidents.notes");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const notes = issue.summary?.trim() ?? "";

  const add = async () => {
    const note = text.trim();
    if (!note || busy) return;
    setBusy(true);
    const ok = await onSave({ summary: (issue.summary ? `${issue.summary}\n` : "") + note });
    setBusy(false);
    if (ok) setText("");
  };
  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void add(); }
  };

  return (
    <section className="iu-sec" aria-labelledby="iu-notes-h">
      <div className="iu-sh"><h2 id="iu-notes-h">{t("title")}</h2></div>
      {notes ? <div className="iu-note">{notes}</div> : !canWrite ? <p className="iu-quiet">{t("empty")}</p> : null}
      {canWrite ? (
        <div className="iu-add">
          <label className="iu-sr" htmlFor="iu-note-box">{t("label")}</label>
          <textarea
            id="iu-note-box"
            className="iu-input iu-ta"
            rows={2}
            placeholder={t("placeholder")}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKey}
          />
          <div className="iu-add-b">
            <Button disabled={busy || !text.trim()} onClick={() => void add()}>{t("add")}</Button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
