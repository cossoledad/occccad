import { useEffect, useRef, useState } from "react";
import type { Artifact, Feature,ReferenceGeometry,ParameterDefinition } from "../../types";
import { api } from "../../api/client";
export function useFeaturePreview(documentId: string, versionId: string, input: Record<string, unknown> | undefined, operation: Feature["operation"], onPreview: (artifact?: Artifact, operation?: Feature["operation"]) => void) {
    const [state, setState] = useState<{
        referenceGeometry?:ReferenceGeometry;parameterCandidates?:ParameterDefinition[];
        key?: string;
        id?: string;
        pending: boolean;
        error?: string;
 failure?:unknown;
        loftSections?: Feature["sections"];
        loftConnections?: [number,number,number][][];
    }>({ pending: false });
    const [retry, setRetry] = useState(0);
    const callback = useRef(onPreview);
    callback.current = onPreview;
    const key = input ? JSON.stringify([documentId, versionId, input]) : undefined;
    const latest = useRef({ input, operation, key });
    latest.current = { input, operation, key };
    useEffect(() => {
        callback.current();
        setState({ key, pending: !!key });
        if (!key)
            return;
        const candidate = latest.current;
        const controller = new AbortController();
        let alive = true;
        const timer = setTimeout(() => {
            void api.previewCommand(documentId, candidate.input!, controller.signal).then(result => {
                if (!alive || latest.current.key !== key || result.baseVersionId !== versionId)
                    return;
                setState({ key, referenceGeometry:result.referenceGeometry,parameterCandidates:result.parameterCandidates,id: result.previewId, pending: false, loftSections:result.loftSections,loftConnections:result.loftConnections });
                callback.current(result.artifact, candidate.operation);
            }).catch(error => { if (alive && !controller.signal.aborted)
                setState({ key, pending: false, error: String(error),failure:error }); });
        }, 250);
        return () => { alive = false; clearTimeout(timer); controller.abort(); callback.current(); };
    }, [key, retry]);
    return {referenceGeometry:state.key===key?state.referenceGeometry:undefined,parameterCandidates:state.key===key?state.parameterCandidates:undefined, loftSections:state.key===key?state.loftSections:undefined,loftConnections:state.key===key?state.loftConnections:undefined, pending: !!key && (state.key !== key || state.pending), previewId: state.key === key ? state.id : undefined,
        failure:state.key===key?state.failure:undefined, error: state.key === key ? state.error : undefined, retry: () => setRetry(value => value + 1) };
}
