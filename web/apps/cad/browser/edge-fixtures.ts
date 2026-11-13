import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {fileURLToPath} from 'node:url';
import fs from 'node:fs/promises';
import path from 'node:path';

const root=fileURLToPath(new URL('../../../../',import.meta.url));
export const edgeFixtures=path.join(root,'build/agent-work/natural-edges/fixtures');

// Both browser suites use the registered native producer, so a clean checkout
// needs no manually saved binary model and cannot silently use a stale fixture.
export async function prepareEdgeFixtures(){
 const logs=path.join(root,'build/agent-check/natural-edges');await fs.mkdir(logs,{recursive:true});
 try {
  const {stdout,stderr}=await promisify(execFile)('mise',['run','check','--scope','geometry','--match','EdgeProfile','--jobs','4'],{
   cwd:root,env:{...process.env,OCCCCAD_EDGE_FIXTURES:edgeFixtures},timeout:180_000,maxBuffer:8*1024*1024,
  });
  await fs.writeFile(path.join(logs,'browser-fixture-preparation.log'),stdout+stderr);
 } catch(error:any){
  await fs.writeFile(path.join(logs,'browser-fixture-preparation.log'),String(error.stdout??'')+String(error.stderr??'')+String(error));
  throw error;
 }
}
