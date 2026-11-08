import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {createServer} from 'vite';
const require=createRequire(import.meta.url),React=require('react'),{renderToStaticMarkup}=require('react-dom/server'),{App}=require('antd');
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {normalizeOperationFailure,OperationFailureGate,useOperationFeedback}=await server.ssrLoadModule('/src/cad/command/operation-feedback.tsx');
 const {CommandInputError}=await server.ssrLoadModule('/src/cad/command/operation-failure.ts');
 const failure={code:'TIMEOUT',message:'preview timeout',requestId:'preview-retry',diagnosticId:'doc/preview-failure'};
 const gate=new OperationFailureGate();assert.equal(gate.accept(failure,'command',100),true);assert.equal(gate.accept(failure,'dialog',101),false);gate.retry(failure,'command');assert.equal(gate.accept(failure,'command',102),true);assert.equal(gate.accept(failure,'dialog',103),false);
 const notices=[],dismissals=[],original=App.useApp;let feedback;
 App.useApp=()=>({notification:{error:config=>notices.push(config),warning:config=>notices.push(config),destroy:key=>dismissals.push(key)}});
 try{
  function Probe(){feedback=useOperationFeedback();return null}renderToStaticMarkup(React.createElement(Probe));
  const report=()=>feedback(failure,'拉伸',{retry:report,retryLabel:'重试'});
  report();const notice=notices.at(-1);assert.equal(notice.duration,0);assert.equal(notice.className,'cad-operation-notification');
  notice.description.props.children.find(child=>child?.props?.children==='重试').props.onClick();
  assert.equal(notices.length,2,'explicit retry can report the same idempotent request immediately');assert.equal(dismissals.length,1);assert.equal(normalizeOperationFailure(failure).diagnosticId,'doc/preview-failure');
  feedback(failure,'另一个出口');assert.equal(notices.length,2,'same rejected operation retains one outlet');
  const repair=Object.assign(new CommandInputError('下游需要修复：倒角.1'),{code:'FEATURE_DOWNSTREAM_REPAIR_REQUIRED'});
  assert.equal(normalizeOperationFailure(repair).severity,'warning');assert.equal(normalizeOperationFailure(repair).retryable,false);
  feedback(repair,'拉伸');assert.equal(notices.at(-1).title,'拉伸预览');assert.match(renderToStaticMarkup(notices.at(-1).description),/倒角.1/);
 }finally{App.useApp=original}
}finally{await server.close()}
