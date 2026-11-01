"use client";

import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useMemo, type ReactNode } from "react";
import { Link } from "../../../../../i18n/navigation";
import { copyText } from "../../../../components/containers/containerActions";
import { dockerContainerAssetId, dockerHostAssetId, dockerStackAssetId, nodeHref } from "../../../../components/containers/containerModel";
import { Bar, LinkButton, ListEmpty, ListError, ListSkeleton } from "../../../../components/containers/PanelParts";
import { RowMenu, useConfirm, type MenuEntry } from "../../../../components/containers/RowMenu";
import { stackFolder, stackMembers, stackRoot, stackState } from "../../../../components/containers/stackModel";
import { useHostDocker } from "../../../../components/containers/useHostDocker";
import { useStackAction } from "../../../../components/containers/useStackAction";
import { useAuth } from "../../../../contexts/AuthContext";
import { useToast } from "../../../../contexts/ToastContext";
import { hasWriteRole } from "../../../../lib/roles";
import type { DockerContainer, DockerStack } from "../../../../../lib/docker";

function StackRow({ hostId, stack, members, root, onChanged }: { hostId: string; stack: DockerStack; members: DockerContainer[]; root: string; onChanged: () => void }) {
  const t = useTranslations("devices-docker.stacks");
  const tc = useTranslations("common");
  const { user } = useAuth();
  const { addToast } = useToast();
  const [ask, dialog] = useConfirm();
  const assetId = dockerStackAssetId(hostId, stack.name);
  const run = useStackAction(stack.name, assetId, onChanged);
  const st = stackState(members, stack.status);
  const folder = stackFolder(stack);
  const word = st.word === "partial" ? t("partial", { running: st.running, total: st.total }) : st.word ? t(st.word) : "";
  const grey = word || (folder && (!root || !folder.startsWith(`${root}/`)) ? folder : "");

  const items: MenuEntry[] = [{ label: t("open"), href: nodeHref(assetId) }];
  if (hasWriteRole(user?.role) && st.tone !== "busy") {
    items.push("sep");
    if (st.running < st.total || !st.total) items.push({ label: t("start"), onSelect: () => void run("stack.up") });
    if (st.running) items.push({ label: t("restart"), onSelect: () => void run("stack.restart") });
    items.push({
      label: t("down"),
      danger: true,
      onSelect: () => ask({ title: t("downTitle", { name: stack.name }), body: t("downBody"), confirm: t("downConfirm"), danger: true, run: () => run("stack.down") }),
    });
  }
  if (stack.config_file) {
    items.push("sep", { label: t("copyPath"), onSelect: () => void copyText(stack.config_file).then((ok) => { if (ok) addToast("success", tc("copied")); }) });
  }

  return (
    <div className="ct-row sk" role="row">
      <span className="ct-lead" aria-hidden="true">{st.tone ? <i className={`ct-dot ${st.tone}`} /> : null}</span>
      <span className="ct-main" role="cell">
        <Link href={nodeHref(assetId)} className="ct-nm">{stack.name}</Link>
        {grey ? <span className={`ct-grey${word ? ` w-${st.tone || "off"}` : " mono"}`}>{grey}</span> : null}
      </span>
      <span className="ct-names" role="cell">
        {members.map((c) => <Link key={c.id} href={nodeHref(dockerContainerAssetId(hostId, c.id))}>{c.name}</Link>)}
      </span>
      <span className="ct-act"><RowMenu label={t("menu", { name: stack.name })} items={items} /></span>
      {dialog}
    </div>
  );
}

/** A Docker host's compose stacks: state, their containers, and Start / Restart / Down… in each row's ⋯. */
export function DockerStacksTab({ hostId, bar }: { hostId: string; bar?: ReactNode }) {
  const t = useTranslations("devices-docker.stacks");
  const tc = useTranslations("containers");
  const { user } = useAuth();
  const { containers, stacks, loading, error, reload } = useHostDocker(hostId, { withStacks: true, intervalMs: 15_000 });
  const sorted = useMemo(() => [...stacks].sort((a, b) => a.name.localeCompare(b.name)), [stacks]);
  const root = useMemo(() => stackRoot(stacks), [stacks]);

  const deploy = hasWriteRole(user?.role) ? (
    <LinkButton href={`${nodeHref(dockerHostAssetId(hostId))}/new-compose`}>
      <Plus size={14} aria-hidden="true" />
      {tc("deployStack")}
    </LinkButton>
  ) : null;

  let body: ReactNode;
  if (loading && !stacks.length) body = <ListSkeleton />;
  else if (error && !stacks.length) body = <ListError text={t("loadFailed")} onRetry={() => void reload()} />;
  else if (!sorted.length) body = <ListEmpty text={t("none")} />;
  else body = (
    <div className="ct-list" role="table" aria-label={t("label")}>
      <div className="ct-row sk ct-cols" role="row">
        <span className="ct-lead" />
        <span>{root ? t("columnIn", { root }) : t("column")}</span>
        <span className="ct-names">{t("containers")}</span>
        <span />
      </div>
      {sorted.map((s) => <StackRow key={s.name} hostId={hostId} stack={s} members={stackMembers(s, containers)} root={root} onChanged={() => void reload()} />)}
    </div>
  );

  return (
    <>
      <Bar left={bar} right={deploy} />
      {body}
    </>
  );
}
