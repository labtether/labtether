import type { SourceType } from "./SourcePicker";

export type AddDeviceAddedEvent = {
  source: SourceType;
  focusQuery?: string;
};

export type SetupMode = "beginner" | "advanced";

export type AddDeviceCompatPrefill = {
  source: SourceType;
  connectorID: string;
  baseURL: string;
  serviceURL: string;
  serviceName: string;
  confidence: number;
  hostAssetID: string;
  authHint?: string;
};
