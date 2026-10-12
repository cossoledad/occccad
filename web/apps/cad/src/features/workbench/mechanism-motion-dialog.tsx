import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Checkbox, Input, InputNumber, Select, Slider, Space, Table, Typography } from 'antd';
import { api } from '../../api/client';
import type { DocumentView } from '../../types';
import type { CommandOperation } from '../../cad/command/command-registry';
import { CommandDialog } from '../../cad/overlay/floating-panel';
import { normalizeOperationFailure, useOperationFeedback } from '../../cad/command/operation-feedback';
import { MechanismMotionSession, type MechanismMotionIntent } from '../../cad/assembly/mechanism-motion-session';
import { MotionRecordingClock, motionTargetAt, replaceMotionKey, sameMotionKeyValue } from '../../cad/assembly/motion-recording';
import { motionFrameAt, motionPlayback, motionPlaybackTime, quantityIn, type Mechanism, type MechanismSessionFrame, type MechanismSessionOpened, type MotionDriver, type MotionKeyframe, type MotionPlayback, type MotionPlaybackClock, type MotionPose, type MotionRun, type MotionStudy, type MotionViewpoint } from '../../cad/assembly/motion-study';
import {motionCommandRange,driverCoordinate,coordinateUnit} from '../../cad/assembly/motion-coordinates';
import { motionViewpointAt } from '../../cad/assembly/motion-frame';
import { randomUUID } from '../../utils/random-uuid';
type Props = {
    view: DocumentView;
    mechanism: Mechanism;
    drivers: MotionDriver[];
    initialDriverIds?: string[];
    study?: MotionStudy;
    record: boolean;
    operation?: CommandOperation;
    sourceMechanismIds?: string[];
    onSession: (session?: MechanismMotionSession) => void;
    onDraft: (study?: MotionStudy) => void;
    onPreview: (poses?: Array<MotionPose & {
        instanceId: string;
    }>) => void;
    onPlayback: (playback?: MotionPlayback) => void;
    onCommit: (command: Record<string, unknown>, studyId?: string) => Promise<DocumentView>;
    onClose: (view?: DocumentView) => void;
    captureViewpoint: () => MotionViewpoint | undefined;
    previewViewpoint: (viewpoint: MotionViewpoint, documentId?: string) => void;
};
export function MechanismMotionDialog(props: Props) {
    const feedback = useOperationFeedback();
    const ownerId = useRef(randomUUID());
    const callbacks = useRef(props);
    callbacks.current = props;
    const [selectedDrivers,setSelectedDrivers]=useState<string[]>(props.study?.activeDriverIds??props.initialDriverIds??[]);
    const [opened, setOpened] = useState<MechanismSessionOpened>(), [frame, setFrame] = useState<MechanismSessionFrame>(), [busy, setBusy] = useState(true), [saving, setSaving] = useState(false), [error, setError] = useState<unknown>();
    const [study, setStudy] = useState<MotionStudy>(), [automatic, setAutomatic] = useState(true), [camera, setCamera] = useState(props.study?.animateViewpoint ?? false);
    const [keyId, setKeyId] = useState<string>(), [insertTime, setInsertTime] = useState(.2), [time, setTime] = useState(0);
    const [inputValues,setInputValues]=useState<Record<string,number>>({});
 const goals=useRef<Record<string,number>>({});
    const [switching, setSwitching] = useState(false);
    const [attempt, setAttempt] = useState(0);
    const [revalidate, setRevalidate] = useState(false), [baking, setBaking] = useState(false);
    const [run, setRun] = useState<MotionRun>(), [clock, setClock] = useState<MotionPlaybackClock>();
    const controller = useRef<MechanismMotionSession | undefined>(undefined), baselineCamera = useRef<MotionViewpoint | undefined>(undefined);
    const pauseRef=useRef<(()=>Promise<void>)|undefined>(undefined);
    const preparedCommit = useRef<Record<string, unknown> | undefined>(undefined), qualifiedTime = useRef(0);
    const automaticClock = useRef(new MotionRecordingClock());
    const draft = useRef<MotionStudy | undefined>(undefined), autoRef = useRef(automatic), cameraRef = useRef(camera), nextTime = useRef(insertTime), recording = useRef(false), closed = useRef(false), epoch = useRef(0), playingClock = useRef(clock);
    playingClock.current = clock;
    autoRef.current = automatic;
    cameraRef.current = camera;
    nextTime.current = insertTime;
    const publishStudy = (value: MotionStudy, pathChanged = true) => { draft.current = value; setStudy(value); callbacks.current.onDraft(value); if (pathChanged)
        setRun(undefined); };
    const commandQuantities=(values:Record<string,number>,ids=controller.current?.checkpoint?.activeDriverIds??[]):MotionKeyframe['values']=>Object.fromEntries(ids.map(id=>{const d=props.drivers.find(d=>d.id===id),coordinate=d&&driverCoordinate(props.mechanism,d);if(!coordinate)throw new Error('命令参数已变化');return [id,{value:values[id]??0,unit:coordinateUnit(coordinate)}]}));
    const makeKey=(id=randomUUID(),at=nextTime.current):MotionKeyframe=>({id,timeSeconds:at,values:commandQuantities(controller.current?.checkpoint?.frame.commandValues??{}),viewpoint:cameraRef.current?callbacks.current.captureViewpoint():undefined});
    const initializeRecording=async(owner:MechanismMotionSession,result:MechanismSessionFrame)=>{
        if(!props.record||!result.qualified||draft.current||recording.current)return;
        recording.current=true;
        try{
            const initial:MotionStudy=owner.identity?.study?structuredClone(owner.identity.study):{id:randomUUID(),name:'模拟.'+((props.view.product?.kinematics?.studies.length??0)+1),mechanismId:props.mechanism.id,activeDriverIds:[...result.activeDriverIds],durationSeconds:0,frames:121,budgetMs:60000,sourceRevisionId:result.baseRevisionId,basisDigest:result.basisDigest,geometryDigest:result.geometryDigest,references:Object.fromEntries(props.drivers.filter(d=>result.activeDriverIds.includes(d.id)).map(d=>[d.id,d.reference!])),initialPoses:result.initialPoses,initialCoordinates:result.frame.coordinates,keyframes:[],animateViewpoint:cameraRef.current};
            if(!initial.keyframes?.length){const key=await owner.record({id:randomUUID(),timeSeconds:0,values:commandQuantities(result.frame.commandValues,result.activeDriverIds),viewpoint:cameraRef.current?callbacks.current.captureViewpoint():undefined});initial.keyframes=[key]}
            if(closed.current||controller.current!==owner)return;
            publishStudy(initial);automaticClock.current.anchor(initial.keyframes!.at(-1)!.timeSeconds);setKeyId(initial.keyframes![0].id);setInsertTime((initial.keyframes?.at(-1)?.timeSeconds??0)+.2);
        }finally{recording.current=false}
    };
    const initializeRecordingRef=useRef(initializeRecording);initializeRecordingRef.current=initializeRecording;
    const insert = async (modify = false, fromFrame = false, intent?: MechanismMotionIntent) => {
        const owner = controller.current;
        if (!owner || !draft.current || recording.current)
            return;
        recording.current = true;
        try {
            if (!fromFrame)
                await owner.settle();
            const existing = modify ? draft.current.keyframes?.find(k => k.id === keyId) : undefined;
            const proposed = makeKey(existing?.id, fromFrame ? intent!.timeSeconds : nextTime.current);
            if (fromFrame)
                proposed.viewpoint = intent!.viewpoint;
            if (fromFrame && draft.current.keyframes?.at(-1) && sameMotionKeyValue(draft.current.keyframes.at(-1)!, proposed))
                return;
            replaceMotionKey(draft.current.keyframes ?? [], proposed);
            const key = await owner.record(proposed);
            if (closed.current || controller.current !== owner)
                return;
            publishStudy({ ...draft.current, keyframes: replaceMotionKey(draft.current.keyframes ?? [], key) });
            if (!fromFrame)
                automaticClock.current.anchor(draft.current.keyframes!.at(-1)!.timeSeconds);
            setKeyId(key.id);
            setTime(key.timeSeconds);
            qualifiedTime.current = key.timeSeconds;
            setInsertTime(Math.min(3600, Math.max(key.timeSeconds, draft.current.keyframes?.at(-1)?.timeSeconds ?? 0) + .2));
        }
        finally {
            recording.current = false;
        }
    };
    const insertRef = useRef(insert);
    insertRef.current = insert;
    useEffect(() => {
        const generation = ++epoch.current;
        closed.current = false;
        setBusy(true);
        setError(undefined);
        setOpened(undefined);
        setFrame(undefined);
        setRun(undefined);
        setClock(undefined);
        draft.current=undefined;setStudy(undefined);
        automaticClock.current = new MotionRecordingClock();
        baselineCamera.current = callbacks.current.captureViewpoint();
        const leaseAbort = new AbortController();
        let lease: ReturnType<typeof setInterval> | undefined;
        const owner = new MechanismMotionSession({
            begin: (input, signal) => api.beginMechanismSession(props.view.document.id, input, signal),
            update: (id, input, signal) => api.updateMechanismSession(props.view.document.id, id, input, signal),
            record: (id, sequence, key, signal) => api.recordMechanismKey(props.view.document.id, id, sequence, key, signal),
            finalize: (id, sequence, value, signal) => api.finalizeMechanismSession(props.view.document.id, id, sequence, value, signal),
            preview: (id, value, signal) => api.previewMechanismRecording(props.view.document.id, id, value, signal),
            cancel: id => api.cancelMechanismSession(props.view.document.id, id),
            prepareSpatial:async()=>{if(playingClock.current)await pauseRef.current?.()},
            onIntent: () => {
                if (!props.record || !autoRef.current || !draft.current)
                    return;
                const at = automaticClock.current.stamp(performance.now());
                if (at > 3600 || (draft.current.keyframes?.length ?? 0) >= 500) {
                    autoRef.current = false;
                    setAutomatic(false);
                    setError(new Error('录制已达上限'));
                    return;
                }
                return {timeSeconds:at,viewpoint:cameraRef.current?callbacks.current.captureViewpoint():undefined};
            },
            onBusy: value => { if (generation === epoch.current && !closed.current)
                setBusy(value); },
            onFrame: async (value, recordable, intent) => {
                if (generation !== epoch.current || closed.current)
                    return;
                setFrame(value);
                callbacks.current.onSession(owner);
                goals.current={...value.frame.commandValues};setInputValues(goals.current);
                if(value.qualified){setSelectedDrivers(value.activeDriverIds);setOpened(opened=>opened?{...opened,...value}:opened)}
                await initializeRecordingRef.current(owner,value);
                callbacks.current.onPlayback();
                callbacks.current.onPreview(Object.entries(value.frame.unitPoses).map(([instanceId, pose]) => ({ instanceId, ...pose })));
                if (value.accepted && value.sequence > 0) {
                    setError(undefined);
                    if (props.record && recordable && intent)
                        await insertRef.current(false, true, intent).catch(setError);
                }
            },
        });
        controller.current = owner;
        callbacks.current.onSession(owner);
        const unload = (event: PageTransitionEvent) => { if (!event.persisted)
            void owner.cancel().catch(() => undefined); };
        window.addEventListener('pagehide', unload);
        void owner.begin({ ownerId: ownerId.current, generation, baseRevisionId: props.view.document.versionId, mechanismId: props.mechanism.id, activeDriverIds: selectedDrivers.length?selectedDrivers:undefined, studyId: props.study?.id, revalidate, sourceMechanismIds: props.sourceMechanismIds }).then(async (result) => {
            if (!result || generation !== epoch.current || closed.current)
                return;
            setOpened(result);
            lease = setInterval(() => { void api.touchMechanismSession(props.view.document.id, result.sessionId, leaseAbort.signal).catch(e => { if (generation === epoch.current && !closed.current)
                setError(e); }); }, 60000);
            await initializeRecordingRef.current(owner,result);
        }).catch(e => { if (generation === epoch.current && !closed.current)
            setError(e); });
        return () => { window.removeEventListener('pagehide', unload); epoch.current++; clearInterval(lease); leaseAbort.abort(); if (!closed.current && baselineCamera.current)
            callbacks.current.previewViewpoint(baselineCamera.current, props.view.document.id); void owner.cancel().catch(() => undefined); if (controller.current === owner) {
            controller.current = undefined;
            callbacks.current.onSession();
        } };
    }, [props.view.document.id, props.view.document.versionId, props.mechanism.id, revalidate, attempt]);
    const close = () => {
        if (saving || closed.current)
            return;
        closed.current = true;
        epoch.current++;
        setClock(undefined);
        void controller.current?.cancel().catch(() => undefined);
        callbacks.current.onSession();
        callbacks.current.onDraft();
        callbacks.current.onPlayback();
        callbacks.current.onPreview();
        if (baselineCamera.current)
            callbacks.current.previewViewpoint(baselineCamera.current, props.view.document.id);
        callbacks.current.onClose();
    };
    useEffect(() => { const signal = props.operation?.signal; if (!signal)
        return; signal.addEventListener('abort', close); return () => signal.removeEventListener('abort', close); }, [props.operation, saving]);
    const selectDriver=async(id:string,checked:boolean)=>{
        const next=checked?[...selectedDrivers,id]:selectedDrivers.filter(value=>value!==id);
        setSelectedDrivers(next);if(!controller.current?.identity)return;
        setSwitching(true);
        try{await controller.current.request({activeDriverIds:next},false)}catch(e){setError(e)}finally{setSwitching(false)}
    };
    const request=async(id:string,value:number)=>{
        const checkpoint=controller.current?.checkpoint;if(!checkpoint?.qualified)return;
        goals.current={...goals.current,[id]:value};setInputValues(goals.current);
        setClock(undefined);setRun(undefined);
        try{await controller.current?.request({values:commandQuantities(goals.current,checkpoint.activeDriverIds)})}catch(e){if(!closed.current)setError(e)}
    };
    const seek = async (value: number) => {
        if (!draft.current)
            return false;
        setClock(undefined);
        try {
            const response = await controller.current?.request({values:motionTargetAt(draft.current,value)}, false);
            if (response?.accepted) {
                setTime(value);
                qualifiedTime.current = value;
                const viewpoint = motionViewpointAt({ ...draft.current, animateViewpoint: cameraRef.current }, value);
                if (viewpoint)
                    callbacks.current.previewViewpoint(viewpoint, props.view.document.id);
                return true;
            }
            setTime(qualifiedTime.current);
            return false;
        }
        catch (e) {
            setTime(qualifiedTime.current);
            setError(e);
            return false;
        }
    };
    // Playback is display interpolation over a qualified finite cache. Confirmation
    // resolves its exact unrounded playhead through the command solver below.
    const exactTime = () => playingClock.current ? motionPlaybackTime(playingClock.current, performance.now()) : time;
    const pause = async () => { const at = exactTime(); playingClock.current=undefined;setClock(undefined); setTime(at); if(!await seek(at))throw new Error('当前时间未通过求解'); };
 pauseRef.current=pause;
    const play = async () => {
        if (clock) {
            await pause();
            return;
        }
        try {
            let cache = run;
            if (!cache) {
                setBaking(true);
                setBusy(true);
                cache = await controller.current!.preview({ ...draft.current!, animateViewpoint: cameraRef.current });
                setRun(cache);
            }
            if (closed.current || !cache.frames.length)
                return;
            const start = time >= cache.frames.at(-1)!.timeSeconds ? 0 : time;
            const next = { frames: cache.frames, startedAt: performance.now(), timeSeconds: start };
            setTime(start);
            setClock(next);
            const playback = motionPlayback(cache, motionFrameAt(cache, start));
            if (playback)
                callbacks.current.onPlayback({ ...playback, clock: next, study: draft.current });
        }
        catch (e) {
            if (!closed.current)
                setError(e);
        }
        finally {
            setBaking(false);
            setBusy(false);
        }
    };
    useEffect(() => { if (!clock)
        return; let handle = 0, lastUpdate = 0; const tick = () => { const now = performance.now(), at = motionPlaybackTime(clock, now); if (now - lastUpdate >= 50 || at >= (clock.frames.at(-1)?.timeSeconds ?? 0)) {
        setTime(at);
        lastUpdate = now;
    } if (at < (clock.frames.at(-1)?.timeSeconds ?? 0))
        handle = requestAnimationFrame(tick);
    else {
        void pause().catch(setError);
    } }; handle = requestAnimationFrame(tick); return () => cancelAnimationFrame(handle); }, [clock]);
    const confirm = async () => {
        if (!['OWNER', 'EDITOR'].includes(props.view.document.permission ?? '')) {
            close();
            return;
        }
        if (!controller.current)
            return;
        setSaving(true);
        setError(undefined);
        try {
            if (!preparedCommit.current) {
                if (playingClock.current) {
                    const at = exactTime();
                    setClock(undefined);
                    if (!await seek(at))
                        return;
                }
                await controller.current.settle();
                preparedCommit.current = await controller.current.finalize(props.record ? { ...draft.current!, animateViewpoint: cameraRef.current } : undefined);
            }
            const command = preparedCommit.current;
            const updated = await callbacks.current.onCommit(command, draft.current?.id);
            closed.current = true;
            epoch.current++;
            void controller.current.cancel().catch(() => undefined);
            callbacks.current.onSession();
            callbacks.current.onDraft();
            callbacks.current.onPlayback();
            callbacks.current.onPreview();
            callbacks.current.onClose(updated);
        }
        catch (e) {
            setError(e);
        }
        finally {
            setSaving(false);
        }
    };
    const failure = error ? normalizeOperationFailure(error) : undefined;
    const raw = String((error as {
        message?: string;
    })?.message ?? error ?? ''), needsRevalidation = raw.includes('SIMULATION_REVALIDATION_REQUIRED');
    const sessionMessage = raw.includes('COMMAND_REFERENCE_REVALIDATION_REQUIRED') ? '命令基准已变化，请重新设置零点。' : raw.includes('UNSUPPORTED_DOF') ? '请选择完整且独立的驱动。' : raw.includes('SIMULATION_REVALIDATION_FAILED') ? '记录未通过当前定义的验证。' : raw.includes('mechanism session capacity') ? '请关闭其他运动窗口后重试。' : undefined;
    const problem = needsRevalidation ? <Alert type="warning" showIcon title="模拟需要重新验证。" action={<Button size="small" onClick={() => setRevalidate(true)}>采用当前基准并验证</Button>}/> : failure ? <Alert type="error" showIcon title={sessionMessage ?? failure.message} action={<Space>{!opened && <Button size="small" aria-label="重试" onClick={() => setAttempt(a => a + 1)}>重试</Button>}<Button size="small" onClick={() => feedback(error, '机构运动')}>详情</Button></Space>}/> : null;
    const displayValues=clock&&draft.current?Object.fromEntries(Object.entries(motionTargetAt(draft.current,time)).map(([id,q])=>[id,q.value])):frame?.frame.commandValues??{};
    const ready=Boolean(frame?.qualified);
    const pending = busy || saving || baking || Boolean(preparedCommit.current) || !opened || !ready;
    const inputLocked = switching || saving || baking || Boolean(preparedCommit.current);
    return <>
  <CommandDialog id="dmu.command-motion" open title="使用命令进行模拟" size="S" placement={props.record ? 'left' : undefined} onClose={close} onConfirm={confirm} confirmLoading={saving} confirmDisabled={busy || saving || baking || !opened || !ready} footer={!props.record}>
   <div className="motion-study-fields">
    <Typography.Text type="secondary">使用的命令</Typography.Text>
    {props.drivers.map(driver=>{
        const coordinate=driverCoordinate(props.mechanism,driver);if(!coordinate)return null;
        const active=frame?.activeDriverIds.includes(driver.id)&&ready,checked=selectedDrivers.includes(driver.id),angular=coordinate.dimension==='ANGLE',unit=angular?'°':'mm';
        const range=motionCommandRange(props.mechanism,driver),factor=angular?180/Math.PI:1,lower=range.lower*factor,upper=range.upper*factor;
        const value=(clock?displayValues[driver.id]:inputValues[driver.id]??displayValues[driver.id]??0)*factor;
        const label=props.drivers.length===1?(angular?'命令角度':'命令长度'):`${driver.name} ${angular?'角度':'长度'}`;
        return <section className="motion-dialog-section" key={driver.id}>
            <Checkbox aria-label={`使用 ${driver.name}`} checked={checked} disabled={busy||inputLocked||Boolean(draft.current?.keyframes?.length)||Boolean(clock)} onChange={event=>void selectDriver(driver.id,event.target.checked)}>{props.mechanism.joints.find(j=>j.id===driver.jointId)?.name} · {angular?'角度':'长度'}</Checkbox>
            <label className="motion-labeled-field"><span>{angular?'角度':'长度'} {unit}</span><InputNumber aria-label={label} value={Number(value.toFixed(2))} precision={2} changeOnBlur={false} readOnly={!active} disabled={inputLocked||Boolean(clock)} onChange={value=>{if(value!==null&&active)void request(driver.id,value/factor)}}/></label>
            <Slider aria-label={props.drivers.length===1?'命令游标':`${driver.name} 游标`} value={value} min={lower} max={upper} step={.1} disabled={!active||inputLocked||Boolean(clock)} onChange={value=>void request(driver.id,value/factor)}/>
            <div className="motion-range-labels"><Typography.Text type="secondary">{lower.toFixed(2)} {unit}</Typography.Text><Typography.Text type="secondary">{upper.toFixed(2)} {unit}</Typography.Text></div>
        </section>;
    })}
    {!ready&&opened&&<Typography.Text type="secondary">请选择完整且独立的驱动</Typography.Text>}
    {frame?.failure && <Typography.Text type="warning">该位置未通过求解。</Typography.Text>}
    {frame?.failure?.diagnosticId && <Button size="small" onClick={() => void navigator.clipboard.writeText('CAD_DIAGNOSTIC ' + frame.failure!.diagnosticId)}>复制诊断</Button>}
    {problem}
   </div>
  </CommandDialog>
  {props.record && <CommandDialog id="dmu.record-motion" open title="模拟" size="M" placement="right" onClose={close} onConfirm={confirm} confirmLoading={saving} confirmDisabled={busy || saving || baking || !opened || !study?.keyframes?.length}>
   <div className="motion-study-fields">
    <Input aria-label="模拟名称" value={study?.name ?? ''} disabled={inputLocked || Boolean(clock)} onChange={e => { if (draft.current)
            publishStudy({ ...draft.current, name: e.target.value }, false); }}/>
    <Space><Checkbox checked={automatic} disabled={inputLocked || Boolean(clock)} onChange={e => {autoRef.current=e.target.checked;setAutomatic(e.target.checked);automaticClock.current.anchor(draft.current?.keyframes?.at(-1)?.timeSeconds??0);}}>自动插入</Checkbox><Checkbox checked={camera} disabled={inputLocked || Boolean(clock)} onChange={e => { setCamera(e.target.checked); if (draft.current)
            publishStudy({ ...draft.current, animateViewpoint: e.target.checked }, false); }}>动画视点</Checkbox></Space>
    <label className="motion-labeled-field"><span>插入时间 s</span><InputNumber aria-label="关键帧时间" min={.01} max={3600} precision={2} changeOnBlur={false} step={.2} value={insertTime} disabled={inputLocked || Boolean(clock)} onChange={v => setInsertTime(v ?? .2)}/></label>
    <Space><Button aria-label="插入" disabled={pending || Boolean(clock)} onClick={() => void insert().catch(setError)}>插入</Button><Button aria-label="修改" disabled={pending || Boolean(clock) || !keyId || keyId === study?.keyframes?.[0]?.id} onClick={() => void insert(true).catch(setError)}>修改</Button><Button aria-label="删除" disabled={pending || Boolean(clock) || !keyId || keyId === study?.keyframes?.[0]?.id} onClick={() => { if (draft.current) {
            publishStudy({ ...draft.current, keyframes: draft.current.keyframes?.filter(k => k.id !== keyId) });
            setKeyId(undefined);
        } }}>删除</Button></Space>
    <Table<MotionKeyframe> size="small" rowKey="id" pagination={false} scroll={{ y: 180 }} dataSource={study?.keyframes} columns={[{ title: '时间 s', render: (_, key) => key.timeSeconds.toFixed(2) }, ...(study?.activeDriverIds??[]).map(id=>{const driver=props.drivers.find(d=>d.id===id)!,coordinate=driverCoordinate(props.mechanism,driver)!;return {title:`${driver.name} ${coordinate.dimension==='ANGLE'?'°':'mm'}`,render:(_:unknown,key:MotionKeyframe)=>quantityIn(key.values[id],coordinateUnit(coordinate,true)).toFixed(2)}})]} rowSelection={{ type: 'radio', selectedRowKeys: keyId ? [keyId] : [], onChange: ids => {const id=String(ids[0]);setKeyId(id);const key=draft.current?.keyframes?.find(k=>k.id===id);if(key)setInsertTime(key.timeSeconds); } }} onRow={key => ({ onDoubleClick: () => { setKeyId(key.id);setInsertTime(key.timeSeconds); void seek(key.timeSeconds); } })}/>
    <div className="motion-playback-controls"><Button aria-label="起点" disabled={pending || !study?.keyframes?.length} onClick={() => void seek(0)}>起点</Button><Button aria-label={clock ? '暂停' : '播放'} type="primary" disabled={pending || (study?.keyframes?.length ?? 0) < 2} onClick={() => void play()}>{clock ? '暂停' : '播放'}</Button><Button aria-label="终点" disabled={pending || !study?.keyframes?.length} onClick={() => void seek(study?.keyframes?.at(-1)?.timeSeconds ?? 0)}>终点</Button></div>
    <Slider aria-label="模拟时间游标" min={0} max={study?.keyframes?.at(-1)?.timeSeconds || .01} step={.01} value={time} disabled={pending || Boolean(clock)} onChange={v => void seek(v)}/>
    <label className="motion-labeled-field"><span>定位时间 s</span><InputNumber aria-label="模拟定位时间" value={time} min={0} max={study?.keyframes?.at(-1)?.timeSeconds ?? 0} precision={2} changeOnBlur={false} step={.01} disabled={pending || Boolean(clock)} onChange={v => void seek(v ?? 0)}/></label>
    {run?.failure && <Typography.Text type="warning">路径未完成 · {run.failure.timeSeconds.toFixed(2)} s</Typography.Text>}
   </div>
  </CommandDialog>}
 </>;
}
