import type {AssistantTurn,Conversation} from "./assistant-api";

// HTTP completion and poll responses can cross. A stale THINKING response must
// not replace a terminal result or erase a just-created plan from the cache.
export function mergeConversation(current:Conversation|undefined,next:Conversation):Conversation{
  const turns=new Map<string,AssistantTurn>((current?.turns??[]).map(turn=>[turn.id,turn]));
  for(const turn of next.turns){
    const previous=turns.get(turn.id);
    if(previous&&previous.status!=="THINKING"&&turn.status==="THINKING")continue;
    turns.set(turn.id,turn);
  }
  const ordered=[...turns.values()].sort((a,b)=>a.createdAt.localeCompare(b.createdAt)||a.id.localeCompare(b.id));
  return {turns:ordered.slice(-30),hasEarlier:Boolean(current?.hasEarlier||next.hasEarlier||ordered.length>30)};
}
export const latestPlan=(turns:AssistantTurn[])=>turns.slice().reverse().find(turn=>turn.status==="COMPLETED"&&turn.taskId)?.taskId;
