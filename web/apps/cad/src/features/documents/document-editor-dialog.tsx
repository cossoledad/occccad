import { useQuery } from "@tanstack/react-query";
import { Button, Form, Input, Modal, Segmented } from "antd";
import { useEffect, useRef } from "react";
import { api } from "../../api/client";
import { queryKeys } from "../../app/query-keys";
import { CadIcon } from "../../cad/overlay/cad-icons";
import type { DocumentSummary, ToolbarCatalog } from "../../types";
import { defaultDocumentName } from "./document-utils";

export type DocumentForm = { name: string; description?: string; type: "PART" | "PRODUCT" };

/** Mount on open: the form is interactive before optional name suggestions arrive. */
export function DocumentEditorDialog({ editing, documents = [], documentTypes, title, busy, onClose, onSave, createAndOpen = false }: {
  editing?: DocumentSummary; documents?: DocumentSummary[]; documentTypes?: ToolbarCatalog["documents"];
  title: string; busy: boolean; onClose: () => void; onSave: (values: DocumentForm) => void; createAndOpen?: boolean;
}) {
  const catalog = useQuery({ queryKey: queryKeys.documents({ defaultNames: true }),
    queryFn: ({ signal }) => api.listDocuments({ limit: 200, allFolders: true }, signal), enabled: !editing });
  const [form] = Form.useForm<DocumentForm>();
  const initialName = editing?.name ?? defaultDocumentName("PART", catalog.data?.documents ?? documents);
  const suggested = useRef(initialName);
  useEffect(() => {
    // A late response may improve a suggestion, but cannot replace a user draft.
    if (editing || !catalog.data || form.getFieldValue("name") !== suggested.current) return;
    const next = defaultDocumentName(form.getFieldValue("type"), catalog.data.documents);
    suggested.current = next;
    form.setFieldValue("name", next);
  }, [catalog.data, editing, form]);
  return <Modal title={title} open onCancel={onClose} onOk={() => form.submit()} confirmLoading={busy}
    footer={createAndOpen ? null : undefined}>
    <Form form={form} layout="vertical" initialValues={{ type: editing?.type ?? "PART", name: initialName, description: editing?.description ?? "" }} onFinish={onSave}>
      <Form.Item name="type" label="文档类型" rules={[{ required: true }]}><Segmented block disabled={Boolean(editing)}
        onChange={value => {
          const next = defaultDocumentName(value as "PART" | "PRODUCT", catalog.data?.documents ?? documents);
          suggested.current = next; form.setFieldValue("name", next);
        }} options={documentTypes?.length ? documentTypes.map(type => ({ label: type.name, value: type.id, icon: <CadIcon name={type.iconKey} /> }))
          : [{ value: "PART", label: "零件" }, { value: "PRODUCT", label: "装配" }]} /></Form.Item>
      <Form.Item name="name" label={createAndOpen ? "名称" : "文档名称"} rules={[{ required: true, max: 120 }]}><Input autoFocus aria-label={createAndOpen ? "名称" : "文档名称"} /></Form.Item>
      <Form.Item name="description" label="说明" style={{ marginBottom: 36 }}><Input.TextArea rows={3} maxLength={500} showCount /></Form.Item>
      {createAndOpen && <Button type="primary" htmlType="submit" loading={busy}>创建并打开</Button>}
    </Form>
  </Modal>;
}
