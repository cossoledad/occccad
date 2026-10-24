/** Definition names are allocated from the accepted scope, never array order. */
export function nextObjectName(prefix:string,names:readonly string[]):string {
 let last=0;const used=new Set(names.map(n=>n.toLocaleLowerCase()));
 for(const name of names){if(name.toLowerCase().startsWith(prefix.toLowerCase()+'.')){const suffix=name.slice(prefix.length+1);if(/^\d+$/.test(suffix)&&Number.isSafeInteger(Number(suffix)))last=Math.max(last,Number(suffix));}}
 for(let ordinal=last+1;;ordinal++){const candidate=`${prefix}.${ordinal}`;if(!used.has(candidate.toLocaleLowerCase()))return candidate;}
}
