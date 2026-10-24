import {Button,Input,Tooltip} from 'antd';
import {AimOutlined,CloseOutlined,DownOutlined,RightOutlined,SwapOutlined} from '@ant-design/icons';
import {useEffect,useRef,useState,type ReactNode} from 'react';
import type {SelectionItem} from '../../types';
import {CommandField,useCommandForm} from '../../cad/overlay/command-form';

export type FeaturePickItem={key:string;value:ReactNode;actionLabel?:string;searchText?:string;detail?:ReactNode;error?:boolean;state?:string;active?:boolean;disabled?:boolean;selections?:SelectionItem[];onLocate?:()=>void;onActivate?:()=>void;onClear?:()=>void;actions?:ReactNode;settings?:ReactNode};
type Props={label:string;value?:ReactNode;active:boolean;disabled?:boolean;onActivate?:()=>void;onCancel?:()=>void;onClear?:()=>void;onLocate?:()=>void;mode?:'single'|'multiple'|'ordered';selected?:boolean;selections?:SelectionItem[];detail?:ReactNode;error?:ReactNode;actions?:ReactNode;children?:ReactNode;items?:FeaturePickItem[]};

/** One reference field. Collections have bounded compact rows, retaining the
 * owning command's accepted identities, ordering and replacement session. */
export function FeaturePickField({label,value,active,disabled,onActivate,onCancel,onClear,onLocate,selected,mode='single',selections,detail,error,actions,children,items}:Props){
 const form=useCommandForm(),[expanded,setExpanded]=useState(true),[query,setQuery]=useState(''),[settings,setSettings]=useState<string>();const rows=useRef<HTMLDivElement>(null);
 const hasValue=selected??Boolean(onClear||selections?.length),cancel=()=>onCancel?onCancel():form.cancelPick?.();
 const locate=(callback:(()=>void)|undefined,picks?:SelectionItem[])=>callback||picks?.length&&form.locate?()=>{cancel();if(callback)callback();else if(picks)form.locate?.(picks)}:undefined;
 const busyItem=items?.find(item=>item.active),invalid=items?.filter(item=>item.error).length??0,invalidToken=items?.filter(item=>item.error).map(item=>item.key).join('|');
 useEffect(()=>{if(busyItem){setExpanded(true);setQuery('')}},[busyItem?.key]);
 useEffect(()=>{if(invalidToken)setExpanded(true)},[invalidToken]);
 useEffect(()=>{if(!items?.length)setQuery('')},[items?.length]);
 useEffect(()=>{if(expanded)(rows.current?.querySelector('[data-item-active=true]')??rows.current?.querySelector('[data-item-error=true]'))?.scrollIntoView({block:'nearest'})},[expanded,busyItem?.key,invalidToken]);
 const compactAction=(name:string,title:string,icon:ReactNode,action:()=>void,blocked?:boolean)=><Tooltip title={title}><Button size="small" type="text" aria-label={name} disabled={disabled||blocked} icon={icon} onClick={action}/></Tooltip>;
 if(items!==undefined){
  const visible=items.filter(item=>item.active||item.error||!query.trim()||(item.searchText??[typeof item.value==='string'?item.value:'',typeof item.detail==='string'?item.detail:''].join(' ')).toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  return <div role="group" aria-label={label} className={`feature-pick-field feature-pick-collection ${active||busyItem?'is-active':''}`} data-pick-active={active||!!busyItem||undefined}>
   <div className="feature-pick-collection-heading"><strong>{label}</strong><span className="feature-pick-count">{items.length} 项</span>{invalid>0&&<span className="command-field-error">{invalid} 项待修复</span>}
    <div className="feature-pick-actions">
     {onActivate&&<Button size="small" aria-label={`选择${label}`} aria-pressed={active} type={active?'primary':'default'} disabled={disabled} onClick={active?cancel:onActivate}>{active?'取消选择':items.length?'增减':'选择'}</Button>}
     {onClear&&compactAction(`移除${label}`,'清空',<CloseOutlined/>,()=>{cancel();onClear()})}
     {items.length>0&&compactAction(`${expanded?'收起':'展开'}${label}`,expanded?'收起':'展开',expanded?<DownOutlined/>:<RightOutlined/>,()=>setExpanded(v=>!v))}{actions}
    </div>
   </div>
   {expanded&&items.length>0&&<>
    {(items.length>5||query)&&<Input.Search aria-label={`筛选${label}`} placeholder="筛选名称" size="small" value={query} onKeyDown={event=>{if(event.key==='Enter'){event.stopPropagation();if(!event.nativeEvent.isComposing&&event.nativeEvent.keyCode!==229)event.preventDefault()}}} onChange={e=>setQuery(e.target.value)}/>}
    <div ref={rows} className="feature-pick-collection-list" role="list" aria-label={`${label}列表`}>
     {visible.map(item=>{const index=items.indexOf(item),actionLabel=item.actionLabel??`${label} ${index+1}`,locateItem=locate(item.onLocate,item.selections);return <div key={item.key} role="listitem" className={`feature-pick-item ${item.active?'is-active':''} ${item.error?'has-error':''}`} data-item-active={item.active||undefined} data-item-error={item.error||undefined} data-state={item.state}>
      {mode==='ordered'&&<span className="feature-pick-ordinal">{index+1}</span>}
      <div className="feature-pick-item-name"><span title={typeof item.value==='string'?item.value:undefined}>{item.value}</span>{item.detail&&<small title={typeof item.detail==='string'?item.detail:undefined}>{item.detail}</small>}</div>
      <div className="feature-pick-item-actions">
       {locateItem&&compactAction(`定位${actionLabel}`,'定位',<AimOutlined/>,locateItem,item.disabled)}
       {item.onActivate&&compactAction(`选择${actionLabel}`,item.active?'取消更换':'更换',<SwapOutlined/>,item.active?cancel:item.onActivate,item.disabled)}
       {item.onClear&&compactAction(`移除${actionLabel}`,'移除',<CloseOutlined/>,()=>{cancel();item.onClear?.()},item.disabled)}
       {item.actions}
       {item.settings&&compactAction(`设置${actionLabel}`,'设置',settings===item.key?<DownOutlined/>:<RightOutlined/>,()=>setSettings(s=>s===item.key?undefined:item.key),item.disabled)}
      </div>
     </div>})}
     {!visible.length&&<span className="feature-pick-empty">没有匹配的对象</span>}
    </div>
    {settings&&items.find(item=>item.key===settings)?.settings&&<div className="feature-pick-item-settings">{items.find(item=>item.key===settings)!.settings}</div>}
   </>}
   {detail&&<small className="feature-pick-detail">{detail}</small>}{error&&<div className="command-field-error" role="alert">{error}</div>}{children}
  </div>;
 }
 const locateSingle=locate(onLocate,selections);
 return <div className={`feature-pick-field ${active?'is-active':''}`} data-pick-active={active||undefined}>
  <CommandField label={label} width="selection" error={error}>
   <div className="feature-pick-content"><span className="feature-pick-value" title={typeof value==='string'?value:undefined}>{value||'未选择'}</span>
    {detail&&<small className="feature-pick-detail">{detail}</small>}
    <div className="feature-pick-actions">
     {locateSingle&&<Button size="small" disabled={disabled||!hasValue} aria-label={`定位${label}`} onClick={locateSingle}>定位</Button>}
     {onActivate&&<Button size="small" disabled={disabled} aria-label={`选择${label}`} aria-pressed={active} type={active?'primary':'default'} onClick={active?cancel:onActivate}>{active?'取消选择':hasValue?mode==='single'?'更换':'增减':'选择'}</Button>}
     {onClear&&<Button size="small" disabled={disabled} aria-label={`移除${label}`} onClick={()=>{cancel();onClear()}}>移除</Button>}{actions}
    </div>
   </div>{children}
  </CommandField>
 </div>;
}
