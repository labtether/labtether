"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { GroupParentSelect } from "../../../components/GroupParentSelect";
import { Input } from "../../../components/ui/Input";
import type { Group } from "../../../console/models";
import { Field, PeopleDialog, errorText } from "../users/PeopleDialog";
import { computeGroupDepth, NESTING_DEPTH_WARN } from "./groupOperations";
import { slugFromName } from "./groupListModel";
import type { CreateGroupInput } from "./useGroupMutationActions";

type GroupCreateDialogProps = {
  open: boolean;
  parentId: string;
  groups: Group[];
  onClose: () => void;
  onCreate: (input: CreateGroupInput) => Promise<void>;
};

/** New group: a name, its slug (filled from the name until edited) and the group it sits inside. */
export function GroupCreateDialog({ open, parentId, groups, onClose, onCreate }: GroupCreateDialogProps) {
  const t = useTranslations("groups");
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [parent, setParent] = useState("");
  const [error, setError] = useState<{ name?: string; slug?: string; hub?: string }>({});
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName("");
    setSlug("");
    setSlugEdited(false);
    setParent(parentId);
    setError({});
    setCreating(false);
  }, [open, parentId]);

  const deep = parent ? computeGroupDepth(groups, parent) + 1 >= NESTING_DEPTH_WARN : false;

  const submit = async () => {
    const cleanName = name.trim();
    const cleanSlug = slug.trim().toLowerCase();
    const found = { name: cleanName ? undefined : t("errors.nameEmpty"), slug: cleanSlug ? undefined : t("errors.slugEmpty") };
    setError(found);
    if (found.name || found.slug) return;
    setCreating(true);
    try {
      const input: CreateGroupInput = { name: cleanName, slug: cleanSlug };
      if (parent) input.parent_group_id = parent;
      await onCreate(input);
    } catch (err) {
      setError({ hub: errorText(err, t("errors.createFailed")) });
      setCreating(false);
    }
  };

  return (
    <PeopleDialog
      open={open}
      title={t("create.title")}
      onClose={onClose}
      onSubmit={() => { void submit(); }}
      submitLabel={t("create.submit")}
      tone="main"
      busy={creating}
      error={error.hub}
    >
      <div className="pp-grid2">
        <Field id="pp-g-name" label={t("form.name")} error={error.name}>
          <Input
            id="pp-g-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (!slugEdited) setSlug(slugFromName(e.target.value));
            }}
            placeholder="Home lab"
            autoComplete="off"
            autoFocus
            disabled={creating}
            error={Boolean(error.name)}
            aria-invalid={error.name ? true : undefined}
            aria-describedby={error.name ? "pp-g-name-err" : undefined}
          />
        </Field>
        <Field id="pp-g-slug" label={t("form.slug")} error={error.slug}>
          <Input
            id="pp-g-slug"
            value={slug}
            onChange={(e) => {
              setSlugEdited(true);
              setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-"));
            }}
            placeholder="home-lab"
            maxLength={64}
            autoComplete="off"
            spellCheck={false}
            disabled={creating}
            error={Boolean(error.slug)}
            aria-invalid={error.slug ? true : undefined}
            aria-describedby={error.slug ? "pp-g-slug-err" : undefined}
          />
        </Field>
      </div>
      <GroupParentSelect
        id="pp-g-parent"
        groups={groups}
        value={parent}
        onChange={setParent}
        disabled={creating}
        label={t("form.inside")}
        noneLabel={t("form.topLevel")}
      />
      {deep ? <p className="pp-note">{t("form.deep", { levels: NESTING_DEPTH_WARN })}</p> : null}
    </PeopleDialog>
  );
}
