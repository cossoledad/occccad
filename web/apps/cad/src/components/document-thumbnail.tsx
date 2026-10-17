import { useEffect, useRef, useState } from "react";
import { isMockMode, previewURL } from "../api/client";
import type { DocumentSummary } from "../types";

/** Preview jobs may finish after the list response, or be repaired on first access. */
export function DocumentThumbnail({ document }: { document: DocumentSummary }) {
  const host=useRef<HTMLSpanElement>(null);
  const [image,setImage]=useState<string>();
  const source=previewURL(document.id,document.versionId);
  useEffect(()=>{
    setImage(undefined);
    if(isMockMode||!source||!host.current)return;
    const abort=new AbortController();let timer:ReturnType<typeof setTimeout>|undefined,url:string|undefined,attempt=0;
    const load=async()=>{
      try {
        const response=await fetch(source,{credentials:"include",signal:abort.signal,cache:"no-cache"});
        if(!response.ok)return;
        const blob=await response.blob();if(abort.signal.aborted)return;
        if(url)URL.revokeObjectURL(url);url=URL.createObjectURL(blob);setImage(url);
        if(response.headers.get("X-OCCCCAD-Thumbnail-State")==="pending"&&attempt++<12)
          timer=setTimeout(load,Math.min(5000,750*2**Math.min(attempt,3)));
      } catch { /* The empty scene remains available offline and after cancellation. */ }
    };
    const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting)){observer.disconnect();void load();}},{rootMargin:"160px"});
    observer.observe(host.current);
    return()=>{abort.abort();observer.disconnect();clearTimeout(timer);if(url)URL.revokeObjectURL(url);};
  },[source]);
  return <span ref={host} className={`document-thumbnail${image?"":" fallback"}`}>
    {image?<img decoding="async" src={image} alt={`${document.name} 缩略图`} onError={()=>setImage(undefined)}/>:<span className="empty-scene-preview" aria-label="空场景"/>}
  </span>;
}
