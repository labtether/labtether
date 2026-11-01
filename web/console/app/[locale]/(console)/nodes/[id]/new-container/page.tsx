"use client";

import { useTranslations } from "next-intl";
import { useParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Link, useRouter } from "../../../../../../i18n/navigation";
import { PageHeader } from "../../../../../components/PageHeader";
import { dockerContainerAssetId, hostKey, nodeHref } from "../../../../../components/containers/containerModel";
import { repoName, runErrors, runParams, type RunErrors, type RunValues } from "../../../../../components/containers/deployModel";
import { DeployCrumbs, Field, HostSelect, dockerTabHref, invalid, useDeployHosts } from "../../../../../components/containers/DeployParts";
import { Button } from "../../../../../components/ui/Button";
import { useToast } from "../../../../../contexts/ToastContext";
import { executeDockerHostAction } from "../../../../../../lib/docker";

const EMPTY: RunValues = { image: "", name: "", ports: "", env: "", command: "" };

/** Run container: pull an image and start one container on a Docker host. */
export default function NewContainerPage() {
  const t = useTranslations("containers.deploy");
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { addToast } = useToast();
  const routeKey = hostKey(decodeURIComponent((params.id ?? "").trim()));
  const hosts = useDeployHosts(routeKey);
  const [host, setHost] = useState(routeKey);
  const [values, setValues] = useState<RunValues>(EMPTY);
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [made, setMade] = useState<{ name: string; href: string } | null>(null);

  const errors: RunErrors = tried ? runErrors(values) : {};
  const msg = (k: keyof RunErrors) => (errors[k] ? t(`errors.${errors[k]}`) : undefined);
  const set = (k: keyof RunValues) => (e: { target: { value: string } }) => setValues((v) => ({ ...v, [k]: e.target.value }));
  const current = hosts.find((h) => h.key === host);
  const back = dockerTabHref(host, "containers");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setTried(true);
    setFailure(null);
    if (Object.keys(runErrors(values)).length) {
      requestAnimationFrame(() => document.querySelector<HTMLElement>(".ct-form [aria-invalid=true]")?.focus());
      return;
    }
    const name = values.name.trim() || repoName(values.image);
    setBusy(true);
    try {
      const result = await executeDockerHostAction(host, "container.create", runParams(values));
      if (result.status !== "succeeded") throw new Error(result.message);
      const id = (result.output ?? "").trim();
      setMade({ name, href: id ? nodeHref(dockerContainerAssetId(host, id)) : back });
      setValues(EMPTY);
      setTried(false);
      addToast("success", t("ranToast", { name, host: current?.label ?? host }));
      setTimeout(() => router.push(back), 900);
    } catch (err) {
      setFailure(err instanceof Error && err.message ? err.message : t("runFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DeployCrumbs host={current} />
      <PageHeader title={t("runTitle")} />
      <form className="ct-form ct-sheet" onSubmit={(e) => void submit(e)} noValidate>
        <div className="ct-grid three">
          <Field id="ct-host" label={t("host")}>
            <HostSelect id="ct-host" hosts={hosts} value={host} onChange={setHost} />
          </Field>
          <Field id="ct-img" label={t("image")} error={msg("image")}>
            <input className="ct-input mono" id="ct-img" value={values.image} onChange={set("image")} placeholder="nginx:1.29" autoComplete="off" spellCheck={false} autoFocus {...invalid("ct-img", msg("image"))} />
          </Field>
          <Field id="ct-name" label={t("name")} error={msg("name")}>
            <input className="ct-input mono" id="ct-name" value={values.name} onChange={set("name")} placeholder={repoName(values.image) || "nginx"} autoComplete="off" spellCheck={false} {...invalid("ct-name", msg("name"))} />
          </Field>
        </div>
        <div className="ct-grid two">
          <Field id="ct-ports" label={t("ports")} error={msg("ports")} hint={t("portsHint")}>
            <textarea className="ct-input mono ct-ta" id="ct-ports" rows={3} value={values.ports} onChange={set("ports")} placeholder={"8080:80\n5353:5353/udp"} spellCheck={false} {...invalid("ct-ports", msg("ports"))} />
          </Field>
          <Field id="ct-env" label={t("environment")} error={msg("env")} hint={t("envHint")}>
            <textarea className="ct-input mono ct-ta" id="ct-env" rows={3} value={values.env} onChange={set("env")} placeholder={"TZ=Etc/UTC\nPUID=1000"} spellCheck={false} {...invalid("ct-env", msg("env"))} />
          </Field>
        </div>
        <Field id="ct-cmd" label={t("command")} wide>
          <input className="ct-input mono" id="ct-cmd" value={values.command} onChange={set("command")} placeholder="python -m http.server 8080" autoComplete="off" spellCheck={false} />
        </Field>
        {failure ? <p className="ct-err" role="alert">{t("runFailedWith", { reason: failure })}</p> : null}
        {made && !failure ? (
          <p className="ct-done" role="status">
            {t("ranLine", { name: made.name })} <Link href={made.href}>{t("open")}</Link>
          </p>
        ) : null}
        <div className="ct-form-foot">
          <Link href={back} className="ct-btn">{t("cancel")}</Link>
          <Button type="submit" variant="primary" loading={busy} disabled={!host}>{t("run")}</Button>
        </div>
      </form>
    </>
  );
}
