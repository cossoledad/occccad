import {useEffect,useMemo,useState,type MouseEvent} from 'react';
import {ReactFlow,ReactFlowProvider,Controls,Handle,Position,applyNodeChanges,useReactFlow,useNodesInitialized,type Node,type Edge,type NodeProps,type NodeChange} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

export type GraphItem={id:string;label:string;caption:string;status?:string;kind:string;muted?:boolean;warning?:boolean;position:{x:number;y:number}};
export type GraphLink={id:string;source:string;target:string;color?:string;directed?:boolean;muted?:boolean};
type GraphData=Record<string,unknown>&GraphItem;
type Props={label:string;nodes:GraphItem[];edges:GraphLink[];selectedId?:string;layoutKey:string;onSelect:(id:string)=>void;onOpen:(id:string)=>void;onContextMenu?:(event:MouseEvent,id:string)=>void};

function Item({data}:NodeProps<Node<GraphData>>){return <div className={`engineering-graph-node ${data.warning?'warning':''} ${data.muted?'muted':''}`}>
 <Handle type="target" position={Position.Left}/><span className="engineering-graph-caption">{data.caption}</span><strong title={data.label}>{data.label}</strong>{data.status&&<span className="engineering-graph-status">{data.status}</span>}<Handle type="source" position={Position.Right}/>
</div>;}
const nodeTypes={engineering:Item};
function Canvas(p:Props){
 const [nodes,setNodes]=useState<Node<GraphData>[]>([]),flow=useReactFlow(),initialized=useNodesInitialized();
 const topology=useMemo(()=>JSON.stringify([p.nodes.map(n=>n.id),p.edges.map(e=>e.id),p.layoutKey]),[p.nodes,p.edges,p.layoutKey]);
 const [placed,setPlaced]=useState('');
 useEffect(()=>{setNodes(previous=>{const positions=new Map(previous.map(n=>[n.id,n.position]));return p.nodes.map(n=>({id:n.id,type:'engineering',data:n,position:placed===topology?positions.get(n.id)??n.position:n.position,selected:n.id===p.selectedId}));});setPlaced(topology);},[p.nodes,p.selectedId,topology]);
 useEffect(()=>{if(initialized)void flow.fitView({padding:.18,maxZoom:1,duration:0});},[initialized,topology]);
 const edges:Edge[]=useMemo(()=>p.edges.map(e=>({id:e.id,source:e.source,target:e.target,type:'smoothstep',style:{stroke:e.color??'var(--color-text-secondary)',strokeWidth:1.25,strokeDasharray:e.muted?'5 4':undefined},markerEnd:e.directed?{type:'arrowclosed' as const,color:e.color??'#65778a'}:undefined})),[p.edges]);
 return <div className="engineering-graph" aria-label={p.label}>
  <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={(changes:NodeChange<Node<GraphData>>[])=>setNodes(n=>applyNodeChanges(changes,n))}
   onNodeClick={(_,n)=>p.onSelect(n.id)} onNodeDoubleClick={(_,n)=>p.onOpen(n.id)} onNodeContextMenu={(event,n)=>p.onContextMenu?.(event,n.id)}
   nodesConnectable={false} edgesReconnectable={false} deleteKeyCode={null} zoomOnDoubleClick={false} minZoom={.12} maxZoom={2} onlyRenderVisibleElements
   proOptions={{hideAttribution:false}} fitView ariaLabelConfig={{'controls.zoomIn.ariaLabel':'放大关系图','controls.zoomOut.ariaLabel':'缩小关系图','controls.fitView.ariaLabel':'适合窗口'}}>
   <Controls showInteractive={false}/>
  </ReactFlow>
 </div>;
}
/** Graph library ownership ends here. Domain projections own identities,
 * relationships, actions and snapshot validation; node dragging stays local. */
export function GraphView(p:Props){return <ReactFlowProvider><Canvas {...p}/></ReactFlowProvider>;}
