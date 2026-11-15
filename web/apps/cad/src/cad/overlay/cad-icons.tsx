import { createContext, createElement, useContext, useState } from "react";
import {builtinCatalog,type IconDefinition} from "../command/workbench-catalog";
import {iconTags as tags,iconAttributes as attributes} from "./icon-elements";
export type CadIconName = string;
export const CadIconContext=createContext<Record<string,IconDefinition>>(builtinCatalog.icons);
export function CadIcon({name,className}:{name:string;className?:string}) {
 const icons=useContext(CadIconContext),icon=icons[name];
 const [failedResource,setFailedResource]=useState<string>();
 const classes=`${icon?.className??"cad-command-icon family-view"} ${className??""}`;
 if(icon?.resource?.startsWith("/assets/")&&failedResource!==icon.resource)return <img data-icon={name} className={classes} src={icon.resource} alt="" onError={event=>{setFailedResource(icon.resource);}}/>;
 return <svg data-icon={name} className={classes} viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
  {icon?.elements?.length?icon.elements.filter(element=>tags.has(element.tag)).map((element,index)=>createElement(element.tag,{...Object.fromEntries(Object.entries(element.attributes).filter(([key])=>attributes.has(key))),key:index})):<rect x="4" y="4" width="12" height="12" rx="2"/>}
 </svg>;
}
