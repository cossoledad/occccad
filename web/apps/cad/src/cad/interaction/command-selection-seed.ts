import type {SelectionItem} from '../../types';
import {featureSelectionHit,type FeatureSelectionSession} from './feature-selection';
import {selectionKey} from './selection-identity';

/** A creation invocation consumes the original selection once, through the
 * same role filter as subsequent picks. It never owns persistent references. */
export class CommandSelectionSeed {
 private values:SelectionItem[]=[];
 private consumed=new Set<string>();
 private singleSlots=new Set<string>();
 private signal?:AbortSignal;
 begin(values:readonly SelectionItem[],signal:AbortSignal):void {
  this.values=values.map(value=>structuredClone(value));
  this.consumed.clear();this.singleSlots.clear();this.signal=signal;
 }
 take(session:FeatureSelectionSession):SelectionItem|undefined {
  if(!this.signal||this.signal.aborted||session.seedEnabled===false)return;
  const slot=session.seedSlot??session.role;
  if(this.singleSlots.has(slot))return;
  const selected=new Set(session.selections.map(selectionKey));
  for(const value of this.values){
   const key=selectionKey(value);if(this.consumed.has(key))continue;
   const hit=featureSelectionHit(value,session);if(!hit)continue;
   this.consumed.add(key);
   if(selected.has(selectionKey(hit)))continue;
   if(!session.seedMultiple||session.role==='profile-region'&&hit.kind==='sketch')this.singleSlots.add(slot);
   return hit;
  }
 }
}
