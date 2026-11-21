import {useState} from 'react';
import {Checkbox, Select} from 'antd';
import {useQuery} from '@tanstack/react-query';
import {api} from '../../api/client';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {CommandField} from '../../cad/overlay/command-form';
import type {DocumentView, InstancePathSegment} from '../../types';

type Props = {
  owner: DocumentView;
  target: InstancePathSegment;
  onClose: () => void;
  onApply: (view: DocumentView) => Promise<void>;
};

export function PinnedRevisionDialog({owner, target, onClose, onApply}: Props) {
  const [ids, setIds] = useState([target.instanceId]);
  const source = target.referencedDocumentId;
  const history = useQuery({
    queryKey: ['instance-source-history', source, owner.document.versionId],
    queryFn: () => api.getHistory(source),
  });
  const latest = history.data?.find(entry => entry.isHead);
  const label = (id: string) => {
    const entry = history.data?.find(item => item.versionId === id);
    return entry ? [`版本 ${entry.sequence}`, entry.versionName].filter(Boolean).join(' · ') : id;
  };
  const outdated = (owner.product?.instances ?? []).filter(instance =>
    instance.referenceMode === 'PINNED' && instance.documentId === source && latest && instance.versionId !== latest.versionId);
  const selected = outdated.filter(instance => ids.includes(instance.id)).map(instance => instance.id);

  const adopt = async () => {
    if (!latest || !selected.length) return;
    const plan = owner.product?.contextBindings?.length ? await api.getProductUpdatePlan(owner.document.id) : undefined;
    const updated = await api.command(owner.document.id, {
      type: 'UPDATE_REFERENCES',
      versionId: owner.document.versionId,
      referencedDocumentId: source,
      replacementVersionId: latest.versionId,
      pinnedInstanceIds: selected,
      updatePlanDigest: plan?.digest,
    });
    await onApply(updated);
    onClose();
  };

  return <CommandDialog id="adopt-pinned-revision" open title="采用零件新版" onClose={onClose} size="M"
    confirmDisabled={!latest || history.isFetching || !selected.length} confirmText="采用并固定" onConfirm={adopt}>
    <CommandField label="所属装配">{owner.document.name}</CommandField>
    <CommandField label="当前实例">{target.instanceName} · {label(target.resolvedVersionId)}</CommandField>
    <CommandField label="源文档最新版">{latest ? label(latest.versionId) : history.isError ? '读取版本失败' : '正在读取'}</CommandField>
    {latest && (outdated.length ? <>
      <CommandField label="采用新版的实例">
        <Select mode="multiple" value={selected} onChange={setIds}
          options={outdated.map(instance => ({value: instance.id, label: instance.name}))} maxTagCount={3}/>
      </CommandField>
      <Checkbox checked={selected.length === outdated.length}
        onChange={event => setIds(event.target.checked ? outdated.map(instance => instance.id) : [])}>
        当前装配内全部待更新的同源实例（{outdated.length}）
      </Checkbox>
    </> : <p>此装配内的同源固定实例均已使用最新版。</p>)}
  </CommandDialog>;
}
