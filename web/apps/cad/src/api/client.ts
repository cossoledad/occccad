import { apiURL, restApi, type CadApi } from "../api";
import { mockApi } from "./mock-api";

export const isMockMode = import.meta.env.VITE_API_MODE === "mock";
export const api: CadApi = isMockMode ? mockApi : restApi;

export function previewURL(documentID: string, versionID: string): string | undefined {
  if (isMockMode) return undefined;
  return apiURL(`/api/documents/${documentID}/preview?v=${encodeURIComponent(versionID)}`);
}
