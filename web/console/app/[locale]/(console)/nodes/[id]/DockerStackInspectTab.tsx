"use client";

import { useTranslations } from "next-intl";
import { useMemo } from "react";
import { ListEmpty, ListError, SectionHead } from "../../../../components/containers/PanelParts";
import { stackMembers } from "../../../../components/containers/stackModel";
import { useHostDocker } from "../../../../components/containers/useHostDocker";

type Props = {
  hostId: string;
  stackName: string;
};

/** What the hub knows about one compose stack and its containers, as JSON. */
export function DockerStackInspectTab({ hostId, stackName }: Props) {
  const t = useTranslations("devices-docker.stacks");
  const { containers, stacks, loading, error, reload } = useHostDocker(hostId, { withStacks: true, intervalMs: 0 });
  const payload = useMemo(() => {
    const stack = stacks.find((s) => s.name.toLowerCase() === stackName.toLowerCase());
    return stack ? { stack, host_id: hostId, matched_containers: stackMembers(stack, containers) } : null;
  }, [stacks, containers, stackName, hostId]);

  if (loading && !payload) return <ListEmpty text={t("loading")} />;
  if (error && !payload) return <ListError text={t("loadFailed")} onRetry={() => void reload()} />;
  if (!payload) return <ListEmpty text={t("notFound")} />;
  return (
    <div className="ct-tab">
      <SectionHead title={t("inspect")} />
      <pre className="ct-sheet ct-json">{JSON.stringify(payload, null, 2)}</pre>
    </div>
  );
}
