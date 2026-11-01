"use client";

import { useTranslations } from "next-intl";
import type { SavedAction } from "../../../hooks/useSavedActions";
import { stepSucceeded, type SessionRun } from "./actionsModel";

/**
 * Under an opened action: its steps (device and command), and the result of the run started here, each step with its
 * exit code (only when the device reported one) and its output.
 */
export function SavedActionPanel({
  action,
  run,
  deviceName,
  loadError,
}: {
  action: SavedAction;
  run?: SessionRun;
  deviceName: (id: string) => string;
  loadError?: string | null;
}) {
  const t = useTranslations("actions.panel");
  const many = action.steps.length > 1;

  return (
    <div className={`au-panel${run ? " two" : ""}`} id={`au-panel-${action.id}`}>
      <div className="au-pc">
        {action.description ? <p className="au-desc">{action.description}</p> : null}
        <h3 className="au-cap">{many ? t("steps") : t("command")}</h3>
        <ol className="au-steps-ro" aria-label={t("steps")}>
          {action.steps.map((step, index) => (
            <li key={`${step.target}-${index}`}>
              <span className="au-std">
                {many ? <span className="au-sn num">{index + 1}</span> : null}
                <b>{deviceName(step.target)}</b>
                {many && step.name ? <span className="au-stn">{step.name}</span> : null}
              </span>
              <pre className="au-code">{step.command}</pre>
            </li>
          ))}
        </ol>
        {loadError ? <p role="alert" className="au-err">{loadError}</p> : null}
      </div>
      {run ? (
        <div className="au-pc">
          <h3 className="au-cap">{t("lastRun")}</h3>
          {run.state === "running" ? <p className="au-word busy" role="status">{t("running")}</p> : null}
          {run.state === "refused" ? <p role="alert" className="au-err">{run.message}</p> : null}
          {run.state === "done" ? (
            <ol className="au-results" aria-label={t("lastRun")}>
              {run.result.steps.map((step, index) => {
                const ok = stepSucceeded(step);
                const code = !step.error && typeof step.exit_code === "number" ? step.exit_code : null;
                return (
                  <li key={`${step.target}-${index}`}>
                    <span className="au-rh">
                      <b>{many ? `${index + 1}. ` : ""}{deviceName(step.target)}</b>
                      <span className={ok ? "au-ok" : "au-bad"}>
                        {code !== null ? t("exit", { code }) : t(ok ? "succeeded" : "failed")}
                      </span>
                      {typeof step.duration_ms === "number" ? <span className="num">{t("duration", { ms: step.duration_ms })}</span> : null}
                    </span>
                    {step.message ? <p className="au-err">{step.message}</p> : null}
                    {typeof step.output === "string"
                      ? <pre className="au-code au-out">{step.output || t("noOutput")}</pre>
                      : null}
                  </li>
                );
              })}
            </ol>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
