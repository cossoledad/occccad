import {contextCommand} from '../../api/context-edit-scope';
import {engineeringProblem} from "../../cad/command/operation-feedback";
import {recordDeveloperEvidence} from "../../cad/command/developer-evidence";
import {PreviewCandidate} from "./preview-candidate";
import {LatestPreviewQueue} from "./latest-preview-queue";
import {randomUUID} from "../../utils/random-uuid";
import { useEffect, useRef, useState } from "react";
import type { Artifact, Feature,ReferenceGeometry,ParameterDefinition } from "../../types";
import { api } from "../../api/client";
export function useFeaturePreview(documentId: string, versionId: string, input: Record<string, unknown> | undefined, operation: Feature["operation"], onPreview: (artifact?: Artifact, operation?: Feature["operation"],splitResult?:Artifact) => void, automatic = true) {
    input=input?contextCommand(documentId,input):undefined;
    const [state, setState] = useState<{
        repairRequired?:{featureId:string;name:string;diagnostic:string}[];referenceGeometry?:ReferenceGeometry;parameterCandidates?:ParameterDefinition[];
        result?:Awaited<ReturnType<typeof api.previewCommand>>;
        key?: string;
        id?: string;
        pending: boolean;
        waitingForBlur?: boolean;
        error?: string;
 failure?:unknown;
        loftSections?: Feature["sections"];
        loftConnections?: [number,number,number][][];
    }>({ pending: false });
    const editingField = useRef<HTMLElement | null>(null);
    const deferred = useRef<(() => void) | undefined>(undefined);
    const setPreviewField = (field: HTMLElement | null) => {
        editingField.current = field;
        if (!field) deferred.current?.();
    };
    const [retry, setRetry] = useState(0);
    const queue=useRef(new LatestPreviewQueue<Awaited<ReturnType<typeof api.previewCommand>>>());
    const scope=JSON.stringify([documentId,versionId,input?.type,input?.targetId]);
    const session=useRef({scope,id:randomUUID()});
    if(session.current.scope!==scope)session.current={scope,id:randomUUID()};
    const callback = useRef(onPreview);
    callback.current = onPreview;
    const key = input ? JSON.stringify([documentId, versionId, input]) : undefined;
    // A session spans many drafts; a request identifies one immutable candidate.
    // Retries of that candidate retain its ID, changed input gets a fresh one.
    const candidateIdentity=useRef({key,id:randomUUID()});
    if(candidateIdentity.current.key!==key)candidateIdentity.current={key,id:randomUUID()};
    const request=input?{...input,requestId:input.requestId??candidateIdentity.current.id,interactionId:input.interactionId??`feature/${session.current.id}`}:undefined;
    const latest = useRef({ input:request, operation, key });
    latest.current = { input:request, operation, key };
    const work=useRef<{key:string;retry:number;task:PreviewCandidate<Awaited<ReturnType<typeof api.previewCommand>>>}|undefined>(undefined);
    const acquire=()=>{
        if(!key||latest.current.key!==key||!latest.current.input)throw new Error("当前输入尚不可提交");
        if(!work.current||work.current.key!==key||work.current.retry!==retry||work.current.task.controller.signal.aborted) {
            const command=latest.current.input;
            work.current={key,retry,task:new PreviewCandidate(signal=>queue.current.submit(
                queuedSignal=>api.previewCommand(documentId,command,queuedSignal),signal))};
        }
        return work.current.task;
    };
    const evaluate=async()=>{
        const expected=key;
        const candidate=acquire();
        const result=await candidate.start();
        if(candidate.controller.signal.aborted||latest.current.key!==expected||result.baseVersionId!==versionId)
            throw new DOMException("预览输入已过期", "AbortError");
        return result;
    };
    const ensure=async()=>{
        const result=await evaluate();
        if(!result.previewId)throw new Error("当前输入未生成可提交的候选结果");
        return result;
    };
    useEffect(() => {
        callback.current();
        const focusedDraft = () => !!editingField.current?.matches(":focus");
        const waitingForBlur = !!key && automatic && focusedDraft();
        setState({key,pending:!!key && automatic && !waitingForBlur,waitingForBlur});
        if(!key)return;
        const candidate=acquire();
        let alive=true;
        // Confirmation bypasses debounce and joins this exact candidate.
        const observe=async()=>{
            try {
                const result=await evaluate();
                if(!alive)return;
                setState({key,result,repairRequired:result.repairRequired,referenceGeometry:result.referenceGeometry,parameterCandidates:result.parameterCandidates,id:result.previewId,pending:false,loftSections:result.loftSections,loftConnections:result.loftConnections});
                callback.current(result.artifact,latest.current.operation,result.splitResultArtifact);
            }catch(error){if(alive&&!candidate.controller.signal.aborted){recordDeveloperEvidence("特征预览",error);setState({key,pending:false,error:engineeringProblem(error),failure:error});}}
        };
        let delayed: ReturnType<typeof setTimeout> | undefined;
        const schedule = () => {
            if (focusedDraft()) {
                deferred.current = schedule;
                return;
            }
            deferred.current = undefined;
            delayed = setTimeout(() => {
                if (focusedDraft()) {
                    deferred.current = schedule;
                    setState({key,pending:false,waitingForBlur:true});
                } else {
                    setState({key,pending:true});
                    void observe();
                }
            }, 250);
        };
        if (automatic) schedule();
        return () => {
            alive = false;
            deferred.current = undefined;
            clearTimeout(delayed);
            candidate.cancel();
            callback.current();
        };
    },[key,retry,automatic]);
    return {setPreviewField,ensure,ready:!!key&&(!automatic||state.key!==key||state.pending||state.waitingForBlur||!!state.id)&&!((state.key===key)&&state.error),result:state.key===key?state.result:undefined,repairRequired:state.key===key?state.repairRequired:undefined,referenceGeometry:state.key===key?state.referenceGeometry:undefined,parameterCandidates:state.key===key?state.parameterCandidates:undefined, loftSections:state.key===key?state.loftSections:undefined,loftConnections:state.key===key?state.loftConnections:undefined, pending: automatic && !!key && (state.key !== key || state.pending), previewId: state.key === key ? state.id : undefined,
        failure:state.key===key?state.failure:undefined, error: state.key === key ? state.error : undefined, retry: () => setRetry(value => value + 1) };
}
