import type { ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import type { Asset } from "../../../console/models";
import { assetFreshness } from "../nodes/nodesPageUtils";
import { fullWhen, shortWhen } from "./issuesModel";

/** A device's tone and problem word: none while it is fine. */
export function deviceState(asset: Asset): { tone: "bad" | "warn" | ""; problem: "offline" | "notResponding" | "noCheckIn" | null } {
  const freshness = assetFreshness(asset);
  if (freshness === "offline") return { tone: "bad", problem: "offline" };
  if (freshness === "unresponsive") return { tone: "warn", problem: "notResponding" };
  if (freshness === "unknown") return { tone: "warn", problem: "noCheckIn" };
  return { tone: "", problem: null };
}

/**
 * One device in a hanging list: a dot only when something is wrong, its name (opens the device), then its status: the
 * problem in its tone with the last check-in time ("Not responding · since 21:12"), or what it runs. A target the hub
 * no longer has as a device keeps its id and says so. extra: a second line under the row; right: the row's last column.
 */
export function DeviceLine({ id, asset, extra, right }: { id: string; asset?: Asset; extra?: ReactNode; right?: ReactNode }) {
  const t = useTranslations("incidents.device");
  const locale = useLocale();
  if (!asset) {
    return (
      <div className="iu-row iu-dev">
        <span className="iu-nm plain">{id}</span>
        <span className="iu-dst">{t("missing")}</span>
        <span className="iu-dev-r">{right}</span>
        {extra ? <div className="iu-dev-x">{extra}</div> : null}
      </div>
    );
  }
  const { tone, problem } = deviceState(asset);
  const meta = asset.metadata ?? {};
  const since = problem && problem !== "noCheckIn" ? shortWhen(asset.last_seen_at, locale) : "";
  return (
    <div className="iu-row iu-dev">
      <span className="iu-lead" aria-hidden="true">{tone ? <i className={`iu-dot ${tone}`} /> : null}</span>
      <Link href={`/nodes/${encodeURIComponent(asset.id)}`} className="iu-nm">{asset.name || asset.id}</Link>
      {problem ? (
        <span className={`iu-dst ${tone}`} title={since ? fullWhen(asset.last_seen_at, locale) : undefined}>
          {t(problem)}
          {since ? <span className="iu-c3"> · {t("since", { time: since })}</span> : null}
        </span>
      ) : (
        <span className="iu-dst">{meta.os_pretty_name || meta.os_name || ""}</span>
      )}
      <span className="iu-dev-r">{right}</span>
      {extra ? <div className="iu-dev-x">{extra}</div> : null}
    </div>
  );
}
