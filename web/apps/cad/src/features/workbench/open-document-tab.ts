import type { QueryClient } from "@tanstack/react-query";
import type { DocumentSummary, DocumentView } from "../../types";
import { queryKeys } from "../../app/query-keys";

export async function openDocumentTab(documentID: string, client: QueryClient,
  openDocument: (id: string) => Promise<DocumentView>, navigate: (path: string) => void): Promise<void> {
  await registerDocumentTab(documentID, client, openDocument);
  navigate(`/documents/${encodeURIComponent(documentID)}`);
}

export async function registerDocumentTab(documentID: string, client: QueryClient,
  openDocument: (id: string) => Promise<DocumentView>): Promise<DocumentView> {
  const view = await openDocument(documentID);
  client.setQueryData(queryKeys.document(documentID), view);
  await client.cancelQueries({ queryKey: queryKeys.openDocuments });
  client.setQueryData<DocumentSummary[]>(queryKeys.openDocuments, (current = []) =>
    current.some((document) => document.id === documentID)
      ? current.map((document) => document.id === documentID ? view.document : document)
      : [...current, view.document]);
  // The explicit open is already registered. Refresh only to merge other tabs
  // that may have been omitted by a canceled initial list request.
  void client.invalidateQueries({ queryKey: queryKeys.openDocuments });
  return view;
}

export function updateOpenDocumentSummary(client: QueryClient, document: DocumentSummary): void {
  client.setQueryData<DocumentSummary[]>(queryKeys.openDocuments, (current = []) =>
    current.map((candidate) => candidate.id === document.id ? document : candidate));
}
