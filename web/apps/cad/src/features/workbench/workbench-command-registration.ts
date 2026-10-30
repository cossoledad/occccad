import {defaultModifierFeature,modifierTypes} from "./solid-feature-model";
import {globalOptionsCommand} from "../preferences/global-options-command";
import {documentRegistry} from "../../cad/document/document-registry";
import {structureCommands,structureInvocation,resolveStructureNode,type StructureActions} from "./commands/structure-commands";
import {useLayoutEffect,useRef} from "react";
import {api} from "../../api/client";
import {defaultSketchToolMode} from "../../cad/sketch/sketch-inline-parameter-input";
import {CommandRegistry,type CadCommand,type CadCommandInvocation,type CommandOperation} from "../../cad/command/command-registry";
import {projectToolbars,validateCatalog,type WorkbenchCatalog,type ContextFacts} from "../../cad/command/workbench-catalog";
import {useWorkbenchStore,type WorkbenchToolID} from "../../state/workbench-store";
import type {DocumentView,Feature} from "../../types";
import type {ShareResource} from "../../components/share-dialog";
import type {CadViewportHandle} from "../../viewport/cad-viewport";
import {deletableTreeNodesForSelections,isSolidFeature} from "./workbench-tree-model";
import type {SpecificationTreeNode} from "./specification-tree";
import {selectionSummaryCommand} from "./commands/selection-summary";
import {treeNodesForSelections} from "./tree-projection-index";

export type WorkbenchCommandBindings={
 openReplaceInstance?:(id?:string)=>void;
 setDesignAssociationOpen?:(open:boolean)=>void;setDeveloperOpen?:(open:boolean)=>void;setAssociationRepairOpen?:(open:boolean)=>void;associationRepairOpen?:boolean;
 openProperties?:()=>void;openOptions?:()=>void;
	prepareAssemblyMove?:()=>Promise<void>;
 setMotionOpen?:(open:boolean)=>void;motionOpen?:boolean;motionCommandEnabled?:(action:string)=>boolean;onMotionCommand?:(action:string,operation?:CommandOperation)=>void;
 treeActions?:StructureActions;closePanels?:(nextCommand?:string)=>void;toolContinuation?:boolean;formActive?:boolean;
 store:ReturnType<typeof useWorkbenchStore.getState>;editingView?:DocumentView;view?:DocumentView;
 canEdit:boolean;canEditRoot:boolean;command:{isPending:boolean};selectedNamingIssue:unknown;conflictOpen:boolean;moveReceiptPending:boolean;workingBodyID?:string;
 treeNodes:SpecificationTreeNode[];viewport:{current:CadViewportHandle|null};
 startSketch:()=>void;finishSketch:()=>void;normalToSelection:(signal?:AbortSignal)=>void|Promise<void>;
 openSolidFeature:(generator:"LINEAR_EXTRUDE"|"REVOLVE",operation?:"NEW_BODY"|"ADD"|"REMOVE")=>void;
 deleteTreeNodes:(nodes:SpecificationTreeNode[])=>void;executeHistory:(direction:"undo"|"redo")=>void;
 setDesignInputsOpen?:(open:boolean)=>void;setConflictOpen:(open:boolean)=>void;setParameterManagerOpen:(open:boolean)=>void;setPublicationManagerOpen:(open:boolean)=>void;
 setMechanismAnalysisOpen?:(open:boolean)=>void;
 setInsertOpen:(open:boolean)=>void;setPatternOpen:(open:boolean)=>void;setReleaseOpen:(open:boolean)=>void;setVersionOpen:(open:boolean)=>void;
 setShareResource:(resource:ShareResource)=>void;
 setSolidEditor:(value:{feature:Feature;digest?:string}|undefined)=>void;
 setBooleanDialog:(value:{feature?:Feature;digest?:string}|undefined)=>void;
 setDatumEditor:(value:"plane"|"axis"|undefined)=>void;setEditingDatumId:(value:string|undefined)=>void;
 showMessage:(text:string)=>void;
};
const sketchToolCommands:WorkbenchToolID[]=["sketch.intersection","sketch.edit.split_intersections","sketch.edit.delete","sketch.edit.copy","sketch.edit.move","sketch.edit.mirror","sketch.edit.split","sketch.edit.trim","sketch.edit.fillet","sketch.edit.chamfer","sketch.edit.extend","sketch.edit.complement","sketch.edit.close","sketch.edit.offset","sketch.edit.spline_insert","sketch.edit.spline_delete","sketch.edit.spline_close","sketch.edit.spline_control","sketch.edit.construction","sketch.project","sketch.ellipse","sketch.elliptical_arc","sketch.rectangle","sketch.rectangle.center","sketch.rectangle.oriented","sketch.circle.three_point","sketch.arc.three_point","sketch.polygon","sketch.polygon.circumscribed","sketch.point","sketch.line","sketch.circle","sketch.arc","sketch.polyline","sketch.spline","sketch.spline.control",
  "sketch.constraint.coincident","sketch.constraint.parallel","sketch.constraint.collinear","sketch.constraint.fixed","sketch.constraint.horizontal","sketch.constraint.vertical",
  "sketch.constraint.perpendicular","sketch.constraint.tangent","sketch.constraint.equal","sketch.dimension.linear",
  "sketch.constraint.horizontal_distance","sketch.constraint.vertical_distance","sketch.constraint.radius","sketch.constraint.major_radius","sketch.constraint.minor_radius","sketch.constraint.angle","sketch.constraint.concentric","sketch.constraint.point_on_object","sketch.constraint.midpoint","sketch.constraint.symmetry"];

export function workbenchCommands(bindings:WorkbenchCommandBindings):CadCommand[]{
 const {store,editingView,view,canEdit,canEditRoot,command,selectedNamingIssue,conflictOpen,moveReceiptPending,workingBodyID,treeNodes,viewport,startSketch,finishSketch,normalToSelection,openSolidFeature,deleteTreeNodes,executeHistory,setConflictOpen,setParameterManagerOpen,setPublicationManagerOpen,setInsertOpen,setPatternOpen,setReleaseOpen,setVersionOpen,setShareResource,setSolidEditor,setBooleanDialog,setDatumEditor,setEditingDatumId,showMessage}=bindings;
 const deletionNodes=deletableTreeNodesForSelections(treeNodes,store.selections);
 const deletionTargets=(invocation?:CadCommandInvocation)=>invocation?.payload?structureInvocation(invocation).nodes??deletionNodes:deletionNodes;
 const resultDeletion=(invocation?:CadCommandInvocation)=>{const nodes=deletionTargets(invocation);return nodes.length>0&&nodes.every(node=>node.kind==="MOTION_RESULT"&&node.capabilities?.includes("DELETE"))};
 return [
  {id:"edit.properties",execute:()=>bindings.openProperties?.(),isEnabled:()=>Boolean(view&&bindings.openProperties&&!command.isPending&&!store.activeSketchID)},
  globalOptionsCommand(bindings.openOptions),

      ({id:"edit.delete",execute:invocation=>deleteTreeNodes((invocation?.payload?structureInvocation(invocation).nodes??deletionNodes:deletionNodes).map(node=>resolveStructureNode(treeNodes,node))),isEnabled:invocation=>Boolean((resultDeletion(invocation)||canEdit||bindings.motionOpen&&canEditRoot&&(invocation?.payload?structureInvocation(invocation).nodes??deletionNodes:deletionNodes).every(node=>["MECHANISM","MECHANISM_JOINT","MOTION_DRIVER","MOTION_STUDY","INTERFERENCE_ANALYSIS","MOTION_RESULT"].includes(node.kind??"")))&&!command.isPending&&(invocation?.payload?structureInvocation(invocation).nodes?.some(node=>node.capabilities?.includes("DELETE")):store.activeToolID==="select"&&deletionNodes.some(node=>node.capabilities?.includes("DELETE"))))}),
      ({ id: "tool.select", continues:bindings.motionOpen?["dmu.run","dmu.study","tree.edit","assembly.move"]:undefined,execute: () => store.setActiveTool("select", "once"),
        isActive: () => store.activeToolID === "select" }),
      ({ id: "assembly.move", continues:bindings.motionOpen?["dmu.run","dmu.study","tree.edit","assembly.move","tool.select"]:undefined,execute: async invocation => {const preparation=bindings.prepareAssemblyMove?.();if(preparation)await preparation;if(invocation?.operation&&!invocation.operation.current)return;store.setActiveTool("assembly.move", "continuous")},
        isEnabled: () => Boolean(canEditRoot&&(!bindings.motionOpen||bindings.motionCommandEnabled?.("move"))), isActive: () => store.activeToolID === "assembly.move" }),
      ...(["exit","ground","revolute","prismatic","cylindrical","planar","rigid","spherical","mechanism","study","apply","run"] as const).map(action=>({id:"dmu."+action,continues:action==="apply"?["dmu.run","dmu.apply"]:undefined,execute:(invocation:CadCommandInvocation|undefined)=>{store.setActiveTool("select","once");return bindings.onMotionCommand?.(action,invocation?.operation)},isEnabled:()=>Boolean(bindings.motionOpen&&view?.product&&!command.isPending&&bindings.onMotionCommand&&editingView?.document.id===view.document.id&&!store.activeSketchID&&(bindings.motionCommandEnabled?.(action)??(action==="apply"?["OWNER","EDITOR"].includes(view.document.permission??""):canEditRoot)))})),
      ({id:"dmu.analyze-mechanism",execute:()=>bindings.setMechanismAnalysisOpen?.(true),isEnabled:()=>Boolean(bindings.setMechanismAnalysisOpen&&view?.product&&editingView?.document.id===view.document.id&&!store.activeSketchID&&!command.isPending)}),
      ({id:"assembly.motion-study",execute:()=>{store.setActiveTool("select","once");bindings.setMotionOpen?.(true);},isEnabled:()=>Boolean(view?.product&&editingView?.document.id===view.document.id&&!store.activeSketchID&&!command.isPending&&bindings.setMotionOpen),isActive:()=>Boolean(bindings.motionOpen)}),
      ({id:"assembly.association-repair",execute:()=>bindings.setAssociationRepairOpen?.(true),isEnabled:()=>Boolean(bindings.setAssociationRepairOpen&&view?.product&&editingView?.document.id===view.document.id&&!store.activeSketchID&&!bindings.motionOpen),isActive:()=>bindings.associationRepairOpen??false}),
      ({id:"assembly.analyze",execute:()=>setConflictOpen(true),
        isEnabled:()=>Boolean(editingView?.product),isActive:()=>conflictOpen}),
      ({id:"assembly.move-receipt",execute:()=>viewport.current?.retryAssemblyMoveCommit(),
        isEnabled:()=>Boolean(moveReceiptPending)}),
      ({ id: "sketch.start", execute: startSketch,
		isEnabled: () => Boolean(canEdit && !selectedNamingIssue && (["plane", "sketch", "face"].includes(store.selection?.kind ?? ""))) }),
      ({ id: "view.normal", execute: invocation=>normalToSelection(invocation?.operation?.signal),
        isEnabled: () => Boolean(store.sketchPlane || store.selection && ["plane","face"].includes(store.selection.kind)) }),
      ({ id: "sketch.finish", execute: finishSketch,
         isEnabled: () => Boolean(canEdit && !command.isPending) }),
      ...sketchToolCommands.map((toolID)=>({id:toolID,execute:(invocation:CadCommandInvocation|undefined)=>store.setActiveTool(toolID,defaultSketchToolMode(toolID,invocation?.continuous)),
        isEnabled:()=>Boolean(canEdit&&store.sketchPlane&&(toolID!=="sketch.project"||!selectedNamingIssue)),isActive:()=>store.activeToolID===toolID})),
      ...(["LINEAR","CIRCULAR","MIRROR"] as const).map(kind=>({id:`part.pattern.${kind.toLowerCase()}`,execute:()=>{
        setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");
        setSolidEditor({feature:{id:"",type:store.activeSketchID?"SKETCH":"SOLID_PATTERN",profile:store.activeSketchID,bodyId:workingBodyID,operation:"ADD",pattern:{id:"",kind,mirrorPlaneId:kind==="MIRROR"?"datum-yz":undefined,axisEntityId:kind==="MIRROR"||store.activeSketchID?undefined:`AXIS_SYSTEM:${editingView?.part?.axisSystems[0]?.id??"axis-system-default"}:${kind==="CIRCULAR"?"Z":"X"}`,distribution:kind==="CIRCULAR"?"FULL_CIRCLE":"FIXED_STEP",count:kind==="MIRROR"?2:6,spacing:kind==="MIRROR"?0:10,angle:kind==="MIRROR"?0:90,phase:0,origin:[0,0,0],direction:kind==="CIRCULAR"?[0,0,1]:[1,0,0],sourceKind:store.activeSketchID?"SKETCH_FRAME":"AUTO",source:{bodyId:workingBodyID??"",featureId:store.activeSketchID??""}}}});
      },isEnabled:()=>Boolean(canEdit&&!command.isPending&&(kind!=="MIRROR"||!store.activeSketchID))})),
      ({id:"part.hole",execute:()=>{setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:{id:"",type:"HOLE",bodyId:workingBodyID,operation:"REMOVE",extent:"BLIND",diameter:5,depth:10,reversed:true}});},isEnabled:()=>canEdit&&!command.isPending&&!store.activeSketchID&&!!editingView?.part?.features.some(isSolidFeature)}),
      ({id:"part.loft",execute:()=>{setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:{id:"",type:"LOFT",bodyId:workingBodyID,operation:"ADD",sections:[]}});},isEnabled:()=>canEdit&&!command.isPending&&!store.activeSketchID}),
      ...modifierTypes.map(type=>({id:`part.${type.toLowerCase()}`, execute:()=>{setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:defaultModifierFeature(type,store.selection?.bodyId??workingBodyID)});},isEnabled:()=>canEdit&&!command.isPending&&!store.activeSketchID})),
      ({ id: "part.boolean", execute: () => {setDatumEditor(undefined);setSolidEditor(undefined);store.setActiveTool("select","once");setBooleanDialog({});}, isEnabled: () => canEdit&&!command.isPending && !store.activeSketchID }),
      ({ id: "part.pad", execute: () => openSolidFeature("LINEAR_EXTRUDE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID) }),
      ({ id: "part.pocket", execute: () => openSolidFeature("LINEAR_EXTRUDE", "REMOVE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID && editingView?.part?.features.some((feature) => isSolidFeature(feature))) }),
      ({ id: "part.revolve", execute: () => openSolidFeature("REVOLVE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID) }),
      ({ id: "part.parameters", execute: () => setParameterManagerOpen(true),
        isEnabled: () => Boolean(editingView?.part) }),
      ({id:"part.design-associations",execute:()=>bindings.setDesignAssociationOpen?.(true),isEnabled:()=>Boolean(canEditRoot&&editingView?.part&&["OWNER","EDITOR"].includes(editingView.document.permission)&&bindings.setDesignAssociationOpen)}),
      ({id:"part.design-inputs",execute:()=>bindings.setDesignInputsOpen?.(true),isEnabled:()=>Boolean(canEdit&&editingView?.part&&editingView.document.id===view?.document.id&&!store.activeSketchID&&!command.isPending)}),
      ({ id: "part.publications", execute: () => setPublicationManagerOpen(true),
		 isEnabled: () => Boolean(editingView?.part || editingView?.product) }),
      ({ id: "product.publications", execute: () => setPublicationManagerOpen(true),
		isEnabled: () => Boolean(editingView?.product) }),
      ({ id: "part.datum-plane", execute: () => { setSolidEditor(undefined); setBooleanDialog(undefined); setEditingDatumId(undefined);setDatumEditor("plane"); },
        isEnabled: () => Boolean(canEdit && !store.activeSketchID && !command.isPending) }),
      ({ id: "part.datum-axis", execute: () => { setSolidEditor(undefined); setBooleanDialog(undefined); setEditingDatumId(undefined);setDatumEditor("axis"); },
        isEnabled: () => Boolean(canEdit && !store.activeSketchID && !command.isPending) }),
      ({id:"product.replace",execute:invocation=>bindings.openReplaceInstance?.((invocation?.payload as {instanceId?:string}|undefined)?.instanceId),isEnabled:()=>Boolean(bindings.openReplaceInstance&&canEditRoot&&view?.product&&editingView?.document.id===view.document.id&&!store.activeSketchID&&!bindings.motionOpen&&!command.isPending)}),
      ({ id: "product.insert", execute: () => setInsertOpen(true), isEnabled: () => Boolean(canEdit) }),
      ({ id: "product.pattern", execute: () => {store.setActiveTool("select","once");setPatternOpen(true);},
        isEnabled: () => Boolean(canEdit) }),
      ({ id: "product.release", execute: () => setReleaseOpen(true), isEnabled: () => Boolean(canEditRoot) }),
      ...(["fix", "fix_together", "contact", "coincident", "concentric", "angle", "parallel", "perpendicular", "distance"] as const).map((constraint) => ({
        id: `assembly.${constraint}`,
        execute: (invocation:CadCommandInvocation|undefined) => store.setActiveTool(`assembly.${constraint}`, invocation?.continuous ? "continuous" : "once"),
        isEnabled: () => Boolean(canEdit && (["fix", "fix_together"].includes(constraint) || !selectedNamingIssue)),
        isActive: () => store.activeToolID === `assembly.${constraint}`,
      })),
      ({ id: "history.version", execute: () => setVersionOpen(true), isEnabled: () => Boolean(canEdit) }),
      ({ id: "document.share", execute: () => editingView && setShareResource({ type: "documents", id: editingView.document.id, name: editingView.document.name }),
        isEnabled: () => editingView?.document.permission === "OWNER" }),
      ({ id: "edit.undo", execute: () => executeHistory("undo"),
        isEnabled: () => Boolean((canEdit||bindings.motionOpen&&canEditRoot) && editingView?.document.canUndo && !command.isPending) }),
      ({ id: "edit.redo", execute: () => executeHistory("redo"),
        isEnabled: () => Boolean((canEdit||bindings.motionOpen&&canEditRoot) && editingView?.document.canRedo && !command.isPending) }),
      ({ id: "view.fit", execute: () => viewport.current?.fit() }),
      ({ id: "view.top", execute: () => viewport.current?.setStandardView("TOP") }),
      ({ id: "view.front", execute: () => viewport.current?.setStandardView("FRONT") }),
      ({ id: "view.right", execute: () => viewport.current?.setStandardView("RIGHT") }),
      ({ id: "view.iso", execute: () => viewport.current?.setStandardView("ISO") }),
	  ({id:"debug.information",execute:()=>bindings.setDeveloperOpen?.(true),isEnabled:()=>true}),
  ({ id: "debug.part", execute: () => editingView && api.downloadDiagnosticBundle(editingView.document.id), isEnabled: () => Boolean(editingView?.part) }),
      ({ id: "debug.assembly", execute: () => editingView && api.downloadAssemblyDiagnostic(editingView.document.id), isEnabled: () => Boolean(editingView?.product) }),
 ...structureCommands(bindings.treeActions,treeNodes,showMessage,treeNodesForSelections(treeNodes,store.selections)),
 selectionSummaryCommand({getContext:()=>({documentId:editingView?.document.id??"",documentName:editingView?.document.name??view?.document.name,selectionCount:store.selections.length}),show:showMessage}),
 ];
}

/** Install once; callbacks read the current existing state rather than capturing it. */
export function useWorkbenchCommandRegistration(registry:CommandRegistry,bindings:WorkbenchCommandBindings,catalog:WorkbenchCatalog|undefined,facts:ContextFacts,contextIdentity:string){
 const bindingsRef=useRef(bindings);bindingsRef.current=bindings;
 const currentFacts=useRef(facts);currentFacts.current=facts;
 const definitions=workbenchCommands(bindings);
 const definitionsRef=useRef(new Map<string,CadCommand>());definitionsRef.current=new Map(definitions.map(command=>[command.id,command]));
 useLayoutEffect(()=>{
  const current=(id:string)=>definitionsRef.current.get(id)!;
  const remove=registry.registerMany(definitions.map(command=>({id:command.id,get continues(){return current(command.id).continues},
   execute:async invocation=>{
    const operation=invocation?.operation;
    if(operation&&registry.declaration(command.id)?.implementation!=="handler"){
     await operation.prepare(async()=>{await bindingsRef.current.viewport.current?.settleAssemblyInteraction();});
     if(!operation.current)return;
     const clear=(nextCommand?:string)=>{const binding=bindingsRef.current;if(binding.closePanels)binding.closePanels(nextCommand);else {binding.viewport.current?.clearCommandPreview();binding.setSolidEditor(undefined);binding.setBooleanDialog(undefined);binding.setDatumEditor(undefined);}};
     if(!invocation?.continuing){clear(command.id);operation.own(()=>clear());operation.own(()=>bindingsRef.current.store.setActiveTool("select","once"));}
    }
    return current(command.id).execute(invocation);
   },isEnabled:invocation=>current(command.id).isEnabled?.(invocation)??true,
   isVisible:()=>current(command.id).isVisible?.()??true,isActive:command.isActive?()=>current(command.id).isActive?.()??false:undefined})));
  return ()=>{registry.cancel();remove();};
 },[registry]);
 useLayoutEffect(()=>{registry.setContext(contextIdentity);},[registry,contextIdentity]);
 useLayoutEffect(()=>{if(bindings.formActive!==undefined)registry.completeForm(bindings.formActive);},[registry,bindings.formActive]);
 useLayoutEffect(()=>{if(!bindings.toolContinuation)registry.completeTool(bindings.store.activeToolID);},[registry,bindings.store.activeToolID,bindings.toolContinuation]);
 // Validate before the configured deck is published.
 useLayoutEffect(()=>{if(catalog){documentRegistry.validateCatalog(catalog);validateCatalog(catalog,id=>registry.has(id));registry.configure(catalog,()=>currentFacts.current);}},[registry,catalog]);
 useLayoutEffect(()=>{registry.notifyStateChanged();});
 return catalog?projectToolbars(catalog):[];
}
