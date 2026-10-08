
export const BASE_TS = "2026-01-01T12:00:00.000Z";

export function makePBSAsset() {
  return {
    id: "pbs-server-lab",
    type: "storage-controller",
    name: "Lab PBS",
    source: "pbs",
    status: "online",
    last_seen_at: BASE_TS,
    metadata: {
      collector_id: "collector-pbs-1",
    },
  };
}
