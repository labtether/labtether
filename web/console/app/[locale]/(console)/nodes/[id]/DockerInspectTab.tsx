"use client";

import { Copy } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { copyText } from "../../../../components/containers/containerActions";
import { dockerHostAssetId, dockerStackAssetId, nodeHref, parseDockerStatus, parsePorts } from "../../../../components/containers/containerModel";
import { KV, ListEmpty, ListError, SectionHead } from "../../../../components/containers/PanelParts";
import { useSpan, useStateWord } from "../../../../components/containers/useContainerWords";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { useToast } from "../../../../contexts/ToastContext";
import { fetchDockerContainerDetail, type DockerContainer } from "../../../../../lib/docker";

function CopyButton({ value, label }: { value: string; label: string }) {
  const tc = useTranslations("common");
  const { addToast } = useToast();
  return (
    <button type="button" className="ct-copy" aria-label={label} onClick={() => void copyText(value).then((ok) => { if (ok) addToast("success", tc("copied")); })}>
      <Copy aria-hidden="true" strokeWidth={2} />
    </button>
  );
}

/** What docker inspect reports for one container: Details, then its labels. */
export function DockerInspectTab({ containerId }: { containerId: string }) {
  const t = useTranslations("devices-docker.inspect");
  const locale = useLocale();
  const word = useStateWord();
  const span = useSpan();
  const status = useFastStatus();
  const [container, setContainer] = useState<DockerContainer | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    fetchDockerContainerDetail(containerId)
      .then((data) => { if (live) { setContainer(data); setError(null); } })
      .catch((err: unknown) => { if (live) setError(err instanceof Error ? err.message : "error"); })
      .finally(() => { if (live) setLoading(false); });
    return () => { live = false; };
  }, [containerId]);

  const asset = useMemo(() => status?.assets?.find((a) => a.id === containerId), [status?.assets, containerId]);
  const agent = asset?.metadata?.agent_id ?? "";

  if (loading) return <ListEmpty text={t("loading")} />;
  if (error) return <ListError text={t("loadFailed")} />;
  if (!container) return <ListEmpty text={t("notFound")} />;

  const st = parseDockerStatus(container.state, container.status);
  const stack = container.stack_name || asset?.metadata?.stack || "";
  const ports = parsePorts(container.ports);
  const labels = Object.entries(container.labels ?? {}).sort(([a], [b]) => a.localeCompare(b));
  const created = new Date(container.created);
  const createdText = Number.isFinite(created.getTime())
    ? created.toLocaleString(locale, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
    : "";

  return (
    <div className="ct-tab">
      <section aria-labelledby="ct-det">
        <SectionHead id="ct-det" title={t("details")} />
        <dl className="ct-sheet ct-kvs">
          <KV k={t("image")} mono>
            <span className="ct-wrap">{container.image}</span>
            <CopyButton value={container.image} label={t("copyImage")} />
          </KV>
          <KV k={t("state")}>
            <span className={st.kind === "running" && !st.health ? "" : `ct-w-${st.kind === "dead" ? "bad" : "off"}`}>{word(st, true)}</span>
            {st.up ? <span className="ct-quiet">{t("upFor", { time: span(st.up) })}</span> : null}
          </KV>
          {agent ? <KV k={t("host")}><Link href={nodeHref(dockerHostAssetId(agent))} className="ct-lnk">{agent}</Link></KV> : null}
          {stack ? (
            <KV k={t("stack")}>{agent ? <Link href={nodeHref(dockerStackAssetId(agent, stack))} className="ct-lnk">{stack}</Link> : stack}</KV>
          ) : null}
          <KV k={t("ports")}>
            {ports.length ? (
              <span className="ct-col">
                {ports.map((p) => <span key={`${p.host}/${p.inside}/${p.proto}`} className="mono">{p.host}<span className="ct-quiet"> → {p.inside}{p.proto === "tcp" ? "" : `/${p.proto}`}</span></span>)}
              </span>
            ) : <span className="ct-quiet">{t("noPorts")}</span>}
          </KV>
          {createdText ? <KV k={t("created")}>{createdText}</KV> : null}
          <KV k={t("containerId")} mono>
            <span className="ct-wrap" title={container.id}>{container.id.slice(0, 12)}</span>
            <CopyButton value={container.id} label={t("copyId")} />
          </KV>
          {container.status ? <KV k={t("status")}><span className="ct-quiet">{container.status}</span></KV> : null}
        </dl>
      </section>
      {labels.length ? (
        <section className="ct-sec" aria-labelledby="ct-lbl">
          <SectionHead id="ct-lbl" title={t("labels")} />
          <dl className="ct-sheet ct-pairs">
            {labels.map(([k, v]) => (
              <div key={k}>
                <dt className="mono">{k}</dt>
                <dd className="mono">{v}</dd>
              </div>
            ))}
          </dl>
        </section>
      ) : null}
    </div>
  );
}
