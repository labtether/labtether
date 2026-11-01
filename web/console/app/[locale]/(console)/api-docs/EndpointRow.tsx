"use client";

import { useState } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { firstJsonSchema, hasDetails, renderSchemaType, type Endpoint, type OpenAPISchema } from "./apiDocsModel";

/** A schema's fields, one per line: name, type, description; nested objects indent under their field. */
function SchemaProperties({ schema, depth = 0 }: { schema: OpenAPISchema; depth?: number }) {
  if (!schema.properties || Object.keys(schema.properties).length === 0) {
    return <span className="iu-api-type">{renderSchemaType(schema)}</span>;
  }
  return (
    <div className="iu-api-props" style={{ paddingLeft: depth * 12 }}>
      {Object.entries(schema.properties).map(([key, prop]) => (
        <div key={key} className="iu-api-prop">
          <span className="iu-api-k">{key}</span>
          <span className="iu-api-type">{renderSchemaType(prop)}</span>
          {prop.description ? <span className="iu-api-desc">{prop.description}</span> : null}
        </div>
      ))}
    </div>
  );
}

function Details({ endpoint }: { endpoint: Endpoint }) {
  const t = useTranslations("api-docs");
  const body = firstJsonSchema(endpoint.requestBody?.content);
  const responses = Object.entries(endpoint.responses ?? {});
  return (
    <div className="iu-api-det">
      {endpoint.description ? <p className="iu-api-text">{endpoint.description}</p> : null}
      {endpoint.parameters.length > 0 ? (
        <div>
          <h3>{t("parameters")}</h3>
          {endpoint.parameters.map((param) => (
            <div key={`${param.in}-${param.name}`} className="iu-api-prop">
              <span className="iu-api-k">{param.name}</span>
              <span className="iu-api-in">{param.in}</span>
              <span className="iu-api-type">{renderSchemaType(param.schema)}</span>
              {param.required ? <span className="iu-api-req">{t("required")}</span> : null}
              {param.description ? <span className="iu-api-desc">{param.description}</span> : null}
            </div>
          ))}
        </div>
      ) : null}
      {endpoint.requestBody ? (
        <div>
          <h3>{t("requestBody")}</h3>
          {endpoint.requestBody.description ? <p className="iu-api-text">{endpoint.requestBody.description}</p> : null}
          {body ? <SchemaProperties schema={body} /> : null}
        </div>
      ) : null}
      {responses.length > 0 ? (
        <div>
          <h3>{t("responses")}</h3>
          {responses.map(([code, resp]) => {
            const schema = firstJsonSchema(resp.content);
            return (
              <div key={code} className="iu-api-resp">
                <div className="iu-api-prop">
                  <span className="iu-api-k">{code}</span>
                  {resp.description ? <span className="iu-api-desc">{resp.description}</span> : null}
                </div>
                {schema ? <SchemaProperties schema={schema} depth={1} /> : null}
              </div>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}

/**
 * One endpoint: method, path and summary on one line; the row opens to its parameters, body and responses.
 * Methods are plain text (no colour per method): colour on this page would only mean status.
 */
export function EndpointRow({ endpoint }: { endpoint: Endpoint }) {
  const t = useTranslations("api-docs");
  const [open, setOpen] = useState(false);
  const details = hasDetails(endpoint);
  return (
    <>
      <button
        type="button"
        className={`iu-row iu-api-row${open ? " open" : ""}`}
        onClick={() => details && setOpen((v) => !v)}
        aria-expanded={details ? open : undefined}
        disabled={!details}
      >
        <span className="iu-api-m">{endpoint.method.toUpperCase()}</span>
        <span className="iu-api-p">{endpoint.path}</span>
        <span className="iu-api-s">
          {endpoint.deprecated ? <span className="iu-api-dep">{t("deprecated")}</span> : null}
          {endpoint.summary}
        </span>
        {details ? <ChevronDown className="iu-api-chev" aria-hidden="true" strokeWidth={2} /> : <span />}
      </button>
      {open && details ? <Details endpoint={endpoint} /> : null}
    </>
  );
}
