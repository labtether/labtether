"use client";

import { useTranslations } from "next-intl";
import type { Asset } from "../../../console/models";
import type { useActions } from "../../../hooks/useActions";
import { Input, Select } from "../../../components/ui/Input";
import { Field, fieldAria } from "./_parts/Editor";
import { Switch } from "./_parts/RowMenu";

type Integration = ReturnType<typeof useActions>;

/** Empty required fields of the chosen connector action: { target | p-<key>: message }. */
export function integrationErrors(integration: Integration, t: (key: string) => string): Record<string, string> {
  const found: Record<string, string> = {};
  if (integration.actionRequiresTarget && !integration.actionTarget.trim()) found.target = t("targetRequired");
  for (const parameter of integration.actionParameters) {
    if (parameter.required && !(integration.actionParamValues[parameter.key] ?? "").trim()) found[`p-${parameter.key}`] = t("paramRequired");
  }
  return found;
}

/**
 * The Integration action fields: integration, action and target (suggesting the devices that integration reports),
 * the action's parameters, and Preview, a dry run that checks the action and changes nothing.
 */
export function IntegrationFields({ integration, assets, errors }: { integration: Integration; assets: Asset[]; errors: Record<string, string> }) {
  const t = useTranslations("actions.integration");
  const descriptor = integration.selectedActionDescriptor;
  const own = assets.filter((asset) => asset.source === integration.selectedConnector);
  const suggestions = own.length ? own : assets;

  if (integration.connectors.length === 0) return <p className="au-hint" role="status">{t("none")}</p>;

  return (
    <>
      <div className="au-grid three">
        <Field id="af-conn" label={t("integration")}>
          <Select className="w-full" id="af-conn" value={integration.selectedConnector} onChange={(e) => integration.setSelectedConnector(e.target.value)}>
            {integration.connectors.map((c) => <option key={c.id} value={c.id}>{c.display_name}</option>)}
          </Select>
        </Field>
        <Field id="af-act" label={t("action")} hint={descriptor?.description}>
          <Select
            className="w-full"
            id="af-act"
            value={integration.selectedConnectorAction}
            disabled={integration.connectorActions.length === 0}
            onChange={(e) => integration.setSelectedConnectorAction(e.target.value)}
            {...fieldAria("af-act", undefined, !!descriptor?.description)}
          >
            {integration.connectorActions.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </Select>
        </Field>
        {integration.actionRequiresTarget ? (
          <Field id="af-tgt" label={t("target")} error={errors.target}>
            <Input
              id="af-tgt"
              className="au-mono"
              list="af-tgt-list"
              value={integration.actionTarget}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => integration.setActionTarget(e.target.value)}
              {...fieldAria("af-tgt", errors.target)}
            />
            <datalist id="af-tgt-list">
              {suggestions.map((asset) => <option key={asset.id} value={asset.id}>{asset.name}</option>)}
            </datalist>
          </Field>
        ) : null}
      </div>
      {integration.connectorActionsError ? <p role="alert" className="au-err au-gap">{t("loadFailed")}</p> : null}
      {integration.actionParameters.length > 0 ? (
        <div className="au-grid three au-gap">
          {integration.actionParameters.map((parameter) => {
            const id = `af-p-${parameter.key}`;
            const error = errors[`p-${parameter.key}`];
            return (
              <Field key={parameter.key} id={id} label={parameter.label || parameter.key} error={error}>
                <Input
                  id={id}
                  className="au-mono"
                  value={integration.actionParamValues[parameter.key] ?? ""}
                  placeholder={parameter.required ? parameter.description : parameter.description || t("optional")}
                  autoComplete="off"
                  spellCheck={false}
                  onChange={(e) => integration.setActionParamValue(parameter.key, e.target.value)}
                  {...fieldAria(id, error)}
                />
              </Field>
            );
          })}
        </div>
      ) : null}
      <div className="au-pv">
        <Switch
          id="af-pv"
          on={integration.actionSupportsDryRun && integration.actionDryRun}
          labelledBy="af-pv-l"
          disabled={!integration.actionSupportsDryRun}
          onChange={integration.setActionDryRun}
        />
        <span id="af-pv-l">{t("preview")}</span>
        <span className="au-hint">{integration.actionSupportsDryRun ? t("previewHint") : t("noPreview")}</span>
      </div>
    </>
  );
}
