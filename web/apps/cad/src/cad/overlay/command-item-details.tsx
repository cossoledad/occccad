import type {ReactNode} from 'react';
export function CommandItemDetails({label,items,selected,onSelect,children}:{label:string;items:{id:string;name:string;detail?:string}[];selected?:string;onSelect:(id:string)=>void;children:ReactNode}){
 return <div className="command-item-details"><div className="command-item-list" role="listbox" aria-label={label}>{items.map(item=><button type="button" role="option" aria-selected={selected===item.id} key={item.id} onClick={()=>onSelect(item.id)}><span>{item.name}</span>{item.detail&&<small>{item.detail}</small>}</button>)}</div><div className="command-item-detail">{children}</div></div>;
}
