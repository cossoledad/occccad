import {engineeringProblem} from "../../cad/command/operation-feedback";

import { pickedPatternCenter } from "./pattern-selection";
import { PatternFeatureEditor } from "./pattern-feature-editor";
import { Alert, Button, Input, Segmented, Switch } from "antd";
import { useEffect, useRef, useState } from "react";
import { CommandDialog } from "../../cad/overlay/floating-panel";
import { api } from "../../api/client";
import type { Artifact, DocumentView, Feature, FeatureInputResolution, SelectionItem } from "../../types";
import type { FeaturePickRole, FeatureSelectionSession } from "../../cad/interaction/feature-selection";
import { selectionKey } from "../../cad/interaction/selection-identity";
import { parameterSourceText } from "./parameter-editor";
import { solidParameterEdit, solidFeatureNames, pickLoftSection } from "./solid-feature-model";
import { selectedAxis, selectedSketch, sketchPick, profileSelectionIDs } from "./feature-picking";
import { FeaturePickField } from "./feature-pick-field";
import { useFeaturePreview } from "./use-feature-preview";
import {profileRegionCandidates,profileRegionPick} from "./profile-region-selection";
import { randomUUID } from "../../utils/random-uuid";
export type FeatureInsertionContext = {afterFeatureId:string;bodyId:string;versionId:string;label:string};
type BoundPick = {
    id: string;
    resolution: FeatureInputResolution;
    definition: NonNullable<Feature["selections"]>[number];
    visual?: SelectionItem;
};
const inputStatusText = (state: FeatureInputResolution) => ({RESOLVED:"有效",MISSING:"原对象已缺失",AMBIGUOUS:"引用存在歧义",TYPE_MISMATCH:"对象类型不匹配",OUTSIDE_CURRENT_TIP:"原对象已不在上游结果中",SOURCE_UNAVAILABLE:"引用来源不可用",CONTRACT_MISMATCH:"引用与来源不一致"})[state.status];
export function SolidFeatureEditor(props: Parameters<typeof SolidFeatureDefinitionEditor>[0]) {
    return props.feature.pattern ? <PatternFeatureEditor {...props}/> : <SolidFeatureDefinitionEditor {...props}/>;
}
function SolidFeatureDefinitionEditor({ view, feature, digest, unit, seed, occurrencePath, occurrenceContext, intentRequestId, insertion, onClose, onApply, onPreview, onSelectionSession, onInputArtifact }: {
    view: DocumentView;
    feature: Feature;
    digest?: string;
    unit: string;
    seed: SelectionItem[];
    occurrencePath?: string;
    occurrenceContext?: Pick<SelectionItem, "instancePath" | "contextVariantKey" | "rootDocumentId">;
    intentRequestId?:string;
    insertion?:FeatureInsertionContext;
    onClose: () => void;
    onApply: (input: Record<string, unknown>) => Promise<unknown>;
    onPreview: (artifact?: Artifact, operation?: Feature["operation"], sketchFeatureId?:string) => void;
    onSelectionSession: (session?: FeatureSelectionSession) => void;
    onInputArtifact: (artifact?: Artifact | Artifact[]) => void;
}) {
    const hole=feature.type==="HOLE";
    const loft = feature.type === "LOFT", modifier = ["FILLET", "CHAMFER", "DRAFT", "SHELL"].includes(feature.type);
    const angular = feature.type === "REVOLVE" || feature.type === "DRAFT";
    const pickRole: FeaturePickRole = hole ? "sketch-point" : modifier ? (feature.type === "FILLET" || feature.type === "CHAMFER" ? "edge" : "face") : "profile";
    const [showConnections,setShowConnections]=useState(false);
    const acceptedSections=useRef<Feature["sections"]>(undefined);
    const [draft, setDraft] = useState<Feature>({ ...feature, profileSelection:feature.profileSelection??(!feature.id&&!modifier&&!loft&&!hole?{mode:"REGIONS",regionIds:[]}:undefined) });
    const terminating=!modifier&&!loft&&!hole&&draft.extent==="TO_PLANE";
    const explicitRegions = !modifier&&!loft&&!hole&&draft.profileSelection?.mode==="REGIONS";
    const profileRole:FeaturePickRole=hole?"sketch-point":explicitRegions?"profile-region":"profile";
    const [regionReplacement,setRegionReplacement]=useState<string>();
    const [role, setRole] = useState<FeaturePickRole>(!feature.id&&!modifier&&!loft&&!hole||feature.profileSelection?.mode==="REGIONS"?"profile-region":pickRole), [seamIndex, setSeamIndex] = useState<number>();
    const [pointResolution,setPointResolution]=useState<FeatureInputResolution>();
    const [terminationVisual,setTerminationVisual]=useState<SelectionItem>();
    const [terminationLoadError,setTerminationLoadError]=useState<string>();
    const [terminationResolution,setTerminationResolution]=useState<FeatureInputResolution>();
    const [planeInputs,setPlaneInputs]=useState<Artifact[]>([]),[planeErrors,setPlaneErrors]=useState<string[]>([]);
    const [neutralVisual, setNeutralVisual] = useState<SelectionItem>();
    const [neutralResolution, setNeutralResolution] = useState<FeatureInputResolution>();
    const [replacement, setReplacement] = useState<string>();
    const [inputError, setInputError] = useState<string>();
    const [picks, setPicks] = useState<BoundPick[]>([]), [axisVisual, setAxisVisual] = useState<SelectionItem>();
    const [error, setError] = useState<string>(), [committing, setCommitting] = useState(false), [binding, setBinding] = useState(0);
    const [inputContext, setInputContext] = useState<Awaited<ReturnType<typeof api.getFeatureInput>>>();
    const [loadingInput, setLoadingInput] = useState(modifier && (!!feature.id || !!insertion));
    const initial = (slot: string, fallback: number) => {
        const p = view.part?.parameters?.find(p => p.ownerFeatureId === feature.id && p.propertySlot === slot);
        return p ? parameterSourceText(p, slot === "angle" ? "deg" : unit, true) : String(slot === "angle" ? fallback : fallback / (({ mm: 1, cm: 10, m: 1000, in: 25.4 } as Record<string, number>)[unit] ?? 1));
    };
    const original = useRef({diameter:feature.id?initial("diameter",feature.diameter??5):"",depth:feature.id?initial("depth",feature.depth??10):"", length: feature.id ? initial("length", feature.length ?? 10) : "", length2: feature.id && feature.extent === "TWO_SIDED" ? initial("length2", feature.length2 ?? 10) : "", angle: feature.id ? initial("angle", feature.angle ?? 360) : "" });
    const [diameter,setDiameter]=useState(initial("diameter",feature.diameter??5)),[depth,setDepth]=useState(initial("depth",feature.depth??10));
    const [length, setLength] = useState(initial("length", feature.length ?? 10)), [second, setSecond] = useState(initial("length2", feature.length2 ?? 10)), [angle, setAngle] = useState(initial("angle", feature.angle ?? 360));
    const intent = useRef(intentRequestId??randomUUID()), version = useRef(view.document.versionId), alive = useRef(true), queue = useRef(Promise.resolve());
    const callbacks = useRef({ onSelectionSession, onInputArtifact });
    callbacks.current = { onSelectionSession, onInputArtifact };
    const index = feature.id ? view.part!.features.findIndex(f => f.id === feature.id) : insertion ? view.part!.features.findIndex(f=>f.id===insertion.afterFeatureId)+1 : view.part!.features.length;
    const upstream = view.part!.features.slice(0, Math.max(0, index));
    const edit = (patch: Partial<Feature>) => { setError(undefined); setDraft(current => ({ ...current, ...patch })); };
    const validRevision = version.current === view.document.versionId && (!insertion || insertion.versionId===view.document.versionId);
    useEffect(() => { alive.current = true; return () => { alive.current = false; callbacks.current.onSelectionSession(); callbacks.current.onInputArtifact(); }; }, []);
    useEffect(() => {
        if (!feature.id && !insertion)
            return;
        let current = true;
        const controller = new AbortController();
        setLoadingInput(true);
        void api.getFeatureInput(view.document.id, { versionId: view.document.versionId, featureId: feature.id||undefined,afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId }, controller.signal).then(context => {
            if (!current)
                return;
            setInputContext(context);
            setPointResolution(context.pointPick);
            if(context.terminationPick){const p=context.terminationPick;setTerminationResolution(p);if(p.status==="RESOLVED"&&p.localId)setTerminationVisual({...occurrenceContext,kind:"face",id:`input:termination:${p.localId}`,documentId:view.document.id,bodyId:p.bodyId,versionId:context.versionId,geometryKey:p.geometryKey,displayStageFeatureId:p.displayStageFeatureId,topologyId:p.localId,occurrencePath});}
            if (context.neutralPick) {
                const p = context.neutralPick;
                setNeutralResolution(p);
                if (p.status === "RESOLVED" && p.localId) setNeutralVisual({ ...occurrenceContext, kind:"face", id:`input:neutral:${p.localId}`, documentId:view.document.id, bodyId:p.bodyId, versionId:context.versionId, geometryKey:p.geometryKey, displayStageFeatureId:p.displayStageFeatureId, topologyId:p.localId, occurrencePath });
            }
            setPicks(context.picks.map(p => ({ id:`original/${p.index}`, definition: feature.selections![p.index], resolution:p, visual: p.status === "RESOLVED" && p.localId ? { ...occurrenceContext, kind: p.kind.toLowerCase() as "edge" | "face", id: `input:${p.kind}:${p.localId}`, documentId: view.document.id, bodyId: feature.bodyId, displayStageFeatureId:context.sourceFeatureId, versionId: context.versionId, geometryKey: context.artifact.geometryKey, topologyId: p.localId, occurrencePath } : undefined })));
        }).catch(cause => { if (current && !controller.signal.aborted)
            if(feature.extent==="TO_PLANE")setTerminationLoadError(String(cause));else setInputError(String(cause)); }).finally(() => { if (current)
            setLoadingInput(false); });
        return () => { current = false; controller.abort(); };
    }, []);
    useEffect(()=>{
        if(!feature.id||!terminating){setPlaneInputs([]);setPlaneErrors([]);return;}
        const controller=new AbortController();let active=true;
        void Promise.allSettled(view.part!.bodies.map(b=>api.getFeatureInput(view.document.id,{versionId:version.current,featureId:feature.id,bodyId:b.id},controller.signal))).then(results=>{
            if(!active)return;const inputs:Artifact[]=[],errors:string[]=[];
            results.forEach((r,i)=>{if(r.status==="fulfilled"){if(r.value.sourceFeatureId)inputs.push(r.value.artifact);}else errors.push(`${view.part!.bodies[i].name}: ${String(r.reason)}`)});
            setPlaneInputs(inputs);setPlaneErrors(errors);
        });
        return()=>{active=false;controller.abort()};
    },[terminating]);
    useEffect(()=>{
        if(!inputContext&&!planeInputs.length)return;
        const artifacts=[...(inputContext?(inputContext.artifacts??[inputContext.artifact]):[]),...planeInputs];
        callbacks.current.onInputArtifact([...new Map(artifacts.map(a=>[`${a.bodyId}/${a.geometryKey}`,a])).values()]);
    },[inputContext,planeInputs]);
    const current = useRef({ draft, role, inputContext, upstream, committing, validRevision, picks });
    current.current = { draft, role, inputContext, upstream, committing, validRevision, picks };
    const bindPick = (selection: SelectionItem, replaceID?: string) => {
        setBinding(n => n + 1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || current.current.committing)
                return;
            const state = current.current;
            if ((selection.kind !== "edge" && selection.kind !== "face") || selection.kind !== pickRole || selection.bodyId !== state.draft.bodyId || !selection.geometryKey)
                return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id || insertion) {
                if (!state.inputContext || selection.geometryKey !== state.inputContext.artifact.geometryKey)
                    return;
                const result = await api.getFeatureInput(view.document.id, { versionId: version.current, featureId: feature.id||undefined, afterFeatureId:insertion?.afterFeatureId,bodyId:insertion?.bodyId, geometryKey: selection.geometryKey, kind: selection.kind.toUpperCase(), localId: selection.topologyId });
                if (!result.selection)
                    throw new Error("所选边面没有可用的持久引用");
                definition = result.selection;
            }
            else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey, selection.kind.toUpperCase() as "EDGE" | "FACE", selection.topologyId, selection.versionId ?? version.current);
                if (!result.persistentSelection)
                    throw new Error("所选边面没有可用的持久引用");
                definition = { selection: result.persistentSelection, sourceVersionId: selection.versionId ?? version.current };
            }
            if (!alive.current || !current.current.validRevision)
                return;
            const key = selectionKey(selection);
            if (replaceID && current.current.picks.some(p=>p.id!==replaceID && p.visual && selectionKey(p.visual)===key)) {
                setError("该对象已在输入中，请选择其他对象或明确移除失效项");
                return;
            }
            setPicks(previous => {
                const bound: BoundPick = {id:replaceID??randomUUID(),definition,visual:selection,resolution:{status:"RESOLVED"}};
                if (replaceID) return previous.map(p=>p.id===replaceID?bound:p);
                return previous.some(p=>p.visual && selectionKey(p.visual)===key) ? previous.filter(p=>!p.visual || selectionKey(p.visual)!==key) : [...previous,bound];
            });
            setReplacement(previous=>previous===replaceID?undefined:previous);
            setError(undefined);
        }).catch(cause => { if (alive.current)
            setError(String(cause)); }).finally(() => { if (alive.current)
            setBinding(n => n - 1); });
    };
    const bindNeutralPlane = (selection: SelectionItem, termination=false) => {
        if (selection.kind !== "face" || !selection.geometryKey) return;
        setBinding(n => n+1);
        queue.current = queue.current.then(async () => {
            if (!alive.current || !current.current.validRevision || current.current.committing) return;
            let definition: NonNullable<Feature["selections"]>[number];
            if (feature.id) {
                const result = await api.getFeatureInput(view.document.id, { versionId:version.current, featureId:feature.id, bodyId:selection.bodyId, geometryKey:selection.geometryKey, kind:"FACE", localId:selection.topologyId });
                if (!result.selection || result.selection.selection.creationEvidence.geometryType !== "PLANE") throw new Error(termination?"终止目标必须是平面":"中性平面必须是平面");
                definition = result.selection;
            } else {
                const result = await api.getTopologyProperties(view.document.id, selection.geometryKey!, "FACE", selection.topologyId, version.current);
                if (result.geometryType !== "PLANE" || !result.persistentSelection) throw new Error("请选择可绑定的实体平面");
                definition = { selection:result.persistentSelection, sourceVersionId:version.current };
            }
            if (!alive.current || !current.current.validRevision) return;
            if(termination){if(current.current.draft.extent!=="TO_PLANE")return;edit({terminationPlaneId:undefined,terminationPlane:definition});setTerminationResolution({status:"RESOLVED"});setTerminationLoadError(undefined);setTerminationVisual(selection);setRole(profileRole);}
            else {edit({neutralPlaneId:undefined, neutralPlane:definition}); setNeutralResolution({status:"RESOLVED"}); setNeutralVisual(selection); setRole(pickRole);}
        }).catch(cause => { if (alive.current) setError(String(cause)); }).finally(()=>{if(alive.current)setBinding(n=>n-1);});
    };
    const pick = (selection: SelectionItem, chosenRole: FeaturePickRole = current.current.role) => {
        if ((selection.ownerDocumentId ?? selection.documentId) !== view.document.id || (selection.occurrencePath ?? "") !== (occurrencePath ?? "") || selection.versionId && selection.versionId !== version.current || committing || !validRevision)
            return;
        if(hole&&chosenRole==="sketch-point") {
            if(selection.kind!=="visual")return;
            const sketch=upstream.find(f=>f.id===selection.featureId&&f.type==="SKETCH"&&!f.suppressed);
            const entity=sketch?.sketch?.entities.find(e=>e.id===selection.entityId&&e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed);
            if(selection.kind!=="visual"||selection.visualType!=="POINT"||!sketch||!entity||["FAILED_SUPPORT","BLOCKED"].includes(sketch.sketch!.support.status??""))return;
            edit({holePoint:{sketchId:sketch.id,reference:{target:"ENTITY",entityId:entity.id,subElement:"POINT"}}});setPointResolution({status:"RESOLVED"});return;
        }
        if (chosenRole === "edge" || chosenRole === "face") {
            bindPick(selection, replacement);
            return;
        }
        if (chosenRole === "point" && loft) {
            const point=pickedPatternCenter(selection);
            if(!point || point.sketchId && !upstream.some(f=>f.id===point.sketchId))return;
            const section={sketchId:point.sketchId??"",point};
            edit({sections:[...(draft.sections??[]).filter(s=>JSON.stringify(s.point)!==JSON.stringify(point)),section]});
            setRole("profile");
            return;
        }
        if(chosenRole === "profile-region" && selection.kind==="profile-region") {
            const candidate=profileRegionCandidates(view,upstream,draft.profile).find(c=>c.sketchId===selection.featureId);
            if(!candidate?.regions.some(r=>r.id===selection.regionId))return;
            const ids=draft.profileSelection?.regionIds??[];
            if(regionReplacement&&ids.includes(selection.regionId)&&regionReplacement!==selection.regionId){setError("该区域已在输入中，请选择其他区域或明确移除失效项");return;}
            const next=regionReplacement?ids.map(id=>id===regionReplacement?selection.regionId:id):ids.includes(selection.regionId)?ids.filter(id=>id!==selection.regionId):[...ids,selection.regionId];
            edit({profile:selection.featureId,profileMemberSlot:undefined,profileSelection:{mode:"REGIONS",regionIds:next}});setRegionReplacement(undefined);
            return;
        }
        if (chosenRole === "profile") {
            const sketch = selectedSketch(view, selection, upstream);
            if (!sketch)
                return;
            if (loft)
                setDraft(previous => ({ ...previous, sections: pickLoftSection(previous.sections ?? [], sketch) }));
            else {
                edit({ profile: sketch.id, profileMemberSlot:sketch.type==="SKETCH_PATTERN"?sketch.profileMemberSlot:undefined, profileSelection:explicitRegions?{mode:"REGIONS",regionIds:[...(view.sketchAnalyses?.[sketch.id]?.regionIds??[])]}:draft.profileSelection });
                if (feature.type === "REVOLVE" && !draft.axisEntityId)
                    setRole("axis");
            }
        }
        else if (chosenRole === "axis") {
            const axis = selectedAxis(view, selection, upstream);
            if (!axis) {
                setError("请选择草图中的直线或基准轴");
                return;
            }
            edit({ axisEntityId: axis });
            setAxisVisual(selection);
        }
        else if (chosenRole === "plane" && selection.kind === "plane") {
            const plane = view.part?.datumPlanes.find(p => p.id === selection.datumPlane?.id || p.id === selection.entityId || p.id === selection.id);
            if (plane) {
                if(terminating){edit({terminationPlaneId:plane.id,terminationPlane:undefined});setTerminationResolution({status:"RESOLVED"});setTerminationLoadError(undefined);setTerminationVisual(selection);setRole(profileRole);return;}
                edit({ neutralPlaneId: plane.id, neutralPlane: undefined });
                setNeutralResolution({status:"RESOLVED"});
                setNeutralVisual(selection);
                setRole(pickRole);
            }
        }
        else if (chosenRole === "plane" && selection.kind === "face") { bindNeutralPlane(selection,terminating); }
        else if (chosenRole === "seam" && selection.kind === "visual" && seamIndex !== undefined) {
            edit({ sections: (acceptedSections.current??draft.sections)?.map((s, i) => i === seamIndex && (s.sketchId === selection.featureId || s.sketchId===selection.patternId&&s.memberSlot===selection.patternMemberSlot) ? { ...s, correspondenceResolved:true, seamEntityId: selection.entityId } : s) });
            setRole("profile");
        }
        else if (chosenRole === "body" && selection.bodyId) {
            edit({ bodyId: selection.bodyId });
            setRole(profileRole);
        }
    };
    const seedOnce = useRef(false);
    useEffect(() => {
        if (seedOnce.current || feature.id || insertion)
            return;
        seedOnce.current = true;
        if(hole){
            const points=seed.filter(s=>s.kind==="visual"&&s.visualType==="POINT"&&upstream.some(f=>f.id===s.featureId&&f.type==="SKETCH"&&!f.suppressed&&f.sketch?.entities.some(e=>e.id===s.entityId&&e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed)));
            if(points.length===1)pick(points[0],"sketch-point");
        }
        else if (modifier)
            seed.forEach(s => pick(s, pickRole));
        else if (loft)
            seed.forEach(s => pick(s, "profile"));
        else {
            const sketch = seed.map(s => selectedSketch(view, s, upstream)).find(Boolean);
            if (sketch)
                edit({ profile: sketch.id, profileMemberSlot:sketch.type==="SKETCH_PATTERN"?sketch.profileMemberSlot:undefined, profileSelection:explicitRegions?{mode:"REGIONS",regionIds:[...(view.sketchAnalyses?.[sketch.id]?.regionIds??[])]}:draft.profileSelection });
            const axis = seed.find(s => selectedAxis(view, s, upstream));
            if (axis)
                pick(axis, "axis");
            else if (sketch && feature.type === "REVOLVE")
                setRole("axis");
        }
    }, []);
    const pickRef = useRef(pick);
    pickRef.current = pick;
    const axisParts = draft.axisEntityId?.split(":");
    const restoredAxis: SelectionItem | undefined = axisParts?.[0] === "SKETCH_LINE" ? {
        kind: "visual", visualType: "CURVE", id: `${occurrencePath || "root"}:${axisParts[1]}:${axisParts[2]}`, featureId: axisParts[1], entityId: axisParts[2], bodyId: upstream.find(f => f.id === axisParts[1])?.bodyId, documentId: view.document.id, versionId: version.current, occurrencePath
    } : undefined;
    const holeSource=upstream.find(f=>f.id===draft.holePoint?.sketchId&&!f.suppressed),holeEntity=holeSource?.sketch?.entities.find(e=>e.id===draft.holePoint?.reference.entityId&&e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed);
    const holeReady=!!holeEntity&&!(["FAILED_SUPPORT","BLOCKED"].includes(holeSource?.sketch?.support.status??""))&&(!pointResolution||pointResolution.status==="RESOLVED");
    const highlights: SelectionItem[] = hole ? holeReady?[{kind:"visual",visualType:"POINT",id:`${occurrencePath||"root"}:${holeSource!.id}:${holeEntity!.id}`,featureId:holeSource!.id,entityId:holeEntity!.id,sketchReference:draft.holePoint!.reference,bodyId:holeSource!.bodyId,documentId:view.document.id,versionId:version.current,occurrencePath}]:[] : modifier ? picks.flatMap(p => p.visual ? [p.visual] : []) : loft ? (draft.sections ?? []).map(s => s.point?.sketchId ? ({kind:"visual",visualType:"POINT",id:`loft-point:${s.sketchId}:${JSON.stringify(s.point.reference)}`,featureId:s.sketchId,entityId:s.point.reference.entityId??s.point.reference.target,sketchReference:s.point.reference,documentId:view.document.id,versionId:version.current,occurrencePath} as SelectionItem) : s.point?.axisEntityId ? ({kind:"axis-system",id:s.point.axisEntityId.split(":")[1],entityId:s.point.axisEntityId.split(":")[1],documentId:view.document.id,versionId:version.current,occurrencePath} as SelectionItem) : sketchPick(view, s.sketchId, occurrencePath, s.memberSlot)) : draft.profile ? explicitRegions ? (draft.profileSelection?.regionIds??[]).map(id=>profileRegionPick(view,draft.profile!,id,occurrencePath)) : [sketchPick(view, draft.profile, occurrencePath, draft.profileMemberSlot)] : [];
    if (axisVisual ?? restoredAxis)
        highlights.push((axisVisual ?? restoredAxis)!);
    if (neutralVisual) highlights.push(neutralVisual);
    if(terminating&&terminationVisual)highlights.push(terminationVisual);
    const contextSelections = feature.type === "REVOLVE" && role === "axis" ? highlights.filter(s => s.kind === "sketch"||s.kind==="profile-region") : [];
    const selections = contextSelections.length ? highlights.filter(s => s.kind !== "sketch"&&s.kind!=="profile-region") : highlights;
    const highlightsToken = JSON.stringify([selections, contextSelections]);
    const seamSection=seamIndex!==undefined?draft.sections?.[seamIndex]:undefined;
    const sketchIds = hole ? upstream.filter(f=>f.type==="SKETCH"&&f.sketch&&!f.suppressed&&!["FAILED_SUPPORT","BLOCKED"].includes(f.sketch.support.status??"")).map(f=>f.id) : role === "seam" && seamSection ? seamSection.memberSlot!==undefined?profileSelectionIDs(view,upstream,seamSection.sketchId,seamSection.memberSlot):[seamSection.sketchId] : profileSelectionIDs(view,upstream);
    const regionCandidates=!modifier&&!loft&&!hole?profileRegionCandidates(view,upstream,explicitRegions?draft.profile:undefined):[];
    const regionIDs=draft.profileSelection?.regionIds??[];
    const availableRegionIDs=regionCandidates.find(c=>c.sketchId===draft.profile)?.regions.map(r=>r.id)??[];
    const profileAnalysis=draft.profile?view.sketchAnalyses?.[draft.profile]:undefined;
    const profileAnalysisError=explicitRegions&&draft.profile&&(!profileAnalysis?.geometryVerified||profileAnalysis.status!=="CLOSED") ? profileAnalysis?.issues.map(issue=>issue.message).join("；")||"草图区域尚未通过精确验证" : undefined;
    const missingRegions=explicitRegions?regionIDs.filter(id=>!availableRegionIDs.includes(id)):[];
    const sketchIdsToken = JSON.stringify(sketchIds);

    let input: Record<string, unknown> | undefined, parameterError: string | undefined;
    const unresolved = picks.filter(p=>p.resolution.status!=="RESOLVED");
    const ready = (!terminating||!terminationLoadError&&!!(draft.terminationPlaneId||draft.terminationPlane)&&(!terminationResolution||terminationResolution.status==="RESOLVED")) && (!explicitRegions||regionIDs.length>0&&!missingRegions.length) && validRevision && !loadingInput && !binding && !inputError && (!(modifier||hole) || !feature.id && !insertion || !!inputContext) && !unresolved.length && (!neutralResolution || neutralResolution.status==="RESOLVED") && (hole ? holeReady&&!!draft.bodyId : loft ? (draft.sections?.length ?? 0) >= 2 && !!draft.sections?.some(s=>!s.point) && !draft.sections?.some((s,i)=>s.point&&i!==0&&i!==draft.sections!.length-1) : modifier ? picks.length > 0 && !!draft.bodyId && (feature.type !== "DRAFT" || !!draft.neutralPlaneId || !!draft.neutralPlane) : !!draft.profile && (feature.type !== "REVOLVE" || !!draft.axisEntityId));
    try {
        const value = { ...draft, selections: modifier ? picks.map(p => p.definition) : draft.selections }, expressions: Record<string, string> = {};
        const read = (slot: "length" | "length2" | "angle" | "diameter" | "depth", text: string) => { const parsed = solidParameterEdit(slot, text, original.current[slot], unit); if (parsed.value !== undefined)
            value[slot] = parsed.value; if (parsed.expression !== undefined)
            expressions[slot] = parsed.expression; };
        if(hole){read("diameter",diameter);if(draft.extent==="BLIND")read("depth",depth);}
        else if (!loft) {
            if (angular)
                read("angle", angle);
            else if (draft.extent !== "THROUGH_ALL" && !terminating) {
                read("length", length);
                if (draft.extent === "TWO_SIDED")
                    read("length2", second);
            }
        }
        const common = { requestId: intent.current, parameterExpressions: expressions };
        if (ready)
            input = feature.id ? { ...common, type: "EDIT_FEATURE", targetId: feature.id, expectedFeatureDigest: digest, feature: value } : modifier ? { ...common, type: "CREATE_MODIFY_FEATURE", feature: value,afterFeatureId:insertion?.afterFeatureId,versionId:insertion?.versionId,geometryKey:insertion?inputContext?.artifact.geometryKey:undefined } : loft||hole ? { ...common, type: "CREATE_SOLID_FEATURE", feature: value } : { ...common, type: "CREATE_SOLID_FEATURE", terminationPlaneId:value.terminationPlaneId,terminationPlane:value.terminationPlane,sketchId: value.profile, profileSelection:value.profileSelection, profileMemberSlot:value.profileMemberSlot, generator: value.type, bodyId: value.bodyId, operation: value.operation, length: value.length ?? 1, length2: value.length2, angle: value.angle ?? 360, extent: value.extent, axisEntityId: value.axisEntityId, reversed: value.reversed };
    }
    catch (cause) {
        parameterError = String(cause);
    }
    const planeGeometryKeys=feature.id?planeInputs.map(a=>a.geometryKey):view.part!.bodies.filter(b=>b.geometryKey).map(b=>b.geometryKey!);
    const preview = useFeaturePreview(view.document.id, view.document.versionId, input, draft.operation, onPreview);
    acceptedSections.current=preview.loftSections;
    useEffect(() => {
        callbacks.current.onSelectionSession({ ...occurrenceContext,role,sketchPoints:hole?upstream.filter(f=>sketchIds.includes(f.id)).flatMap(f=>f.sketch?.entities.filter(e=>e.kind==="POINT"&&!e.sourceEntityId&&!e.suppressed).map(e=>({sketchId:f.id,entityId:e.id}))??[]):undefined,planeGeometryKeys:terminating?planeGeometryKeys:undefined, profileRegions:explicitRegions?regionCandidates:undefined, connectionLines:showConnections?preview.loftConnections:undefined, documentId: view.document.id, versionId: version.current, occurrencePath, bodyId: modifier ? draft.bodyId : undefined, geometryKey:modifier && (feature.id || insertion) ? inputContext?.artifact.geometryKey : undefined, sketchIds, contextSelections: contextSelections.map(s => ({ ...occurrenceContext, ...s })), selections: selections.map(s => ({ ...occurrenceContext, ...s })), onPick: s => pickRef.current(s) });
    }, [role, highlightsToken, sketchIdsToken, draft.bodyId,inputContext?.artifact.geometryKey,showConnections,JSON.stringify(preview.loftConnections),explicitRegions,draft.profile,terminating,JSON.stringify(planeGeometryKeys)]);
    const apply = async () => { if (!input || !preview.previewId || preview.pending || committing)
        return; setCommitting(true); try {
        await onApply({ ...input, previewId: preview.previewId });
        onClose();
    }
    catch (cause) {
        setError(String(cause));
        setCommitting(false);
    } };
    const name = (id?: string) => upstream.find(f => f.id === id)?.name ?? (id?"引用草图已丢失":"未选择");
    return <CommandDialog id="solid-feature-editor" open title={`${feature.id ? "编辑" : "创建"} ${solidFeatureNames[feature.type] ?? feature.type}`} size="S" onClose={onClose} onConfirm={apply} confirmLoading={committing} confirmDisabled={!input || !preview.previewId || preview.pending || committing}>
  <fieldset disabled={committing} className="instance-pattern-fields feature-input-fields">
   {insertion&&<Alert type="info" title={`插入位置：${insertion.label} 之后`} description={`新建${solidFeatureNames[feature.type]}，仅拾取此步骤的实体；完整候选通过后一次提交。`}/>}
   {hole&&<>
    <FeaturePickField label="定位点" value={draft.holePoint?holeReady?`${name(draft.holePoint.sketchId)} · 已选择显式点`:pointResolution?inputStatusText(pointResolution):"原定位点已失效":"选择上游普通草图的显式点"} active={role==="sketch-point"} onActivate={()=>setRole("sketch-point")} onClear={draft.holePoint?()=>{edit({holePoint:undefined});setPointResolution(undefined)}:undefined}/>
    {draft.holePoint&&!holeReady&&<Alert type="warning" title="孔位需要修复" description="请明确重选上游草图点；支撑失效时请先修复定位草图。"/>}
    <small>孔轴沿定位草图的支撑法向，可反向；孔位通过草图尺寸和约束修改。</small>
    <FeaturePickField label="目标实体" value={view.part?.bodies.find(b=>b.id===draft.bodyId)?.name??"选择实体"} disabled={!!feature.id} active={role==="body"} onActivate={()=>setRole("body")}/>
    <label>直径（{unit}）<Input value={diameter} onChange={e=>setDiameter(e.target.value)}/></label>
    <label>终止<Segmented value={draft.extent} options={[{value:"BLIND",label:"平底盲孔"},{value:"THROUGH_ALL",label:"单向贯穿"}]} onChange={extent=>edit({extent:extent as Feature["extent"]})}/></label>
    {draft.extent==="BLIND"&&<label>深度（{unit}）<Input value={depth} onChange={e=>setDepth(e.target.value)}/></label>}
   </>}
   {!modifier && !loft && !hole && <FeaturePickField label="轮廓" value={draft.profile ? name(draft.profile) : explicitRegions?"请在视图区点击闭合区域":"请在视图区选择草图"} active={role === profileRole} onActivate={() => setRole(profileRole)} onClear={draft.profile ? () => {edit({ profile: undefined,profileMemberSlot:undefined,profileSelection:explicitRegions?{mode:"REGIONS",regionIds:[]}:draft.profileSelection });setRegionReplacement(undefined);} : undefined}/>}
   {!modifier&&!loft&&!hole&&<>
    <label>轮廓范围<Segmented value={explicitRegions?"REGIONS":"WHOLE_SKETCH"} options={[{value:"REGIONS",label:"选定区域"},{value:"WHOLE_SKETCH",label:"整张草图"}]} onChange={mode=>{edit({profileSelection:mode==="REGIONS"?{mode:"REGIONS",regionIds:[]}:{mode:"WHOLE_SKETCH"}});setRegionReplacement(undefined);setRole(mode==="REGIONS"?"profile-region":"profile");}}/></label>
    <small>{explicitRegions?"固定区域集合；点击区域增减选择，不会吸收后来新增的独立区域。":"整张草图：包含求值时全部合法闭合区域，包括后来新增区域。"}</small>
    {explicitRegions&&<>
     <FeaturePickField label="区域集" value={`已选择 ${regionIDs.length} 个区域${missingRegions.length?`，待修复 ${missingRegions.length} 项`:""}`} active={role==="profile-region"&&!regionReplacement} onActivate={()=>{setRegionReplacement(undefined);setRole("profile-region");}}/>
     <Button size="small" disabled={!draft.profile||!availableRegionIDs.length} onClick={()=>{edit({profileSelection:{mode:"REGIONS",regionIds:[...availableRegionIDs]}});setRegionReplacement(undefined);}}>全选当前区域（固定集合）</Button>
     {regionIDs.map((id,i)=><div className="feature-reference-row" key={id} data-region-status={availableRegionIDs.includes(id)?"RESOLVED":"MISSING"}>
      <span className="feature-reference-detail"><strong>区域 {i+1}</strong><span>{regionReplacement===id?"请点击替代区域":availableRegionIDs.includes(id)?"有效":profileAnalysisError?"上游草图不可用":"区域已失效，请重新选择"}</span></span>
      <Button size="small" aria-label={`替换区域 ${i+1}`} onClick={()=>{setRegionReplacement(id);setRole("profile-region");}}>替换</Button>
      <Button size="small" aria-label={`移除区域 ${i+1}`} onClick={()=>{edit({profileSelection:{mode:"REGIONS",regionIds:regionIDs.filter(r=>r!==id)}});setRegionReplacement(undefined);}}>移除</Button>
     </div>)}
     {profileAnalysisError&&<Alert type="error" title="上游草图无法提供合法区域" description={engineeringProblem(profileAnalysisError)}/>}
     {missingRegions.length>0&&!profileAnalysisError&&<Alert type="warning" title="区域输入需要修复" description="原区域删除或拓扑改变后无法对应。请明确替换或移除失效区域；不会自动换区或使用整张草图。"/>}
    </>}
   </>}
   {draft.profile && upstream.find(f=>f.id===draft.profile)?.type==="SKETCH_PATTERN" && <label>草图成员槽位<Input type="number" min={0} step={1} value={draft.profileMemberSlot} onChange={e=>edit({profileMemberSlot:e.target.value===""?undefined:Number(e.target.value)})}/></label>}
   {feature.type === "REVOLVE" && <FeaturePickField label="旋转轴" value={draft.axisEntityId ? "已选择 1 条轴线" : "请在视图区选择直线或轴"} active={role === "axis"} onActivate={() => setRole("axis")} onClear={draft.axisEntityId ? () => { edit({ axisEntityId: undefined }); setAxisVisual(undefined); } : undefined}/>}
   {modifier && <FeaturePickField label={pickRole === "edge" ? "边集" : "面集"} value={loadingInput ? "正在恢复选择…" : unresolved.length ? `有效 ${picks.length-unresolved.length} 项，待修复 ${unresolved.length} 项` : `已选择 ${picks.length} ${pickRole === "edge" ? "条边" : "个面"}`} active={role === pickRole && !replacement} disabled={loadingInput || !!inputError || !!binding} onActivate={() => {setReplacement(undefined);setRole(pickRole);}} onClear={picks.length && !binding ? () => {setPicks([]);setReplacement(undefined);} : undefined}/>}
   {modifier && feature.id && picks.length>0 && <details open={unresolved.length>0 || !!replacement}>
    <summary>输入明细（{picks.length} 项）</summary>
    {picks.map((p,i)=><div className="feature-reference-row" key={p.id} data-input-status={p.resolution.status}>
     <span className="feature-reference-detail" title={inputStatusText(p.resolution)}><strong>{pickRole==="edge"?"边":"面"} {i+1}</strong><span>{replacement===p.id?"请在上游几何中选择替代对象":inputStatusText(p.resolution)}</span></span>
     <Button size="small" aria-label={`替换输入 ${i+1}`} aria-pressed={replacement===p.id} disabled={!!binding} onClick={()=>{setReplacement(p.id);setRole(pickRole);}}>替换</Button>
     <Button size="small" aria-label={`移除输入 ${i+1}`} disabled={!!binding} onClick={()=>{setPicks(previous=>previous.filter(item=>item.id!==p.id));setReplacement(undefined);}}>移除</Button>
    </div>)}
   </details>}
   {unresolved.length>0 && <Alert type="warning" title="输入需要修复" description="请明确替换或移除每个失效输入。有效输入会保留；未处理前不能预览或确认。"/>}
   {terminating&&<>
    <FeaturePickField label="终止平面" value={terminationLoadError?"原目标解析受阻，请重新选择":terminationResolution&&terminationResolution.status!=="RESOLVED"?inputStatusText(terminationResolution):draft.terminationPlane?"已选择上游实体平面":view.part?.datumPlanes.find(p=>p.id===draft.terminationPlaneId)?.name??"选择基准平面或上游实体平面"} active={role==="plane"} disabled={loadingInput||!!binding} onActivate={()=>setRole("plane")} onClear={draft.terminationPlane||draft.terminationPlaneId?()=>{edit({terminationPlane:undefined,terminationPlaneId:undefined});setTerminationResolution(undefined);setTerminationLoadError(undefined);setTerminationVisual(undefined);}:undefined}/>
    <small>到平面：使用目标的无限延伸支撑平面，随目标位置和朝向更新。</small>
    {terminationResolution&&terminationResolution.status!=="RESOLVED"&&<Alert type="warning" title="终止平面需要修复" description="请明确重新选择当前特征之前的合法平面。"/>}
    {terminationLoadError&&<Alert type="error" title="原终止目标解析失败" description={engineeringProblem(terminationLoadError)}/>}
    {planeErrors.map((text,i)=><Alert key={i} type="error" title="部分上游输入不可用" description={engineeringProblem(text)}/>)}
   </>}
   {feature.type === "DRAFT" && <>
    <FeaturePickField label="中性平面" value={neutralResolution && neutralResolution.status!=="RESOLVED" ? inputStatusText(neutralResolution) : draft.neutralPlane ? "已选择实体平面" : view.part?.datumPlanes.find(p => p.id === draft.neutralPlaneId)?.name ?? "选择基准面或实体平面"} active={role === "plane"} disabled={loadingInput || !!inputError || !!binding} onActivate={() => {setReplacement(undefined);setRole("plane");}} onClear={(draft.neutralPlane || draft.neutralPlaneId) && !binding ? ()=>{edit({neutralPlane:undefined,neutralPlaneId:undefined});setNeutralVisual(undefined);setNeutralResolution(undefined);} : undefined}/>
    {neutralResolution && neutralResolution.status!=="RESOLVED" && <Alert type="warning" title="中性平面需要修复" description="请选择当前特征之前的基准面或实体平面。"/>}
   </>}

   {loft && <>
    <FeaturePickField label="截面" value={`已选择 ${draft.sections?.length ?? 0} 个截面`} active={role === "profile"} onActivate={() => setRole("profile")} onClear={draft.sections?.length ? () => edit({ sections: [] }) : undefined}/>
    <FeaturePickField label="点端" value="选择草图点或基准原点" active={role === "point"} onActivate={()=>setRole("point")}/>
    <label>显示连接线<Switch checked={showConnections} onChange={setShowConnections}/></label>
    {(preview.loftSections ?? draft.sections ?? []).map((section, i) => <div className="feature-section-row" key={`${section.sketchId}/${i}`}>
     <span>{i + 1}. {section.point ? `点端 · ${(section.point.axisEntityId?view.part?.axisSystems.find(a=>a.id===section.point?.axisEntityId?.split(":")[1])?.name??"引用原点已丢失":name(section.sketchId))}` : name(section.sketchId)}</span>
     {upstream.find(f=>f.id===section.sketchId)?.type==="SKETCH_PATTERN"&&<label>成员槽位<Input type="number" min={0} step={1} value={section.memberSlot} onChange={e=>edit({sections:(acceptedSections.current??draft.sections)?.map((s,j)=>i===j?{...s,memberSlot:e.target.value===""?undefined:Number(e.target.value)}:s)})}/></label>}
     <Button size="small" aria-label={`上移截面 ${i + 1}`} disabled={!i} onClick={() => { const sections = [...draft.sections!]; [sections[i - 1], sections[i]] = [sections[i], sections[i - 1]]; edit({ sections }); }}>↑</Button>
     <Button size="small" aria-label={`移除截面 ${i + 1}`} onClick={() => edit({ sections: draft.sections?.filter((_, j) => j !== i) })}>移除</Button>
     {!section.point && <><Button size="small" onClick={()=>edit({sections:(acceptedSections.current??draft.sections)?.map((s,j)=>i===j?{...s,correspondenceResolved:false,seamEntityId:undefined,seamAngle:0,reversed:false}:s)})}>自动对应</Button>
     <label>反向<Switch checked={section.reversed} onChange={reversed => edit({ sections: (acceptedSections.current??draft.sections)?.map((s, j) => i === j ? { ...s, correspondenceResolved:true, reversed } : s) })}/></label>
     <Button size="small" aria-pressed={role === "seam" && seamIndex === i} onClick={() => { setSeamIndex(i); setRole("seam"); }}>选择闭合起始边</Button>
     {upstream.find(f => f.id === section.sketchId)?.sketch?.entities.some(e => e.kind === "CIRCLE") && <label>闭合点角度（deg）<Input type="number" value={section.seamAngle ?? 0} onChange={e => edit({ sections: (acceptedSections.current??draft.sections)?.map((s, j) => i === j ? { ...s, correspondenceResolved:true, seamAngle: Number(e.target.value) } : s) })}/></label>}
     </>}
    </div>)}
    <label>截面连接<Segmented value={draft.ruled ? "ruled" : "smooth"} options={[{ value: "smooth", label: "平滑" }, { value: "ruled", label: "直纹" }]} onChange={v => edit({ ruled: v === "ruled" })}/></label>
   </>}
   {!modifier && !hole && <>
    <label>材料<Segmented value={draft.operation} options={[...(!feature.id ? [{ value: "NEW_BODY", label: "新实体" }] : []), { value: "ADD", label: "添加" }, { value: "REMOVE", label: "切除" }, { value: "INTERSECT", label: "交集" }]} onChange={operation => edit({ operation: operation as Feature["operation"], extent: operation !== "REMOVE" && draft.extent === "THROUGH_ALL" || operation==="INTERSECT"&&terminating ? "FINITE" : draft.extent, ...(operation==="INTERSECT"?{terminationPlane:undefined,terminationPlaneId:undefined}:{}) })}/></label>
    {draft.operation !== "NEW_BODY" && <FeaturePickField label="目标实体" value={view.part?.bodies.find(b => b.id === draft.bodyId)?.name ?? "请在视图区选择实体"} disabled={!!feature.id||!!insertion} active={role === "body"} onActivate={() => setRole("body")}/>}
   </>}
   {!loft && !hole && (angular ? <label>角度（deg）<Input value={angle} disabled={committing} onChange={e => setAngle(e.target.value)}/></label> : <>
    {!modifier && <label>范围<Segmented value={draft.extent ?? "FINITE"} options={[{ value: "FINITE", label: "长度" }, { value: "TWO_SIDED", label: "双侧" }, { value: "SYMMETRIC", label: "对称" }, ...(["PAD","LINEAR_EXTRUDE"].includes(feature.type)&&["ADD","REMOVE","NEW_BODY"].includes(draft.operation??"")?[{value:"TO_PLANE",label:"到平面"}]:[]), ...(draft.operation === "REMOVE" ? [{ value: "THROUGH_ALL", label: "贯穿" }] : [])]} onChange={extent => {edit({ extent: extent as Feature["extent"],terminationPlane:undefined,terminationPlaneId:undefined });setTerminationResolution(undefined);setTerminationLoadError(undefined);setTerminationVisual(undefined);if(extent==="TO_PLANE")setRole("plane");else setRole(profileRole);}}/></label>}
    {draft.extent !== "THROUGH_ALL" && !terminating && <label>{feature.type === "FILLET" ? "半径" : feature.type === "CHAMFER" ? "距离" : feature.type === "SHELL" ? "厚度" : "长度"}（{unit}）<Input value={length} disabled={committing} onChange={e => setLength(e.target.value)}/></label>}
    {draft.extent === "TWO_SIDED" && <label>反向长度（{unit}）<Input value={second} disabled={committing} onChange={e => setSecond(e.target.value)}/></label>}
   </>)}
   {!loft && <label>反向<Switch checked={draft.reversed} disabled={committing} onChange={reversed => edit({ reversed })}/></label>}
   {feature.id && <label>抑制<Switch checked={draft.suppressed} disabled={committing} onChange={suppressed => edit({ suppressed })}/></label>}
   <small role="status">{!validRevision ? "模型已改变，请关闭并重新打开此命令" : binding || loadingInput ? "正在读取输入…" : preview.pending ? "正在预览…" : "在视图区点击添加选择，再次点击取消；参数修改后自动预览。"}</small>
   {inputError && <Alert type="error" title="无法取得上游输入" description={engineeringProblem(inputError)}/>}
   {(error ?? parameterError ?? preview.error) && <Alert type="error" title="特征求值失败" description={engineeringProblem(error ?? parameterError ?? preview.error)}/>}
   {preview.error && <Button onClick={preview.retry}>重新预览</Button>}

  </fieldset>
 </CommandDialog>;
}
