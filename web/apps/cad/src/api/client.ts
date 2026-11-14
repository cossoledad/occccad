import { apiURL, type CadApi } from "../api";
import { selectedApi, isMockMode } from "@cad-api-mode";

export { isMockMode };
export const api: CadApi = selectedApi;

export function previewURL(documentID: string, versionID: string): string | undefined {
  if (isMockMode) return undefined;
  return apiURL(`/api/documents/${documentID}/preview?v=${encodeURIComponent(versionID)}`);
}
