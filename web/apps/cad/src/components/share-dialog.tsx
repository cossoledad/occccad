import {engineeringLabel} from "../cad/presentation/engineering-text";
import {engineeringProblem,useOperationFeedback} from "../cad/command/operation-feedback";
import { DeleteOutlined, ShareAltOutlined } from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Form, List, Modal, Select, Space, Tag, Typography } from "antd";
import { useMemo } from "react";
import { api } from "../api/client";

export type ShareResource = { type: "documents" | "folders"; id: string; name: string };

export function ShareDialog({ resource, onClose }: { resource?: ShareResource; onClose: () => void }) {
  const feedback=useOperationFeedback();
  const client = useQueryClient();
  const key = ["shares", resource?.type, resource?.id];
  const grants = useQuery({ queryKey: key, queryFn: () => api.listShares(resource!.type, resource!.id), enabled: Boolean(resource) });
  const users = useQuery({ queryKey: ["share-subjects", "users"], queryFn: api.listUsers, enabled: Boolean(resource) });
  const teams = useQuery({ queryKey: ["share-subjects", "teams"], queryFn: api.listTeams, enabled: Boolean(resource) });
  const subjects = useMemo(() => [
    ...(users.data ?? []).map((user) => ({ value: `USER:${user.id}`, label: `${user.displayName} · ${user.email}` })),
    ...(teams.data ?? []).map((team) => ({ value: `TEAM:${team.id}`, label: `${team.name} · 团队` })),
  ], [users.data, teams.data]);
  const add = useMutation({ mutationFn: (values: { subject: string; role: "VIEWER" | "EDITOR" }) => {
    const [subjectType, subjectID] = values.subject.split(":") as ["USER" | "TEAM", string];
    return api.share(resource!.type, resource!.id, subjectType, subjectID, values.role);
  }, onError:error=>feedback(error,"保存共享权限"), onSuccess: async () => { await client.invalidateQueries({ queryKey: key }); } });
  const remove = useMutation({ mutationFn: (grantID: string) => api.unshare(resource!.type, resource!.id, grantID),
    onError:error=>feedback(error,"移除共享权限"), onSuccess: async () => { await client.invalidateQueries({ queryKey: key }); } });

  return <Modal width={620} title={<Space><ShareAltOutlined />共享“{resource?.name}”</Space>} open={Boolean(resource)}
    onCancel={onClose} footer={<Button onClick={onClose}>完成</Button>} destroyOnHidden>
    <Form layout="inline" initialValues={{ role: "VIEWER" }} onFinish={(values) => add.mutate(values)} className="share-form">
      <Form.Item name="subject" rules={[{ required: true }]} className="share-subject"><Select showSearch optionFilterProp="label" placeholder="选择用户或团队" options={subjects} /></Form.Item>
      <Form.Item name="role"><Select options={[{ value: "VIEWER", label: "可查看" }, { value: "EDITOR", label: "可编辑" }]} /></Form.Item>
      <Button type="primary" htmlType="submit" loading={add.isPending}>添加</Button>
    </Form>
    {(grants.error||users.error||teams.error)&&<Alert type="error" title="无法读取共享信息" description={engineeringProblem(grants.error??users.error??teams.error)} action={<Button size="small" onClick={()=>{void grants.refetch();void users.refetch();void teams.refetch()}}>重试</Button>}/>}
    <List loading={grants.isLoading} locale={{ emptyText: "尚未共享" }} dataSource={grants.data ?? []}
      renderItem={(grant) => <List.Item actions={grant.inherited ? [] : [<Button key="remove" danger type="text" icon={<DeleteOutlined />}
        loading={remove.isPending} onClick={() => remove.mutate(grant.id)}>移除</Button>]}>
        <List.Item.Meta title={grant.subjectName} description={<Typography.Text type="secondary">{grant.subjectType==="USER"?"用户":"团队"}{grant.inherited && ` · 继承自 ${grant.sourceName ?? "上级文件夹"}`}</Typography.Text>} />
        <Tag color={grant.role === "EDITOR" ? "blue" : "default"}>{engineeringLabel(grant.role,"权限待检查")}</Tag>
      </List.Item>} />
  </Modal>;
}
