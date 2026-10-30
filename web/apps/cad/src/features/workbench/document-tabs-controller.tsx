import {useOperationFeedback} from "../../cad/command/operation-feedback";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { DocumentTabs } from "./document-tabs";
import { DocumentEditorDialog, type DocumentForm } from "../documents/document-editor-dialog";
import { useDocumentSessions } from "../../cad/document/document-session-context";


export function DocumentTabsController() {
  const sessions = useDocumentSessions();
  const location = useLocation();
  const navigate = useNavigate();
  const client = useQueryClient();
  const feedback=useOperationFeedback();
  const [creating, setCreating] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const visibleOrder = useRef<string[]>([]);
  const opened = useQuery({ queryKey: queryKeys.openDocuments, queryFn: api.listOpenDocuments });
  const activeID = location.pathname.match(/^\/documents\/([^/]+)/)?.[1] ?? "";

  const createDocument = async (values: DocumentForm) => {
    if (creating) return;
    setCreating(true);
    try {
      const created = await api.createDocument(values.type, values.name, values.description ?? "");
      setCreateOpen(false);
      client.setQueryData(queryKeys.document(created.document.id), created);
      void Promise.all([
        client.invalidateQueries({ queryKey: ["documents"] }),
        client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
      ]);
      navigate(`/documents/${created.document.id}`);
    } catch (error) { feedback(error,"文档操作"); }
    finally { setCreating(false); }
  };
  const closeDocument = async (id: string) => {
    try {
      await api.closeOpenDocument(id);
      sessions.close(id);
      client.setQueryData(queryKeys.openDocuments, (current: Awaited<ReturnType<typeof api.listOpenDocuments>> | undefined) =>
        current?.filter((document) => document.id !== id) ?? []);
      if (id === activeID) {
        const index = visibleOrder.current.indexOf(id);
        const next = visibleOrder.current[index + 1] ?? visibleOrder.current[index - 1];
        navigate(next ? `/documents/${next}` : "/");
      }
    } catch (error) { feedback(error,"文档操作"); }
  };

  const updateVisibleOrder = useCallback((ids: string[]) => { visibleOrder.current = ids; }, []);
  const productHostID = opened.data?.find((document) => document.id === activeID && document.type === "PRODUCT")?.id;
  return <>
    <DocumentTabs documents={(opened.data ?? []).map(({ id, name, type }) => ({ id, name, type }))}
      activeID={activeID} productHostID={productHostID} onVisibleOrderChange={updateVisibleOrder} onCreate={() => setCreateOpen(true)}
      onSwitch={(id) => navigate(`/documents/${id}`)} onClose={(id) => void closeDocument(id)} />
    {createOpen && <DocumentEditorDialog title="新建文档" createAndOpen busy={creating}
      onClose={() => setCreateOpen(false)} onSave={values => void createDocument(values)} />}
  </>;
}
