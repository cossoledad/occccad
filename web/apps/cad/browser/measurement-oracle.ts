import type {Page} from '@playwright/test';

// Explicit read-only numerical oracle, outside command/preview timing. Never
// decorate Artifact DTOs with a fake persisted mass field.
export async function measuredArtifactVolume(page:Page,artifact:{geometryKey?:string}|undefined):Promise<number>{
 const documentID=new URL(page.url()).pathname.split('/documents/')[1]?.split('/')[0];
 if(!documentID||!artifact?.geometryKey)throw new Error('An owned immutable artifact is required for the mass oracle');
 const query=new URLSearchParams({geometryKey:artifact.geometryKey,kind:'BODY',localId:'0'});
 const response=await page.request.get(`/api/documents/${encodeURIComponent(documentID)}/topology-properties?${query}`);
 if(!response.ok())throw new Error(`Explicit body properties failed: ${response.status()} ${await response.text()}`);
 const result=await response.json();if(typeof result.properties?.volume!=='number')throw new Error('Missing exact volume');
 return result.properties.volume;
}
