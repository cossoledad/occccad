import {Button,Checkbox,Dropdown,Empty,Select,Space,Typography,type MenuProps} from 'antd';
import {useEffect,useMemo,useState} from 'react';
import type {DocumentView,SelectionItem} from '../../types';
import {assemblyDependencies,dependencySlice,dependencyState,type DependencyCategory,type DependencyNode} from '../../cad/assembly/assembly-dependencies';
import {owningInstanceSelection} from '../../cad/assembly/instance-selection';
import {CommandDialog} from '../../cad/overlay/floating-panel';
import {GraphView,type GraphItem,type GraphLink} from '../../components/graph-view';

type Props={view:DocumentView;ownerOccurrence?:string;selection?:SelectionItem|null;canEdit:boolean;onClose:()=>void;onSelect?:(node:DependencyNode,revision:string)=>void;onLocate:(node:DependencyNode,revision:string)=>void;onEdit:(id:string,revision:string)=>void;onToggle:(id:string,suppressed:boolean,revision:string)=>void};
const categories:Array<{value:DependencyCategory;label:string}>=[{value:'CONSTRAINT',label:'约束'},{value:'ASSOCIATIVITY',label:'上下文关联'},{value:'RELATION',label:'公式与参数'}];
const colours={CONSTRAINT:'#65778a',ASSOCIATIVITY:'#25846a',RELATION:'#8c69aa'};
export function AssemblyDependencyPanel(p:Props){
 const context=JSON.stringify([p.view.document.id,p.ownerOccurrence??'']);
 const graph=useMemo(()=>assemblyDependencies(p.view,p.ownerOccurrence),[p.view,p.ownerOccurrence]);
 const initial=p.selection?owningInstanceSelection(p.view,p.selection,p.ownerOccurrence):null;
 const initialRoot=graph.nodes.find(n=>n.instanceId===initial?.instanceId)?.id??graph.rootId;
 const [root,setRoot]=useState(initialRoot),[selected,setSelected]=useState<string>(),[expanded,setExpanded]=useState<Set<string>>(()=>new Set([initialRoot]));
 const [filters,setFilters]=useState<DependencyCategory[]>(['CONSTRAINT']),[includeSuppressed,setIncludeSuppressed]=useState(true),[layout,setLayout]=useState(0),[menu,setMenu]=useState<{id:string;x:number;y:number}>();
 useEffect(()=>{setRoot(initialRoot);setExpanded(new Set([initialRoot]));setSelected(undefined);setMenu(undefined);},[context]);
 const index=useMemo(()=>new Map(graph.nodes.map(n=>[n.id,n])),[graph]);
 const slice=useMemo(()=>dependencySlice(graph,root,expanded,{categories:filters,includeSuppressed}),[graph,root,expanded,filters,includeSuppressed]);
 useEffect(()=>{const selection=p.selection;if(!selection)return;
  const instance=owningInstanceSelection(p.view,selection,p.ownerOccurrence);
  const node=instance?graph.nodes.find(n=>n.instanceId===instance.instanceId)
   :selection.documentId===p.view.document.id&&(selection.occurrencePath??'')===(p.ownerOccurrence??'')?graph.nodes.find(n=>selection.kind==='assembly-constraint'?n.constraintId===selection.constraintId:selection.kind==='parameter'?n.parameterId===selection.id:false):undefined;
  if(node)setSelected(node.id);
 },[p.selection,graph,p.ownerOccurrence]);
 const current=selected?index.get(selected):undefined;
 const nodes:GraphItem[]=useMemo(()=>{const rows=new Map<number,number>();return slice.nodes.map(n=>{const rank=slice.ranks.get(n.id)??0,row=rows.get(rank)??0;rows.set(rank,row+1);return {...n,status:[dependencyState(n),n.hidden?'已隐藏':''].filter(Boolean).join(' · '),muted:n.suppressed,warning:n.missing||['BROKEN','BROKEN_PUBLICATION','UNAVAILABLE'].includes(n.state??''),position:{x:rank*264,y:row*100}};});},[slice]);
 const links:GraphLink[]=useMemo(()=>slice.edges.map(e=>({...e,color:e.category?colours[e.category]:'#aeb9c3',muted:index.get(e.source)?.suppressed||index.get(e.target)?.suppressed})),[slice.edges,index]);
 const rootOptions=graph.nodes.filter(n=>['assembly','component'].includes(n.kind)&&!n.missing).map(n=>({value:n.id,label:n.label}));
 const setAsRoot=(id:string)=>{if(!index.has(id))return;setRoot(id);setExpanded(new Set([id]));setSelected(id);setMenu(undefined);const node=index.get(id);if(node&&!node.missing)p.onSelect?.(node,graph.revisionId);};
 const expand=(id:string)=>{setExpanded(previous=>new Set([...previous,id]));setMenu(undefined);};
 const all=()=>{setExpanded(new Set(graph.nodes.map(n=>n.id)));setMenu(undefined);};
 const actions=(n:DependencyNode):MenuProps['items']=>[
  {key:'expand',label:'展开节点',disabled:expanded.has(n.id),onClick:()=>expand(n.id)},
  {key:'all',label:'展开全部',onClick:all},
  {key:'root',label:'设为根',disabled:n.missing||!['assembly','component'].includes(n.kind),onClick:()=>setAsRoot(n.id)},
  {type:'divider'},
  {key:'locate',label:'定位',disabled:n.missing||!['component','constraint','parameter'].includes(n.kind),onClick:()=>p.onLocate(n,graph.revisionId)},
  ...(n.kind==='constraint'?[{key:'edit',label:'编辑',disabled:!p.canEdit||n.missing,onClick:()=>p.onEdit(n.constraintId!,graph.revisionId)},
   {key:'activation',label:n.suppressed?'启用':'停用',disabled:!p.canEdit||n.missing,onClick:()=>p.onToggle(n.constraintId!,!n.suppressed,graph.revisionId)}]:[]),
 ];
 return <CommandDialog id="assembly-dependencies" title={`依赖分析 · ${p.view.document.name}`} open size="L" footer={false} onClose={p.onClose} onConfirm={p.onClose} bodyClassName="assembly-dependency-body" scrollBody={false}>
  <Space wrap className="assembly-dependency-toolbar">
   <Select aria-label="分析根组件" showSearch optionFilterProp="label" value={slice.rootId} options={rootOptions} onChange={setAsRoot} style={{width:220}}/>
   <Button size="small" onClick={()=>setExpanded(previous=>new Set([...previous,...slice.nodes.map(n=>n.id)]))}>展开一层</Button><Button size="small" onClick={all}>展开全部</Button>
   <Button size="small" onClick={()=>{setExpanded(new Set([slice.rootId]));setSelected(undefined);}}>收起</Button><Button size="small" onClick={()=>setLayout(n=>n+1)}>整理布局</Button>
  </Space>
  <div className="assembly-dependency-filters">
   <Checkbox.Group value={filters} onChange={values=>{setFilters(values as DependencyCategory[]);setMenu(undefined);}} options={categories}/>
   <Checkbox checked={includeSuppressed} onChange={e=>setIncludeSuppressed(e.target.checked)}>包含停用</Checkbox>
  </div>
  {slice.nodes.length?<GraphView label="装配依赖关系图" nodes={nodes} edges={links} selectedId={selected} layoutKey={`${context}:${slice.rootId}:${layout}`}
   onSelect={id=>{setSelected(id);const n=index.get(id);if(n&&['component','constraint','parameter'].includes(n.kind)&&!n.missing)p.onSelect?.(n,graph.revisionId);}}
   onOpen={()=>all()} onContextMenu={(e,id)=>{e.preventDefault();setSelected(id);setMenu({id,x:e.clientX,y:e.clientY});}}/>:<Empty description="没有匹配的依赖关系"/>}
  {menu&&index.has(menu.id)&&<Dropdown open trigger={[]} onOpenChange={open=>{if(!open)setMenu(undefined);}} menu={{items:actions(index.get(menu.id)!),onClick:()=>setMenu(undefined)}}><span style={{position:'fixed',left:menu.x,top:menu.y,width:1,height:1}}/></Dropdown>}
  <div className="assembly-dependency-summary">
   <span>{slice.nodes.filter(n=>n.kind==='component').length} 个组件 · {slice.nodes.filter(n=>['constraint','association','formula'].includes(n.kind)).length} 个关系</span>
   {current&&<><Typography.Text strong>{current.label}</Typography.Text><span>{dependencyState(current)}</span>{current.expression&&<Typography.Text code>{current.expression}</Typography.Text>}</>}
  </div>
 </CommandDialog>;
}
