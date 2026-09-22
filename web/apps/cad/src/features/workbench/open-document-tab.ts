import type { QueryClient } from "@tanstack/react-query";
import type { DocumentSummary, DocumentView } from "../../types";
import { queryKeys } from "../../app/query-keys";

export async function openDocumentTab(documentID: string, client: QueryClient,
  getDocument: (id: string) => Promise<DocumentView>, navigate: (path: string) => void): Promise<void> {
  // Reading through the API registers an open document on the server. A cached
  // query or route-only transition does not guarantee that registration.
  const view = await getDocument(documentID);
  client.setQueryData(queryKeys.document(documentID), view);
  await client.cancelQueries({ queryKey: queryKeys.openDocuments });
  client.setQueryData<DocumentSummary[]>(queryKeys.openDocuments, (current = []) =>
    [...current.filter((document) => document.id !== documentID), view.document]);
  navigate(`/documents/${encodeURIComponent(documentID)}`);
  await client.invalidateQueries({ queryKey: queryKeys.openDocuments });
}
