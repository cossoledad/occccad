import builtinJSON from '../../../../../../services/internal/workbenchconfig/catalog.json';
import type { ToolbarCatalogEntry } from '../../types';

export type CadWorkbenchID=string;

export type ContextFacts = {
 isMock:boolean;moveReceiptPending:boolean;hostType: string; targetType: string; sketchActive: boolean; canEdit: boolean; rootCanEdit: boolean;
 busy: boolean; selectionKind: string; selectionCount: number; hasWorkingBody: boolean;
};
export type Condition = { fact?: keyof ContextFacts; equals?: string | boolean | number; all?: Condition[]; any?: Condition[]|null; not?: Condition };
export type IconDefinition = { className?:string; elements?: {tag: string; attributes: Record<string,string>}[]; resource?: string };
export type CommandDeclaration = {labels?:Record<string,string>;id:string;name:string;helpText:string;iconKey:string;repeatable:boolean;implementation:'handler'|'tool'|'form';visibleWhen:Condition;enabledWhen:Condition};
export type WorkbenchCatalog = {
 schemaVersion:1;
 documents:{id:string;name:string;helpText:string;iconKey:string;adapter:string}[];
 icons:Record<string,IconDefinition>;
 commands:CommandDeclaration[];
 groups:{variants?:Record<string,string>;id:string;name:string;styleKey:ToolbarCatalogEntry['styleKey'];position:ToolbarCatalogEntry['position'];orientation:ToolbarCatalogEntry['orientation'];commands:{commandId:string;groupKey:string;order:number}[]}[];
 tabs:{domain:string;id:string;name:string;modeLabel:string;workbench:string;section:string;order:number}[];
 contexts:{id:string;when:Condition;tabs:string[]}[];
 placements:{tabId:string;groupId:string;order:number}[];
};
export const builtinCatalog = builtinJSON as unknown as WorkbenchCatalog;

export function matches(condition:Condition, facts:ContextFacts):boolean {
 return (!condition.fact || facts[condition.fact] === condition.equals)
  && (!condition.all || condition.all.every(child=>matches(child,facts)))
  && (!condition.any || condition.any.some(child=>matches(child,facts)))
  && (!condition.not || !matches(condition.not,facts));
}
export function contextTabs(catalog:WorkbenchCatalog,facts:ContextFacts) {
 const contexts=catalog.contexts.filter(context=>matches(context.when,facts));
 if(contexts.length!==1)throw new Error(`Expected one workbench context, found ${contexts.length}`);
 const ids=new Set(contexts[0].tabs);
 return catalog.tabs.filter(tab=>ids.has(tab.id)).sort((a,b)=>a.order-b.order);
}
export function projectToolbars(catalog:WorkbenchCatalog):ToolbarCatalogEntry[] {
 const commands=new Map(catalog.commands.map(command=>[command.id,command]));
 const groups=new Map(catalog.groups.map(group=>[group.id,group]));
 const tabs=new Map(catalog.tabs.map(tab=>[tab.id,tab]));
 return catalog.placements.map(placement=>{
  const group=groups.get(placement.groupId)!,tab=tabs.get(placement.tabId)!;
  return {id:group.id,name:group.name,workbench:tab.workbench,section:tab.section,tabIds:[tab.id],position:group.position,styleKey:group.styleKey,orientation:group.orientation,sortOrder:placement.order,
   items:group.commands.map(item=>{const command=commands.get(item.commandId)!;return {commandId:command.id,name:command.name,helpText:command.helpText,iconKey:command.iconKey,repeatable:command.repeatable,variantLabel:group.variants?.[item.groupKey],groupKey:item.groupKey,sortOrder:item.order};}).sort((a,b)=>a.sortOrder-b.sortOrder)};
 });
}
export function validateCatalog(catalog:WorkbenchCatalog,hasImplementation?:(id:string)=>boolean):void {
 if(catalog.schemaVersion!==1)throw new Error('Unsupported workbench catalog');
 const unique=(kind:string,ids:string[])=>{if(ids.some((id,index)=>!id||ids.indexOf(id)!==index))throw new Error(`Duplicate or empty ${kind} ID`);return new Set(ids);};
 unique('document',catalog.documents.map(value=>value.id));
 const commands=unique('command',catalog.commands.map(value=>value.id));
 const groups=unique('group',catalog.groups.map(value=>value.id));
 const tabs=unique('tab',catalog.tabs.map(value=>value.id));
 unique('context',catalog.contexts.map(value=>value.id));
 for(const command of catalog.commands){
  if(!catalog.icons[command.iconKey])throw new Error(`Missing icon: ${command.iconKey}`);
  if(hasImplementation&&!hasImplementation(command.id))throw new Error(`Missing command implementation: ${command.id}`);
 }
 for(const group of catalog.groups){unique('group command',group.commands.map(item=>item.commandId));for(const item of group.commands)if(!commands.has(item.commandId))throw new Error(`Missing command: ${item.commandId}`);}
 for(const context of catalog.contexts)for(const id of context.tabs)if(!tabs.has(id))throw new Error(`Missing tab: ${id}`);
 unique('placement',catalog.placements.map(item=>`${item.tabId}/${item.groupId}`));
 for(const item of catalog.placements)if(!tabs.has(item.tabId)||!groups.has(item.groupId))throw new Error('Missing catalog placement reference');
}
