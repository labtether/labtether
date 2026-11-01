"use client";

import { useTranslations } from "next-intl";
import { useParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Link, useRouter } from "../../../../../../i18n/navigation";
import { PageHeader } from "../../../../../components/PageHeader";
import { dockerStackAssetId, hostKey, nodeHref } from "../../../../../components/containers/containerModel";
import { COMPOSE_SAMPLE, stackErrors, type StackErrors, type StackValues } from "../../../../../components/containers/deployModel";
import { DeployCrumbs, Field, HostSelect, dockerTabHref, invalid, useDeployHosts } from "../../../../../components/containers/DeployParts";
import { Button } from "../../../../../components/ui/Button";
import { useToast } from "../../../../../contexts/ToastContext";
import { executeDockerHostAction } from "../../../../../../lib/docker";

/** Deploy stack: a compose project from a compose.yaml, on a Docker host. */
export default function NewComposePage() {
  const t = useTranslations("containers.deploy");
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { addToast } = useToast();
  const routeKey = hostKey(decodeURIComponent((params.id ?? "").trim()));
  const hosts = useDeployHosts(routeKey);
  const [host, setHost] = useState(routeKey);
  const [values, setValues] = useState<StackValues>({ name: "", compose: COMPOSE_SAMPLE });
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [made, setMade] = useState<{ name: string; href: string } | null>(null);

  const errors: StackErrors = tried ? stackErrors(values) : {};
  const msg = (k: keyof StackErrors) => (errors[k] ? t(`errors.${errors[k]}`) : undefined);
  const current = hosts.find((h) => h.key === host);
  const back = dockerTabHref(host, "stacks");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setTried(true);
    setFailure(null);
    if (Object.keys(stackErrors(values)).length) {
      requestAnimationFrame(() => document.querySelector<HTMLElement>(".ct-form [aria-invalid=true]")?.focus());
      return;
    }
    const name = values.name.trim();
    setBusy(true);
    try {
      const result = await executeDockerHostAction(host, "stack.deploy", { stack_name: name, compose_yaml: values.compose.trim() });
      if (result.status !== "succeeded") throw new Error(result.message);
      setMade({ name, href: nodeHref(dockerStackAssetId(host, name)) });
      addToast("success", t("deployedToast", { name, host: current?.label ?? host }));
      setTimeout(() => router.push(back), 1000);
    } catch (err) {
      setFailure(err instanceof Error && err.message ? err.message : t("deployFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <DeployCrumbs host={current} />
      <PageHeader title={t("stackTitle")} />
      <form className="ct-form ct-sheet" onSubmit={(e) => void submit(e)} noValidate>
        <div className="ct-grid two">
          <Field id="ct-host" label={t("host")}>
            <HostSelect id="ct-host" hosts={hosts} value={host} onChange={setHost} />
          </Field>
          <Field id="ct-sn" label={t("name")} error={msg("name")}>
            <input className="ct-input mono" id="ct-sn" value={values.name} onChange={(e) => setValues((v) => ({ ...v, name: e.target.value }))} placeholder="whoami" autoComplete="off" spellCheck={false} autoFocus {...invalid("ct-sn", msg("name"))} />
          </Field>
        </div>
        <Field id="ct-yaml" label="compose.yaml" error={msg("compose")} wide>
          <textarea className="ct-input mono ct-ta ct-yaml" id="ct-yaml" rows={14} value={values.compose} onChange={(e) => setValues((v) => ({ ...v, compose: e.target.value }))} spellCheck={false} {...invalid("ct-yaml", msg("compose"))} />
        </Field>
        {failure ? <p className="ct-err" role="alert">{t("deployFailedWith", { reason: failure })}</p> : null}
        {made && !failure ? (
          <p className="ct-done" role="status">
            {t("deployedLine", { name: made.name })} <Link href={made.href}>{t("open")}</Link>
          </p>
        ) : null}
        <div className="ct-form-foot">
          <Link href={back} className="ct-btn">{t("cancel")}</Link>
          <Button type="submit" variant="primary" loading={busy} disabled={!host}>{t("deploy")}</Button>
        </div>
      </form>
    </>
  );
}
