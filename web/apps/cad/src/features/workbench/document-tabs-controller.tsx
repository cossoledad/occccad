import { useQuery, useQueryClient } from "@tanstack/react-query";
import { App, Button, Form, Input, Modal, Segmented } from "antd";
import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { DocumentTabs } from "./document-tabs";
import { defaultDocumentName } from "../documents/document-utils";

type CreateDocumentValues = { type: "PART" | "PRODUCT"; name: string; description?: string };

export function DocumentTabsController() {
  const location = useLocation();
  const navigate = useNavigate();
  const client = useQueryClient();
  const { message } = App.useApp();
  const [creating, setCreating] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm<CreateDocumentValues>();
  const opened = useQuery({ queryKey: queryKeys.openDocuments, queryFn: api.listOpenDocuments });
  const catalog = useQuery({ queryKey: queryKeys.documents({ defaultNames: true }),
    queryFn: () => api.listDocuments({ limit: 200, allFolders: true }) });
  const activeID = location.pathname.match(/^\/documents\/([^/]+)/)?.[1] ?? "";

  const createDocument = async (values: CreateDocumentValues) => {
    if (creating) return;
    setCreating(true);
    try {
      const created = await api.createDocument(values.type, values.name, values.description ?? "");
      setCreateOpen(false);
      await Promise.all([
        client.invalidateQueries({ queryKey: ["documents"] }),
        client.invalidateQueries({ queryKey: queryKeys.openDocuments }),
      ]);
      navigate(`/documents/${created.document.id}`);
    } catch (error) { message.error((error as Error).message); }
    finally { setCreating(false); }
  };
  const closeDocument = async (id: string) => {
    try {
      await api.closeOpenDocument(id);
      await client.invalidateQueries({ queryKey: queryKeys.openDocuments });
      if (id === activeID) {
        const next = opened.data?.find((document) => document.id !== id)?.id;
        navigate(next ? `/documents/${next}` : "/");
      }
    } catch (error) { message.error((error as Error).message); }
  };
  const openCreateDocument = () => {
    createForm.setFieldsValue({ type: "PART", name: defaultDocumentName("PART", catalog.data?.documents ?? []), description: "" });
    setCreateOpen(true);
  };

  return <>
    <DocumentTabs documents={(opened.data ?? []).map(({ id, name, type }) => ({ id, name, type }))}
      activeID={activeID} onCreate={openCreateDocument}
      onSwitch={(id) => navigate(`/documents/${id}`)} onClose={(id) => void closeDocument(id)} />
    <Modal title="新建文档" open={createOpen} onCancel={() => { setCreateOpen(false); createForm.resetFields(); }} footer={null} destroyOnHidden>
      <Form form={createForm} layout="vertical" onFinish={(values) => void createDocument(values)}>
        <Form.Item name="type" label="文档类型" rules={[{ required: true }]}><Segmented block
          onChange={(value) => createForm.setFieldValue("name", defaultDocumentName(value as "PART" | "PRODUCT", catalog.data?.documents ?? []))} options={[
          { value: "PART", label: "Part" }, { value: "PRODUCT", label: "Product" },
        ]} /></Form.Item>
        <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input autoFocus /></Form.Item>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item>
        <Button block type="primary" htmlType="submit" loading={creating}>创建并打开</Button>
      </Form>
    </Modal>
  </>;
}
