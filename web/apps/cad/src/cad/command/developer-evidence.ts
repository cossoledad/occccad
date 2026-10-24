import {useSyncExternalStore} from 'react';
type Evidence={scope:string;time:number;value:unknown};
let current:Evidence|undefined;const listeners=new Set<()=>void>();
// Transient developer evidence, unrelated to model, history or solver state.
export function recordDeveloperEvidence(scope:string,value:unknown){current={scope,time:Date.now(),value};for(const notify of listeners)notify();}
export function useDeveloperEvidence(){return useSyncExternalStore(listener=>{listeners.add(listener);return()=>listeners.delete(listener)},()=>current,()=>undefined);}
