import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import { hasWriteup, type Issue } from "./issuesModel";
import { useIssueWriteup, type WriteupFields } from "./useIssueWriteup";

type Props = {
  issue: Issue;
  canWrite: boolean;
  /** The save is the page's one main button while the issue is resolved and the form is open. */
  primary: boolean;
  onSave: (req: Record<string, unknown>) => Promise<boolean>;
};

/**
 * After an issue is resolved: what caused it, what to follow up and what was learned. Shown as text once saved
 * (with Edit); the form opens on its own while nothing is written yet.
 */
export function IssueWriteup({ issue, canWrite, primary, onSave }: Props) {
  const t = useTranslations("incidents.writeup");
  const saved = hasWriteup(issue);
  const { fields, dirty, saving, load, change, save } = useIssueWriteup();
  const [editing, setEditing] = useState(!saved);

  // Follow the hub's copy until someone starts typing.
  useEffect(() => { if (!editing || !dirty) load(issue); }, [editing, dirty, issue, load]);

  if (!editing && saved) {
    const followUps = (issue.action_items ?? []).filter((item) => item.trim());
    return (
      <section className="iu-sec" aria-labelledby="iu-wu-h">
        <div className="iu-sh">
          <h2 id="iu-wu-h">{t("title")}</h2>
          {canWrite ? <button type="button" className="iu-sh-link" onClick={() => setEditing(true)}>{t("edit")}</button> : null}
        </div>
        <dl className="iu-wud">
          {issue.root_cause?.trim() ? <div><dt>{t("rootCause")}</dt><dd>{issue.root_cause}</dd></div> : null}
          {followUps.length ? <div><dt>{t("followUpsTitle")}</dt><dd><ul>{followUps.map((item, i) => <li key={i}>{item}</li>)}</ul></dd></div> : null}
          {issue.lessons_learned?.trim() ? <div><dt>{t("lessons")}</dt><dd>{issue.lessons_learned}</dd></div> : null}
        </dl>
      </section>
    );
  }

  if (!canWrite) {
    return (
      <section className="iu-sec" aria-labelledby="iu-wu-h">
        <div className="iu-sh"><h2 id="iu-wu-h">{t("title")}</h2></div>
        <p className="iu-quiet">{t("none")}</p>
      </section>
    );
  }

  const area = (key: keyof WriteupFields, label: string, rows: number) => (
    <div className="iu-field">
      <label htmlFor={`iu-wu-${key}`}>{label}</label>
      <textarea id={`iu-wu-${key}`} className="iu-input iu-ta" rows={rows} value={fields[key]} onChange={(e) => change(key, e.target.value)} />
    </div>
  );

  return (
    <section className="iu-sec" aria-labelledby="iu-wu-h">
      <div className="iu-sh"><h2 id="iu-wu-h">{t("title")}</h2></div>
      <div className="iu-sheet iu-wu" role="group" aria-labelledby="iu-wu-h">
        {area("rootCause", t("rootCause"), 3)}
        {area("followUps", t("followUps"), 2)}
        {area("lessons", t("lessons"), 2)}
        <div className="iu-sheet-foot">
          {saved ? <Button variant="ghost" onClick={() => { load(issue); setEditing(false); }}>{t("cancel")}</Button> : null}
          <Button
            variant={primary ? "primary" : "secondary"}
            disabled={saving || !dirty}
            onClick={() => { void save(onSave).then((ok) => { if (ok) setEditing(false); }); }}
          >
            {saving ? t("saving") : t("save")}
          </Button>
        </div>
      </div>
    </section>
  );
}
