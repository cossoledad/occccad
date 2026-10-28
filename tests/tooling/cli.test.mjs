import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

// Exercise mise's real argument boundary in a checkout fixture. No CAD build,
// database or external storage is available even if the wrapper regresses.
test("mise preserves missing/empty values for strict CLI validation and pure JSON", (t) => {
  const repo = path.resolve(import.meta.dirname, "../.."),
    root = fs.mkdtempSync(path.join(os.tmpdir(), "occccad-cli-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const d of ["dev/tasks/cpp", "build/tools", "services", "config"])
    fs.mkdirSync(path.join(root, d), { recursive: true });
  fs.writeFileSync(path.join(root, "services/go.mod"), "module fixture\n");
  fs.writeFileSync(
    path.join(root, "mise.toml"),
    '[settings.task]\nrun_auto_install=false\n[task_config]\nincludes=["dev/tasks"]\n',
  );
  for (const f of [
    "dev/entry",
    "dev/tasks/cpp/build",
    "dev/tasks/plan",
    "build/tools/occccad-dev",
  ]) {
    fs.copyFileSync(path.join(repo, f), path.join(root, f));
    fs.chmodSync(path.join(root, f), 0o755);
  }
  const env = {
    ...process.env,
    MISE_CONFIG_DIR: path.join(root, "config"),
    MISE_TRUSTED_CONFIG_PATHS: root,
  };
  for (const args of [
    ["cpp:build", "--target"],
    ["cpp:build", "--target", ""],
    ["plan", "--jobs"],
    ["plan", "--jobs", "0"],
    ["plan", "--match", ""],
    ["plan", "--jobs", "--json"],
  ]) {
    const result = spawnSync("mise", ["run", "--quiet", ...args], {
      cwd: root,
      env,
      encoding: "utf8",
      timeout: 15000,
    });
    assert.ifError(result.error);
    assert.notEqual(result.status, 0, args.join(" "));
    assert.match(
      result.stderr,
      /needs an argument|requires a non-empty value|positive decimal integer/,
    );
    assert.ok(
      !result.stderr.includes("OCCCCAD_NATIVE_COMMAND"),
      "validation reached native build",
    );
    if (args.includes("--json"))
      assert.equal(JSON.parse(result.stdout).status, "FAIL");
  }
  const help = spawnSync("mise", ["run", "--quiet", "plan", "--help"], {
    cwd: root,
    env,
    encoding: "utf8",
    timeout: 15000,
  });
  assert.equal(help.status, 0, help.stderr);
  assert.match(help.stderr, /-scope/);
  assert.ok(!fs.existsSync(path.join(root, "build/cmake")));
});

test("mise cancellation fails and cleans detached owned descendants with live redacted logs", {timeout:15000}, async t => {
  const {spawn}=await import('node:child_process');
  const repo=path.resolve(import.meta.dirname,'../..'),root=fs.mkdtempSync(path.join(os.tmpdir(),'occccad-cancel-'));
  const delay=ms=>new Promise(r=>setTimeout(r,ms));
  for(const d of['dev/tasks/web','build/tools','services','web/apps/cad','config','bin'])fs.mkdirSync(path.join(root,d),{recursive:true});
  fs.writeFileSync(path.join(root,'services/go.mod'),'module fixture\n');
  fs.writeFileSync(path.join(root,'mise.toml'),'[settings.task]\nrun_auto_install=false\n[task_config]\nincludes=["dev/tasks"]\n');
  for(const f of['dev/entry','dev/tasks/web/build','build/tools/occccad-dev']){fs.copyFileSync(path.join(repo,f),path.join(root,f));fs.chmodSync(path.join(root,f),0o755)}
  const pidfile=path.join(root,'owned.pid'),nonce='fixture-cancel-'+Date.now();
  fs.writeFileSync(path.join(root,'work.mjs'),`import fs from 'node:fs';import{spawn}from'node:child_process';process.on('SIGTERM',()=>{});const child=spawn(process.execPath,['-e',${JSON.stringify(`process.on('SIGTERM',()=>{});setInterval(()=>{},1000); /* ${nonce} */`)}],{detached:true,stdio:'ignore'});fs.writeFileSync(${JSON.stringify(pidfile)},String(child.pid));console.log('first-phase ${nonce} postgres://actor:fixture-password@host/db');setInterval(()=>{},1000);`);
  fs.writeFileSync(path.join(root,'bin/pnpm'),'#!/bin/sh\nexec '+JSON.stringify(process.execPath)+' '+JSON.stringify(path.join(root,'work.mjs'))+' "$@"\n',{mode:0o755});
  const env={...process.env,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,MISE_CONFIG_DIR:path.join(root,'config'),MISE_TRUSTED_CONFIG_PATHS:root,OCCCCAD_ADMIN_PASSWORD:nonce};
  const foreign=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});
  const child=spawn('mise',['run','web:build'],{cwd:root,env,detached:true,stdio:['ignore','pipe','pipe']});
  let ended=false,stderr='';const done=new Promise(resolve=>child.once('exit',(code,signal)=>{ended=true;resolve({code,signal})}));child.stderr.on('data',b=>stderr=(stderr+b).slice(-32768));child.stdout.resume();
  let ownedPid;
  try{
    let logs=[];
    for(let i=0;i<100;i++){if(ended)throw Error('fixture ended before cancellation: '+stderr);logs=fs.existsSync(path.join(root,'build/logs'))?fs.readdirSync(path.join(root,'build/logs')).filter(p=>p.endsWith('.log')):[];if(fs.existsSync(pidfile)&&logs.some(p=>fs.readFileSync(path.join(root,'build/logs',p),'utf8').includes('first-phase')))break;await delay(50)}
    assert.ok(fs.existsSync(pidfile),'native command was not launched');ownedPid=Number(fs.readFileSync(pidfile,'utf8'));assert.ok(ownedPid>1);
    const before=logs.map(p=>fs.readFileSync(path.join(root,'build/logs',p),'utf8')).join('');
    assert.ok(before.includes('first-phase'),'log was buffered until task exit');assert.ok(!before.includes(nonce)&&!before.includes('actor:fixture-password'),'plaintext persisted before cancellation');
    process.kill(-child.pid,'SIGTERM');const result=await done;assert.notEqual(result.code,0,'cancelled build succeeded');
    let alive=true;for(let i=0;i<100;i++){try{const stat=fs.readFileSync(`/proc/${ownedPid}/stat`,'utf8');alive=!stat.slice(stat.lastIndexOf(')')+1).trim().startsWith('Z ')}catch{alive=false}if(!alive)break;await delay(50)}
    assert.equal(alive,false,'detached TERM-ignoring descendant remains');assert.equal(foreign.exitCode,null,'foreign process killed');
    const all=fs.readdirSync(path.join(root,'build/logs')).map(p=>fs.readFileSync(path.join(root,'build/logs',p),'utf8')).join('');assert.ok(!all.includes(nonce)&&!all.includes('actor:fixture-password'));
  }finally{
    try{process.kill(-child.pid,'SIGTERM')}catch{}
    if(ownedPid){try{const cmd=fs.readFileSync(`/proc/${ownedPid}/cmdline`,'utf8');if(cmd.includes(nonce))process.kill(ownedPid,'SIGKILL')}catch{}}
    foreign.kill('SIGTERM');await new Promise(r=>foreign.once('exit',r));
    fs.rmSync(root,{recursive:true,force:true});
  }
});
