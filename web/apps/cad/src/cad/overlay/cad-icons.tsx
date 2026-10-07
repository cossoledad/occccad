import { createContext, createElement, useContext, useState } from "react";
import {builtinCatalog,type IconDefinition} from "../command/workbench-catalog";
export type CadIconName = string;
export const CadIconContext=createContext<Record<string,IconDefinition>>(builtinCatalog.icons);
const tags=new Set(["path","circle","rect","line","polyline","polygon","ellipse"]);
const attributes=new Set(["d","cx","cy","r","x","y","width","height","rx","ry","x1","x2","y1","y2","points","className"]);
export function CadIcon({name,className}:{name:string;className?:string}) {
 const icons=useContext(CadIconContext),icon=icons[name];
 const [failedResource,setFailedResource]=useState<string>();
 const classes=`${icon?.className??"cad-command-icon family-view"} ${className??""}`;
 if(icon?.resource?.startsWith("/assets/")&&failedResource!==icon.resource)return <img className={classes} src={icon.resource} alt="" onError={event=>{setFailedResource(icon.resource);}}/>;
 return <svg className={classes} viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.55" strokeLinecap="round" strokeLinejoin="round">
  {icon?.elements?.length?icon.elements.filter(element=>tags.has(element.tag)).map((element,index)=>createElement(element.tag,{...Object.fromEntries(Object.entries(element.attributes).filter(([key])=>attributes.has(key))),key:index})):<rect x="4" y="4" width="12" height="12" rx="2"/>}
 </svg>;
}
