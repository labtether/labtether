export const EXECUTABLE_UPDATE_SCOPES = ["os_packages"] as const;

export type UpdatePlanInput = {
  targets: string[];
  scopes: string[];
};

function parseCSV(value: string): string[] {
  return value
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

export function parseUpdatePlanInput(targetValue: string, scopeValue: string): UpdatePlanInput {
  const targets = [...new Set(parseCSV(targetValue))];
  if (targets.length === 0) {
    throw new Error("At least one device is required.");
  }

  const scopes = [...new Set(parseCSV(scopeValue).map((scope) => scope.toLowerCase()))];
  if (scopes.length === 0) {
    return { targets, scopes: [...EXECUTABLE_UPDATE_SCOPES] };
  }
  const unsupported = scopes.filter(
    (scope) => !EXECUTABLE_UPDATE_SCOPES.includes(scope as (typeof EXECUTABLE_UPDATE_SCOPES)[number]),
  );
  if (unsupported.length > 0) {
    throw new Error(`Unsupported update scope: ${unsupported.join(", ")}.`);
  }
  return { targets, scopes };
}
