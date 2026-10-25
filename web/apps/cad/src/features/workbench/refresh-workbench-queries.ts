import type {QueryClient} from '@tanstack/react-query';
import {queryKeys} from '../../app/query-keys';

/** Only an authoritative document read belongs to the command/display barrier.
 * Ancillary queries remain invalidated, but their latency cannot hold a gesture. */
export function refreshWorkbenchQueries(client:QueryClient,changedID:string,hostID:string,hasSnapshot:boolean):Promise<void>{
 const authoritative=hasSnapshot
  ? changedID===hostID?Promise.resolve():client.invalidateQueries({queryKey:queryKeys.document(hostID)})
  : client.invalidateQueries({queryKey:queryKeys.document(changedID)});
 void Promise.all([
  client.invalidateQueries({queryKey:queryKeys.history(changedID),refetchType:'active'}),
  client.invalidateQueries({queryKey:queryKeys.documentProperties(changedID),refetchType:'active'}),
  client.invalidateQueries({queryKey:['product-design-session',hostID]}),
  client.invalidateQueries({queryKey:['context-catalog',hostID]}),
  client.invalidateQueries({queryKey:['product-update-plan',hostID]}),
  client.invalidateQueries({queryKey:['documents']}),
 ]).catch(()=>{/* Each query retains its own error/retry state. */});
 return authoritative;
}
