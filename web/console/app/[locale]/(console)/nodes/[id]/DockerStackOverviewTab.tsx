"use client";

import { useTranslations } from "next-intl";
import { useMemo } from "react";
import { Link } from "../../../../../i18n/navigation";
import { dockerContainerAssetId, dockerHostAssetId, nodeHref } from "../../../../components/containers/containerModel";
import { KV, ListEmpty, ListError, SectionHead } from "../../../../components/containers/PanelParts";
import { useConfirm } from "../../../../components/containers/RowMenu";
import { stackMembers, stackState } from "../../../../components/containers/stackModel";
import { useHostDocker } from "../../../../components/containers/useHostDocker";
import { useStackAction } from "../../../../components/containers/useStackAction";
import { Button } from "../../../../components/ui/Button";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasWriteRole } from "../../../../lib/roles";

type Props = {
  hostId: string;
  stackName: string;
  stackAssetId: string;
};

/** A compose stack's page: its state, host, compose file and containers, with Start, Restart and Down…. */
export function DockerStackOverviewTab({ hostId, stackName, stackAssetId }: Props) {
  const t = useTranslations("devices-docker.stacks");
  const { user } = useAuth();
  const { containers, stacks, loading, error, reload } = useHostDocker(hostId, { withStacks: true });
  const stack = useMemo(() => stacks.find((s) => s.name.toLowerCase() === stackName.toLowerCase()) ?? null, [stacks, stackName]);
  const members = useMemo(() => (stack ? stackMembers(stack, containers) : []), [stack, containers]);
  const run = useStackAction(stackName, stackAssetId, () => void reload());
  const [ask, dialog] = useConfirm();

  if (loading && !stack) return <ListEmpty text={t("loading")} />;
  if (error && !stack) return <ListError text={t("loadFailed")} onRetry={() => void reload()} />;
  if (!stack) return <ListEmpty text={t("notFound")} />;

  const st = stackState(members, stack.status);
  const word = st.word === "partial" ? t("partial", { running: st.running, total: st.total }) : st.word ? t(st.word) : t("running");
  const acts = hasWriteRole(user?.role) && st.tone !== "busy" ? (
    <>
      {st.running < st.total || !st.total ? <Button size="sm" onClick={() => void run("stack.up")}>{t("start")}</Button> : null}
      {st.running ? <Button size="sm" onClick={() => void run("stack.restart")}>{t("restart")}</Button> : null}
      <Button size="sm" variant="danger" onClick={() => ask({ title: t("downTitle", { name: stack.name }), body: t("downBody"), confirm: t("downConfirm"), danger: true, run: () => run("stack.down") })}>
        {t("down")}
      </Button>
    </>
  ) : null;

  return (
    <div className="ct-tab">
      <section aria-labelledby="ct-stk">
        <SectionHead id="ct-stk" title={t("details")}>{acts}</SectionHead>
        <dl className="ct-sheet ct-kvs">
          <KV k={t("state")}>
            {st.tone ? <i className={`ct-dot ${st.tone}`} aria-hidden="true" /> : null}
            <span>{word}</span>
          </KV>
          <KV k={t("host")}><Link href={nodeHref(dockerHostAssetId(hostId))} className="ct-lnk">{hostId}</Link></KV>
          <KV k={t("composeFile")} mono><span className="ct-wrap">{stack.config_file || "—"}</span></KV>
          <KV k={t("containers")}>
            {members.length ? (
              <span className="ct-names wrap">
                {members.map((c) => <Link key={c.id} href={nodeHref(dockerContainerAssetId(hostId, c.id))}>{c.name}</Link>)}
              </span>
            ) : <span className="ct-quiet">{t("noContainers")}</span>}
          </KV>
        </dl>
      </section>
      {dialog}
    </div>
  );
}
