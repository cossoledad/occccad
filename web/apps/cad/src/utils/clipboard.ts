// Called only by an explicit user action. Retain a fallback for HTTP CAD hosts.
export async function copyTextToClipboard(text:string):Promise<boolean>{
  try{if(navigator.clipboard){await navigator.clipboard.writeText(text);return true;}}catch{/* use selected text fallback */}
  if(typeof document==="undefined")return false;
  const active=document.activeElement as HTMLElement|null, input=document.createElement("textarea");
  input.value=text;input.style.position="fixed";input.style.opacity="0";document.body.appendChild(input);
  try{input.select();return document.execCommand("copy");}catch{return false;}finally{input.remove();active?.focus();}
}
