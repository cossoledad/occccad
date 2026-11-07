import {useEffect,useMemo,useRef,useState,type MouseEvent} from 'react';
import {ReactFlow,ReactFlowProvider,Controls,Handle,Position,applyNodeChanges,useReactFlow,useNodesInitialized,type Node,type Edge,type NodeProps,type NodeChange} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {CadIcon} from '../cad/overlay/cad-icons';

export type GraphItem={id:string;selectionId?:string;label:string;caption:string;status?:string;kind:string;icon?:string;reference?:boolean;width?:number;muted?:boolean;warning?:boolean;position:{x:number;y:number}};
export type GraphLink={id:string;source:string;target:string;color?:string;directed?:boolean;reverse?:boolean;muted?:boolean};
type GraphData=Record<string,unknown>&GraphItem;
type Props={label:string;nodes:GraphItem[];edges:GraphLink[];selectedId?:string;layoutKey:string;onSelect:(id:string)=>void;onOpen:(id:string)=>void;onContextMenu?:(event:MouseEvent,id:string)=>void};

function Item({data}:NodeProps<Node<GraphData>>){return <div className={`engineering-graph-node ${data.warning?'warning':''} ${data.muted?'muted':''}`} title={[data.caption,data.status,data.reference?'同一对象的重复引用':''].filter(Boolean).join(' · ')}>
 <Handle id="in-left" type="target" position={Position.Left}/><Handle id="out-left" type="source" position={Position.Left}/>
 <CadIcon name={data.icon??'reference'}/><strong title={data.label}>{data.label}</strong>
 {data.status&&<span className="engineering-graph-status" aria-label={data.status}/>}
 <Handle id="in-right" type="target" position={Position.Right}/><Handle id="out-right" type="source" position={Position.Right}/>
</div>;}
const nodeTypes={engineering:Item};
function Canvas(p:Props){
 // Measure the complete incoming topology before viewport culling. The
 // default hook checks visible nodes only and can leave new branches outside
 // the old viewport unmeasured, so fitView never discovers them.
 const [nodes,setNodes]=useState<Node<GraphData>[]>([]),flow=useReactFlow(),initialized=useNodesInitialized({includeHiddenNodes:true});
 const topology=useMemo(()=>JSON.stringify([p.nodes.map(n=>n.id),p.edges.map(e=>e.id),p.layoutKey]),[p.nodes,p.edges,p.layoutKey]);
 const [placed,setPlaced]=useState(''),fitted=useRef('');
 useEffect(()=>{setNodes(previous=>{const existing=new Map(previous.map(n=>[n.id,n]));return p.nodes.map(n=>({...existing.get(n.id),id:n.id,type:'engineering',data:n,style:{width:n.width},ariaLabel:[n.caption,n.label,n.status].filter(Boolean).join(' · '),position:placed===topology?existing.get(n.id)?.position??n.position:n.position,selected:(n.selectionId??n.id)===p.selectedId}));});setPlaced(topology);},[p.nodes,p.selectedId,topology]);
 useEffect(()=>{if(initialized&&placed===topology&&fitted.current!==topology){fitted.current=topology;void flow.fitView({padding:.18,maxZoom:1,duration:0});}},[initialized,topology,placed]);
 const edges:Edge[]=useMemo(()=>p.edges.map(e=>({id:e.id,source:e.source,target:e.target,sourceHandle:e.reverse?'out-left':'out-right',targetHandle:e.reverse?'in-right':'in-left',type:'straight',style:{stroke:e.color??'var(--color-text-secondary)',strokeWidth:1,strokeDasharray:e.muted?'4 3':undefined},markerEnd:e.directed?{type:'arrowclosed' as const,color:e.color??'#65778a'}:undefined})),[p.edges]);
 return <div className="engineering-graph" aria-label={p.label}>
  <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={(changes:NodeChange<Node<GraphData>>[])=>setNodes(n=>applyNodeChanges(changes,n))}
   onNodeClick={(_,n)=>p.onSelect(n.data.selectionId??n.id)} onNodeDoubleClick={(_,n)=>p.onOpen(n.data.selectionId??n.id)} onNodeContextMenu={(event,n)=>p.onContextMenu?.(event,n.data.selectionId??n.id)}
   nodesConnectable={false} edgesReconnectable={false} deleteKeyCode={null} zoomOnDoubleClick={false} minZoom={.12} maxZoom={2} onlyRenderVisibleElements={initialized}
   proOptions={{hideAttribution:false}} fitView ariaLabelConfig={{'controls.zoomIn.ariaLabel':'放大关系图','controls.zoomOut.ariaLabel':'缩小关系图','controls.fitView.ariaLabel':'适合窗口'}}>
   <Controls showInteractive={false}/>
  </ReactFlow>
 </div>;
}
/** Graph library ownership ends here. Domain projections own identities,
 * relationships, actions and snapshot validation; node dragging stays local. */
export function GraphView(p:Props){return <ReactFlowProvider><Canvas {...p}/></ReactFlowProvider>;}
