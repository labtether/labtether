"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";

/* ---------- OpenAPI types (minimal surface we consume) ---------- */

type OpenAPISchema = {
  type?: string;
  format?: string;
  description?: string;
  properties?: Record<string, OpenAPISchema>;
  items?: OpenAPISchema;
  $ref?: string;
  enum?: unknown[];
  example?: unknown;
  additionalProperties?: OpenAPISchema | boolean;
};

type OpenAPIParameter = {
  name: string;
  in: "path" | "query" | "header" | "cookie";
  description?: string;
  required?: boolean;
  schema?: OpenAPISchema;
};

type OpenAPIMediaContent = {
  schema?: OpenAPISchema;
};

type OpenAPIRequestBody = {
  description?: string;
  content?: Record<string, OpenAPIMediaContent>;
  required?: boolean;
};

type OpenAPIResponse = {
  description?: string;
  content?: Record<string, OpenAPIMediaContent>;
};

type OpenAPIOperation = {
  summary?: string;
  description?: string;
  tags?: string[];
  parameters?: OpenAPIParameter[];
  requestBody?: OpenAPIRequestBody;
  responses?: Record<string, OpenAPIResponse>;
  operationId?: string;
  deprecated?: boolean;
};

type OpenAPIPathItem = {
  get?: OpenAPIOperation;
  post?: OpenAPIOperation;
  put?: OpenAPIOperation;
  patch?: OpenAPIOperation;
  delete?: OpenAPIOperation;
  head?: OpenAPIOperation;
  options?: OpenAPIOperation;
};

type OpenAPISpec = {
  openapi?: string;
  info?: { title?: string; version?: string; description?: string };
  paths?: Record<string, OpenAPIPathItem>;
  components?: { schemas?: Record<string, OpenAPISchema> };
};

/* ---------- HTTP method helpers ---------- */

type HttpMethod = "get" | "post" | "put" | "patch" | "delete" | "head" | "options";

const HTTP_METHODS: HttpMethod[] = ["get", "post", "put", "patch", "delete", "head", "options"];

type MethodStyle = {
  badge: string;
  label: string;
};

const METHOD_STYLES: Record<HttpMethod, MethodStyle> = {
  get:     { badge: "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20", label: "GET" },
  post:    { badge: "bg-blue-500/10 text-blue-400 border border-blue-500/20",         label: "POST" },
  put:     { badge: "bg-indigo-500/10 text-indigo-400 border border-indigo-500/20",   label: "PUT" },
  patch:   { badge: "bg-amber-500/10 text-amber-400 border border-amber-500/20",      label: "PATCH" },
  delete:  { badge: "bg-red-500/10 text-red-400 border border-red-500/20",            label: "DELETE" },
  head:    { badge: "bg-slate-500/10 text-slate-400 border border-slate-500/20",      label: "HEAD" },
  options: { badge: "bg-slate-500/10 text-slate-400 border border-slate-500/20",      label: "OPTIONS" },
};

/* ---------- Endpoint model ---------- */

type Endpoint = {
  id: string;
  method: HttpMethod;
  path: string;
  summary: string;
  description?: string;
  tags: string[];
  parameters: OpenAPIParameter[];
  requestBody?: OpenAPIRequestBody;
  responses?: Record<string, OpenAPIResponse>;
  deprecated: boolean;
};

/* ---------- Schema renderer ---------- */

function renderSchemaType(schema: OpenAPISchema | undefined): string {
  if (!schema) return "any";
  if (schema.$ref) {
    const parts = schema.$ref.split("/");
    return parts[parts.length - 1] ?? schema.$ref;
  }
  if (schema.type === "array" && schema.items) {
    return `${renderSchemaType(schema.items)}[]`;
  }
  if (schema.enum) {
    return schema.enum.map((v) => JSON.stringify(v)).join(" | ");
  }
  return schema.type ?? "any";
}

function SchemaProperties({ schema, depth = 0 }: { schema: OpenAPISchema; depth?: number }) {
  if (!schema.properties || Object.keys(schema.properties).length === 0) {
    return (
      <span className="text-xs font-mono text-[var(--muted)]">{renderSchemaType(schema)}</span>
    );
  }

  const indent = depth * 12;

  return (
    <div className="space-y-1" style={{ paddingLeft: indent }}>
      {Object.entries(schema.properties).map(([key, prop]) => (
        <div key={key} className="flex items-start gap-2 text-xs font-mono">
          <span className="text-[var(--text)] shrink-0">{key}</span>
          <span className="text-[var(--muted)] shrink-0">{renderSchemaType(prop)}</span>
          {prop.description && (
            <span className="text-[var(--muted)] font-sans not-italic truncate opacity-70">
              — {prop.description}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

/* ---------- Endpoint row ---------- */

function EndpointRow({ endpoint, t }: { endpoint: Endpoint; t: ReturnType<typeof useTranslations<"api-docs">> }) {
  const [expanded, setExpanded] = useState(false);
  const style = METHOD_STYLES[endpoint.method];

  const hasDetails =
    endpoint.description ||
    endpoint.parameters.length > 0 ||
    endpoint.requestBody !== undefined ||
    (endpoint.responses && Object.keys(endpoint.responses).length > 0);

  const firstJsonSchema = (content: Record<string, OpenAPIMediaContent> | undefined): OpenAPISchema | undefined => {
    if (!content) return undefined;
    const entry = content["application/json"] ?? Object.values(content)[0];
    return entry?.schema;
  };

  return (
    <div className="border-b border-[var(--panel-border)] last:border-0">
      <button
        type="button"
        className="w-full flex items-center gap-3 py-2.5 px-1 text-left hover:bg-[var(--hover)] transition-colors duration-[var(--dur-instant)] rounded group -mx-1"
        onClick={() => hasDetails && setExpanded((prev) => !prev)}
        aria-expanded={expanded}
      >
        <span
          className={`shrink-0 inline-flex items-center justify-center w-16 text-[10px] font-mono font-bold tracking-wider rounded px-1.5 py-0.5 ${style.badge}`}
        >
          {style.label}
        </span>
        <span className="flex-1 font-mono text-sm text-[var(--text)] truncate">
          {endpoint.path}
        </span>
        {endpoint.deprecated && (
          <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--bad)] opacity-70 shrink-0">
            deprecated
          </span>
        )}
        {endpoint.summary && (
          <span className="text-xs text-[var(--muted)] truncate hidden sm:block max-w-xs">
            {endpoint.summary}
          </span>
        )}
        {hasDetails && (
          <span className="text-[var(--muted)] opacity-50 group-hover:opacity-100 shrink-0 text-xs">
            {expanded ? "▴" : "▾"}
          </span>
        )}
      </button>

      {expanded && hasDetails && (
        <div className="ml-[4.75rem] mb-3 space-y-3 text-sm">
          {endpoint.description && (
            <p className="text-xs text-[var(--muted)] leading-relaxed">{endpoint.description}</p>
          )}

          {endpoint.parameters.length > 0 && (
            <div>
              <p className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)] mb-1.5">
                {t("parameters")}
              </p>
              <div className="space-y-1">
                {endpoint.parameters.map((param) => (
                  <div key={`${param.in}-${param.name}`} className="flex items-start gap-2 text-xs font-mono">
                    <span className="text-[var(--text)] shrink-0">{param.name}</span>
                    <span className="text-[var(--muted)] text-[10px] px-1 rounded bg-[var(--surface)] shrink-0">
                      {param.in}
                    </span>
                    <span className="text-[var(--muted)] shrink-0">{renderSchemaType(param.schema)}</span>
                    {param.required && (
                      <span className="text-[var(--bad)] text-[10px] shrink-0">required</span>
                    )}
                    {param.description && (
                      <span className="text-[var(--muted)] font-sans truncate opacity-70">
                        — {param.description}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {endpoint.requestBody && (
            <div>
              <p className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)] mb-1.5">
                {t("requestBody")}
              </p>
              {endpoint.requestBody.description && (
                <p className="text-xs text-[var(--muted)] mb-1">{endpoint.requestBody.description}</p>
              )}
              {firstJsonSchema(endpoint.requestBody.content) && (
                <div className="rounded bg-[var(--surface)] border border-[var(--panel-border)] p-2">
                  <SchemaProperties schema={firstJsonSchema(endpoint.requestBody.content)!} />
                </div>
              )}
            </div>
          )}

          {endpoint.responses && Object.keys(endpoint.responses).length > 0 && (
            <div>
              <p className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)] mb-1.5">
                {t("responses")}
              </p>
              <div className="space-y-2">
                {Object.entries(endpoint.responses).map(([code, resp]) => {
                  const statusStyle =
                    code.startsWith("2")
                      ? "text-emerald-400"
                      : code.startsWith("4")
                        ? "text-amber-400"
                        : code.startsWith("5")
                          ? "text-red-400"
                          : "text-[var(--muted)]";
                  const schema = firstJsonSchema(resp.content);
                  return (
                    <div key={code}>
                      <div className="flex items-center gap-2 text-xs font-mono">
                        <span className={`${statusStyle} font-bold`}>{code}</span>
                        {resp.description && (
                          <span className="text-[var(--muted)] font-sans">{resp.description}</span>
                        )}
                      </div>
                      {schema && (
                        <div className="mt-1 rounded bg-[var(--surface)] border border-[var(--panel-border)] p-2">
                          <SchemaProperties schema={schema} />
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Tag group ---------- */

function TagGroup({
  tag,
  endpoints,
  t,
}: {
  tag: string;
  endpoints: Endpoint[];
  t: ReturnType<typeof useTranslations<"api-docs">>;
}) {
  const [collapsed, setCollapsed] = useState(false);

  return (
    <Card className="mb-4">
      <button
        type="button"
        className="w-full flex items-center justify-between mb-1 group"
        onClick={() => setCollapsed((prev) => !prev)}
        aria-expanded={!collapsed}
      >
        <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
          <span aria-hidden="true">// </span>
          {tag}
          <span className="ml-2 text-[var(--muted)] opacity-60 font-sans normal-case">
            ({endpoints.length})
          </span>
        </h2>
        <span className="text-[var(--muted)] opacity-50 group-hover:opacity-100 text-xs">
          {collapsed ? "▾" : "▴"}
        </span>
      </button>

      {!collapsed && (
        <div className="mt-1">
          {endpoints.map((ep) => (
            <EndpointRow key={ep.id} endpoint={ep} t={t} />
          ))}
        </div>
      )}
    </Card>
  );
}

/* ---------- Main page ---------- */

export default function ApiDocsPage() {
  const t = useTranslations("api-docs");
  const [spec, setSpec] = useState<OpenAPISpec | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v2/openapi.json", { cache: "no-store" })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json() as Promise<OpenAPISpec>;
      })
      .then((data) => {
        if (!cancelled) setSpec(data);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setLoadError(err instanceof Error ? err.message : String(err));
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  /* Parse spec into flat endpoint list */
  const endpoints = useMemo<Endpoint[]>(() => {
    if (!spec?.paths) return [];
    const result: Endpoint[] = [];
    for (const [path, pathItem] of Object.entries(spec.paths)) {
      for (const method of HTTP_METHODS) {
        const op = pathItem[method];
        if (!op) continue;
        result.push({
          id: `${method}:${path}`,
          method,
          path,
          summary: op.summary ?? "",
          description: op.description,
          tags: op.tags ?? ["Other"],
          parameters: op.parameters ?? [],
          requestBody: op.requestBody,
          responses: op.responses,
          deprecated: op.deprecated ?? false,
        });
      }
    }
    return result;
  }, [spec]);

  /* Group by tag */
  const tagGroups = useMemo<Map<string, Endpoint[]>>(() => {
    const map = new Map<string, Endpoint[]>();
    for (const ep of endpoints) {
      const tag = ep.tags[0] ?? "Other";
      const existing = map.get(tag);
      if (existing) {
        existing.push(ep);
      } else {
        map.set(tag, [ep]);
      }
    }
    return map;
  }, [endpoints]);

  /* Filter by search */
  const filteredGroups = useMemo<Map<string, Endpoint[]>>(() => {
    if (!search.trim()) return tagGroups;
    const q = search.trim().toLowerCase();
    const result = new Map<string, Endpoint[]>();
    for (const [tag, eps] of tagGroups) {
      const matched = eps.filter(
        (ep) =>
          ep.path.toLowerCase().includes(q) ||
          ep.summary.toLowerCase().includes(q) ||
          ep.method.includes(q) ||
          tag.toLowerCase().includes(q),
      );
      if (matched.length > 0) result.set(tag, matched);
    }
    return result;
  }, [tagGroups, search]);

  const totalEndpoints = endpoints.length;
  const filteredCount = [...filteredGroups.values()].reduce((acc, eps) => acc + eps.length, 0);

  return (
    <>
      <PageHeader
        title={t("title")}
        subtitle={t("subtitle")}
      />

      {/* Auth note */}
      <Card className="mb-4 border-[var(--accent)]/10">
        <p className="text-xs text-[var(--muted)]">
          <span className="text-[var(--accent)] font-mono font-semibold">// </span>
          {t("authentication")}
        </p>
      </Card>

      {/* Search */}
      <div className="relative mb-4">
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("search")}
          className="w-full rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/40 transition-[border-color,box-shadow] duration-[var(--dur-fast)]"
        />
        {totalEndpoints > 0 && (
          <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] font-mono text-[var(--muted)]">
            {search ? `${filteredCount}/` : ""}{totalEndpoints} endpoints
          </span>
        )}
      </div>

      {/* Loading */}
      {!spec && !loadError && (
        <Card>
          <div className="flex items-center justify-center py-10">
            <p className="text-sm text-[var(--muted)]">{t("loading")}</p>
          </div>
        </Card>
      )}

      {/* Error */}
      {loadError && (
        <Card className="border-[var(--bad)]/20">
          <p className="text-sm text-[var(--bad)]">{t("loadError")}: {loadError}</p>
        </Card>
      )}

      {/* No results */}
      {spec && filteredGroups.size === 0 && (
        <Card>
          <p className="text-sm text-[var(--muted)] text-center py-6">{t("noResults")}</p>
        </Card>
      )}

      {/* Endpoint groups */}
      {[...filteredGroups.entries()].map(([tag, eps]) => (
        <TagGroup key={tag} tag={tag} endpoints={eps} t={t} />
      ))}
    </>
  );
}
