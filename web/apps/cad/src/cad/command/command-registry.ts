import {matches,validateCatalog,type WorkbenchCatalog,type ContextFacts,type CommandDeclaration} from './workbench-catalog';
export type CadCommandState = { enabled: boolean; visible: boolean; active: boolean };
export type CadCommandInvocation = { continuous?: boolean; operation?: CommandOperation; payload?:unknown };

export class CommandCleanupError extends AggregateError {
 constructor(errors:unknown[]){super(errors,"Command cleanup failed");}
}

/** One invocation owns cancellation and disposable resources, never model state. */
export class CommandOperation {
 private readonly controller=new AbortController();
 private readonly disposers:(()=>void)[]=[];
 get signal():AbortSignal{return this.controller.signal;}
 get current():boolean{return !this.signal.aborted;}
 own(dispose:()=>void):void {if(this.current)this.disposers.push(dispose);else dispose();}
 cancel():void {
  if(!this.current)return;
  this.controller.abort();
  const errors:unknown[]=[];
  for(const dispose of this.disposers.splice(0).reverse())try{dispose();}catch(error){errors.push(error);}
  if(errors.length)throw new CommandCleanupError(errors);
 }
}
export interface CadCommand {
 readonly id: string;
 execute(invocation?: CadCommandInvocation): void | Promise<void>;
 isEnabled?(invocation?:CadCommandInvocation): boolean;
 isVisible?(): boolean;
 isActive?(): boolean;
}
export class CommandRegistry {
 private readonly commands=new Map<string,CadCommand>();
 private readonly listeners=new Set<()=>void>();
 private declarations=new Map<string,CommandDeclaration>();
 private facts?:()=>ContextFacts;
 private operation?:CommandOperation;
 private operationID?:string;
 private contextID?:string;
 private readonly invocations=new Map<CommandOperation,string>();
 private revision=0;
 private readonly uiActions=new Map<string,()=>void>();
 constructor(){
  for(const id of ['ui.command-search','ui.shortcut-help'])this.commands.set(id,{id,execute:()=>{this.uiActions.get(id)?.();},isEnabled:()=>this.uiActions.has(id)});
 }
 bindUI(id:string,execute:()=>void):()=>void {
  if(!this.commands.has(id)||this.uiActions.has(id))throw new Error(`Invalid or duplicate UI action: ${id}`);
  this.uiActions.set(id,execute);this.notifyStateChanged();
  return ()=>{this.uiActions.delete(id);this.notifyStateChanged();};
 }
 register(command:CadCommand):()=>void {return this.registerMany([command]);}
 registerMany(commands:readonly CadCommand[]):()=>void {
  const ids=new Set<string>();
  for(const command of commands){if(typeof command.execute!=="function")throw new Error(`Missing command implementation: ${command.id}`);if(!command.id||ids.has(command.id)||this.commands.has(command.id))throw new Error(`Duplicate command implementation: ${command.id}`);ids.add(command.id);}
  for(const command of commands)this.commands.set(command.id,command);
  this.notifyStateChanged();
  return ()=>{
   for(const command of commands)if(this.commands.get(command.id)===command)this.commands.delete(command.id);
   this.cancelInvocations(operation=>ids.has(this.invocations.get(operation)!));this.notifyStateChanged();
  };
 }
 configure(catalog:WorkbenchCatalog,facts:()=>ContextFacts):void {
  validateCatalog(catalog,id=>this.has(id));
  this.declarations=new Map(catalog.commands.map(command=>[command.id,command]));this.facts=facts;this.notifyStateChanged();
 }
 setContext(identity:string):void {if(this.contextID===identity)return;this.contextID=identity;this.cancelInvocations(()=>true);this.notifyStateChanged();}
 private cancelInvocations(matches:(operation:CommandOperation)=>boolean):void {
  const errors:unknown[]=[];
  for(const operation of this.invocations.keys())if(matches(operation)){
   this.invocations.delete(operation);
   if(operation===this.operation){this.operation=undefined;this.operationID=undefined;}
   try{operation.cancel();}catch(error){errors.push(error);}
  }
  if(errors.length)throw new CommandCleanupError(errors);
 }
 cancel():void {const previous=this.operation;this.operation=undefined;this.operationID=undefined;if(previous){this.invocations.delete(previous);previous.cancel();}}
 completeForm(active:boolean):void {if(!active&&this.operationID&&this.declarations.get(this.operationID)?.implementation==='form')this.cancel();}
 completeTool(activeToolID:string):void {
  if(this.operationID&&this.declarations.get(this.operationID)?.implementation==='tool'&&activeToolID==='select'&&this.operationID!=='tool.select')this.cancel();
 }
 declaration(id:string):CommandDeclaration|undefined{return this.declarations.get(id);}
 has(id:string):boolean{return this.commands.has(id);}
 state(id:string,invocation?:CadCommandInvocation):CadCommandState {
  const command=this.commands.get(id);
  if(!command)return {enabled:false,visible:false,active:false};
  const declaration=this.declarations.get(id),facts=this.facts?.();
  return {enabled:(command.isEnabled?.(invocation)??true)&&(!declaration||!facts||matches(declaration.enabledWhen,facts)),
   visible:(command.isVisible?.()??true)&&(!declaration||!facts||matches(declaration.visibleWhen,facts)),active:command.isActive?.()??Boolean(this.operation?.current&&this.operationID===id)};
 }
 async execute(id:string,invocation?:CadCommandInvocation):Promise<boolean> {
  const command=this.commands.get(id);
  if(!command||!this.state(id,invocation).enabled)return false;
  const interactive=this.declarations.get(id)?.implementation!=='handler'&&this.declarations.has(id);
  if(interactive)this.cancel();
  const operation=new CommandOperation();this.invocations.set(operation,id);
  if(interactive){this.operation=operation;this.operationID=id;}
  try {
   await command.execute({...invocation,operation});
   const current=operation.current;
   if(!interactive)operation.cancel();
   if(current)this.notifyStateChanged();
   return current;
  } catch(error){
   const current=operation.current;
   if(operation===this.operation){this.operation=undefined;this.operationID=undefined;}
   try{operation.cancel();}catch(cleanup){throw new CommandCleanupError([error,cleanup]);}
   if(!current&&!(error instanceof CommandCleanupError))return false;
   throw error;
  } finally {if(!interactive||!operation.current)this.invocations.delete(operation);}
 }

 subscribe=(listener:()=>void):(()=>void)=>{this.listeners.add(listener);return ()=>this.listeners.delete(listener);};
 getSnapshot=():number=>this.revision;
 notifyStateChanged():void {this.revision++;for(const listener of this.listeners)listener();}
}
