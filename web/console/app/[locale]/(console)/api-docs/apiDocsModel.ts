/* The part of an OpenAPI 3 document the API Docs page reads, and how it turns into grouped endpoint rows. */

export type OpenAPISchema = {
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

export type OpenAPIParameter = {
  name: string;
  in: "path" | "query" | "header" | "cookie";
  description?: string;
  required?: boolean;
  schema?: OpenAPISchema;
};

export type OpenAPIMediaContent = { schema?: OpenAPISchema };

export type OpenAPIRequestBody = {
  description?: string;
  content?: Record<string, OpenAPIMediaContent>;
  required?: boolean;
};

export type OpenAPIResponse = {
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

export type HttpMethod = "get" | "post" | "put" | "patch" | "delete" | "head" | "options";
export const HTTP_METHODS: HttpMethod[] = ["get", "post", "put", "patch", "delete", "head", "options"];

type OpenAPIPathItem = Partial<Record<HttpMethod, OpenAPIOperation>>;

export type OpenAPISpec = {
  openapi?: string;
  info?: { title?: string; version?: string; description?: string };
  paths?: Record<string, OpenAPIPathItem>;
  components?: { schemas?: Record<string, OpenAPISchema> };
};

export type Endpoint = {
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

/** A schema's type as one short word: a $ref's last part, "T[]" for arrays, enum values joined by "|". */
export function renderSchemaType(schema: OpenAPISchema | undefined): string {
  if (!schema) return "any";
  if (schema.$ref) {
    const parts = schema.$ref.split("/");
    return parts[parts.length - 1] ?? schema.$ref;
  }
  if (schema.type === "array" && schema.items) return `${renderSchemaType(schema.items)}[]`;
  if (schema.enum) return schema.enum.map((v) => JSON.stringify(v)).join(" | ");
  return schema.type ?? "any";
}

/** The JSON body's schema, or the first content type's when there is no JSON. */
export function firstJsonSchema(content: Record<string, OpenAPIMediaContent> | undefined): OpenAPISchema | undefined {
  if (!content) return undefined;
  return (content["application/json"] ?? Object.values(content)[0])?.schema;
}

export const hasDetails = (ep: Endpoint) =>
  Boolean(ep.description || ep.parameters.length || ep.requestBody || (ep.responses && Object.keys(ep.responses).length));

/** Every operation in the document as one row, in the document's order. */
export function parseEndpoints(spec: OpenAPISpec | null): Endpoint[] {
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
        tags: op.tags?.length ? op.tags : ["Other"],
        parameters: op.parameters ?? [],
        requestBody: op.requestBody,
        responses: op.responses,
        deprecated: op.deprecated ?? false,
      });
    }
  }
  return result;
}

/** Endpoints under their first tag, tags in first-seen order. */
export function groupByTag(endpoints: Endpoint[]): Map<string, Endpoint[]> {
  const map = new Map<string, Endpoint[]>();
  for (const ep of endpoints) {
    const tag = ep.tags[0] ?? "Other";
    const list = map.get(tag);
    if (list) list.push(ep); else map.set(tag, [ep]);
  }
  return map;
}

/** Keeps endpoints whose path, summary, method or tag contains the filter text; empty groups drop out. */
export function filterGroups(groups: Map<string, Endpoint[]>, query: string): Map<string, Endpoint[]> {
  const q = query.trim().toLowerCase();
  if (!q) return groups;
  const result = new Map<string, Endpoint[]>();
  for (const [tag, eps] of groups) {
    const matched = eps.filter((ep) =>
      ep.path.toLowerCase().includes(q) || ep.summary.toLowerCase().includes(q) || ep.method.includes(q) || tag.toLowerCase().includes(q));
    if (matched.length) result.set(tag, matched);
  }
  return result;
}
