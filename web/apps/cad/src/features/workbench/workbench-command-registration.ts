import {documentRegistry} from "../../cad/document/document-registry";
import {structureCommands,structureInvocation,resolveStructureNode,type StructureActions} from "./commands/structure-commands";
import {useLayoutEffect,useRef} from "react";
import {api} from "../../api/client";
import {defaultSketchToolMode} from "../../cad/sketch/sketch-inline-parameter-input";
import {CommandRegistry,type CadCommand,type CadCommandInvocation} from "../../cad/command/command-registry";
import {projectToolbars,validateCatalog,type WorkbenchCatalog,type ContextFacts} from "../../cad/command/workbench-catalog";
import {useWorkbenchStore,type WorkbenchToolID} from "../../state/workbench-store";
import type {DocumentView,Feature} from "../../types";
import type {ShareResource} from "../../components/share-dialog";
import type {CadViewportHandle} from "../../viewport/cad-viewport";
import {deletableTreeNodesForSelections,isSolidFeature} from "./workbench-tree-model";
import type {SpecificationTreeNode} from "./specification-tree";
import {selectionSummaryCommand} from "./commands/selection-summary";

export type WorkbenchCommandBindings={
 treeActions?:StructureActions;closePanels?:()=>void;toolContinuation?:boolean;formActive?:boolean;
 store:ReturnType<typeof useWorkbenchStore.getState>;editingView?:DocumentView;view?:DocumentView;
 canEdit:boolean;canEditRoot:boolean;command:{isPending:boolean};selectedNamingIssue:unknown;conflictOpen:boolean;moveReceiptPending:boolean;workingBodyID?:string;
 treeNodes:SpecificationTreeNode[];viewport:{current:CadViewportHandle|null};
 startSketch:()=>void;finishSketch:()=>void;normalToSelection:(signal?:AbortSignal)=>void|Promise<void>;
 openSolidFeature:(generator:"LINEAR_EXTRUDE"|"REVOLVE",operation?:"NEW_BODY"|"ADD"|"REMOVE")=>void;
 deleteTreeNodes:(nodes:SpecificationTreeNode[])=>void;executeHistory:(direction:"undo"|"redo")=>void;
 setConflictOpen:(open:boolean)=>void;setParameterManagerOpen:(open:boolean)=>void;setPublicationManagerOpen:(open:boolean)=>void;
 setInsertOpen:(open:boolean)=>void;setPatternOpen:(open:boolean)=>void;setReleaseOpen:(open:boolean)=>void;setVersionOpen:(open:boolean)=>void;
 setShareResource:(resource:ShareResource)=>void;
 setSolidEditor:(value:{feature:Feature;digest?:string}|undefined)=>void;
 setBooleanDialog:(value:{feature?:Feature;digest?:string}|undefined)=>void;
 setDatumEditor:(value:"plane"|"axis"|undefined)=>void;setEditingDatumId:(value:string|undefined)=>void;
 showMessage:(text:string)=>void;
};
const sketchToolCommands:WorkbenchToolID[]=["sketch.edit.delete","sketch.edit.copy","sketch.edit.move","sketch.edit.mirror","sketch.edit.split","sketch.edit.trim","sketch.edit.fillet","sketch.edit.chamfer","sketch.edit.extend","sketch.edit.complement","sketch.edit.close","sketch.edit.offset","sketch.edit.spline_insert","sketch.edit.spline_delete","sketch.edit.spline_close","sketch.edit.spline_control","sketch.edit.construction","sketch.project","sketch.ellipse","sketch.elliptical_arc","sketch.rectangle","sketch.rectangle.center","sketch.rectangle.oriented","sketch.circle.three_point","sketch.arc.three_point","sketch.polygon","sketch.polygon.circumscribed","sketch.point","sketch.line","sketch.circle","sketch.arc","sketch.polyline","sketch.spline","sketch.spline.control",
  "sketch.constraint.coincident","sketch.constraint.parallel","sketch.constraint.collinear","sketch.constraint.fixed","sketch.constraint.horizontal","sketch.constraint.vertical",
  "sketch.constraint.perpendicular","sketch.constraint.tangent","sketch.constraint.equal","sketch.dimension.linear",
  "sketch.constraint.horizontal_distance","sketch.constraint.vertical_distance","sketch.constraint.radius","sketch.constraint.major_radius","sketch.constraint.minor_radius","sketch.constraint.angle","sketch.constraint.concentric","sketch.constraint.point_on_object","sketch.constraint.midpoint","sketch.constraint.symmetry"];

export function workbenchCommands(bindings:WorkbenchCommandBindings):CadCommand[]{
 const {store,editingView,view,canEdit,canEditRoot,command,selectedNamingIssue,conflictOpen,moveReceiptPending,workingBodyID,treeNodes,viewport,startSketch,finishSketch,normalToSelection,openSolidFeature,deleteTreeNodes,executeHistory,setConflictOpen,setParameterManagerOpen,setPublicationManagerOpen,setInsertOpen,setPatternOpen,setReleaseOpen,setVersionOpen,setShareResource,setSolidEditor,setBooleanDialog,setDatumEditor,setEditingDatumId,showMessage}=bindings;
 const deletionNodes=deletableTreeNodesForSelections(treeNodes,store.selections);
 return [

      ({id:"edit.delete",execute:invocation=>deleteTreeNodes((invocation?.payload?structureInvocation(invocation).nodes??deletionNodes:deletionNodes).map(node=>resolveStructureNode(treeNodes,node))),isEnabled:invocation=>Boolean(canEdit&&!command.isPending&&(invocation?.payload?structureInvocation(invocation).nodes?.some(node=>node.capabilities?.includes("DELETE")):store.activeToolID==="select"&&deletionNodes.some(node=>node.capabilities?.includes("DELETE"))))}),
      ({ id: "tool.select", execute: () => store.setActiveTool("select", "once"),
        isActive: () => store.activeToolID === "select" }),
      ({ id: "assembly.move", execute: () => store.setActiveTool("assembly.move", "continuous"),
        isEnabled: () => Boolean(canEditRoot), isActive: () => store.activeToolID === "assembly.move" }),
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
        setSolidEditor({feature:{id:"",type:store.activeSketchID?"SKETCH":"SOLID_PATTERN",profile:store.activeSketchID,bodyId:workingBodyID,operation:"ADD",pattern:{id:"",kind,mirrorPlaneId:kind==="MIRROR"?"datum-yz":undefined,axisEntityId:kind==="MIRROR"||store.activeSketchID?undefined:`AXIS_SYSTEM:${editingView?.part?.axisSystems[0]?.id??"axis-system-default"}:${kind==="CIRCULAR"?"Z":"X"}`,distribution:kind==="CIRCULAR"?"FULL_CIRCLE":"FIXED_STEP",count:kind==="MIRROR"?2:6,spacing:kind==="MIRROR"?0:10,angle:kind==="MIRROR"?0:90,phase:0,origin:[0,0,0],direction:kind==="CIRCULAR"?[0,0,1]:[1,0,0],sourceKind:store.activeSketchID?"SKETCH_FRAME":"GENERATOR_TOOL",source:{bodyId:workingBodyID??"",featureId:store.activeSketchID??""}}}});
      },isEnabled:()=>Boolean(canEdit&&!command.isPending&&(kind!=="MIRROR"||!store.activeSketchID))})),
      ({id:"part.loft",execute:()=>{setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:{id:"",type:"LOFT",bodyId:workingBodyID,operation:"NEW_BODY",sections:[]}});},isEnabled:()=>canEdit&&!command.isPending&&!store.activeSketchID}),
      ...(["FILLET","CHAMFER","DRAFT","SHELL"] as const).map(type=>({id:`part.${type.toLowerCase()}`, execute:()=>{setDatumEditor(undefined);setBooleanDialog(undefined);store.setActiveTool("select","once");setSolidEditor({feature:{id:"",type,bodyId:store.selection?.bodyId??workingBodyID,length:1,angle:5,neutralPlaneId:"datum-xy",selections:[]}});},isEnabled:()=>canEdit&&!command.isPending&&!store.activeSketchID})),
      ({ id: "part.boolean", execute: () => {setDatumEditor(undefined);setSolidEditor(undefined);store.setActiveTool("select","once");setBooleanDialog({});}, isEnabled: () => canEdit&&!command.isPending && !store.activeSketchID }),
      ({ id: "part.pad", execute: () => openSolidFeature("LINEAR_EXTRUDE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID) }),
      ({ id: "part.pocket", execute: () => openSolidFeature("LINEAR_EXTRUDE", "REMOVE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID && editingView?.part?.features.some((feature) => isSolidFeature(feature))) }),
      ({ id: "part.revolve", execute: () => openSolidFeature("REVOLVE"), isEnabled: () => Boolean(canEdit && !store.activeSketchID) }),
      ({ id: "part.parameters", execute: () => setParameterManagerOpen(true),
        isEnabled: () => Boolean(editingView?.part) }),
      ({ id: "part.publications", execute: () => setPublicationManagerOpen(true),
		 isEnabled: () => Boolean(editingView?.part || editingView?.product) }),
      ({ id: "product.publications", execute: () => setPublicationManagerOpen(true),
		isEnabled: () => Boolean(editingView?.product) }),
      ({ id: "part.datum-plane", execute: () => { setSolidEditor(undefined); setBooleanDialog(undefined); setEditingDatumId(undefined);setDatumEditor("plane"); },
        isEnabled: () => Boolean(canEdit && !store.activeSketchID && !command.isPending) }),
      ({ id: "part.datum-axis", execute: () => { setSolidEditor(undefined); setBooleanDialog(undefined); setEditingDatumId(undefined);setDatumEditor("axis"); },
        isEnabled: () => Boolean(canEdit && !store.activeSketchID && !command.isPending) }),
      ({ id: "product.insert", execute: () => setInsertOpen(true), isEnabled: () => Boolean(canEdit) }),
      ({ id: "product.pattern", execute: () => setPatternOpen(true),
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
        isEnabled: () => Boolean(canEdit && editingView?.document.canUndo && !command.isPending) }),
      ({ id: "edit.redo", execute: () => executeHistory("redo"),
        isEnabled: () => Boolean(canEdit && editingView?.document.canRedo && !command.isPending) }),
      ({ id: "view.fit", execute: () => viewport.current?.fit() }),
      ({ id: "view.top", execute: () => viewport.current?.setStandardView("TOP") }),
      ({ id: "view.front", execute: () => viewport.current?.setStandardView("FRONT") }),
      ({ id: "view.right", execute: () => viewport.current?.setStandardView("RIGHT") }),
      ({ id: "view.iso", execute: () => viewport.current?.setStandardView("ISO") }),
	  ({ id: "debug.part", execute: () => editingView && api.downloadDiagnosticBundle(editingView.document.id), isEnabled: () => Boolean(editingView?.part) }),
      ({ id: "debug.assembly", execute: () => editingView && api.downloadAssemblyDiagnostic(editingView.document.id), isEnabled: () => Boolean(editingView?.product) }),
 ...structureCommands(bindings.treeActions,treeNodes,showMessage),
 selectionSummaryCommand({getContext:()=>({documentId:editingView?.document.id??"",selectionCount:store.selections.length}),show:showMessage}),
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
  const remove=registry.registerMany(definitions.map(command=>({id:command.id,
   execute:invocation=>{
    const operation=invocation?.operation;
    if(operation&&registry.declaration(command.id)?.implementation!=="handler"){
     const clear=()=>{const binding=bindingsRef.current;if(binding.closePanels)binding.closePanels();else {binding.viewport.current?.clearCommandPreview();binding.setSolidEditor(undefined);binding.setBooleanDialog(undefined);binding.setDatumEditor(undefined);}};
     clear();operation.own(clear);operation.own(()=>bindingsRef.current.store.setActiveTool("select","once"));
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
