import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url),{createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {ShowcaseWheel}=await server.ssrLoadModule('/src/features/auth/showcase-wheel.ts');
 const mouse=new ShowcaseWheel();assert.equal(mouse.step(100,0,800,0),1);
 for(let t=20;t<700;t+=20)assert.equal(mouse.step(40,0,800,t),0,'inertial burst must not skip all images');
 assert.equal(mouse.step(-3,1,800,900),-1,'line units and reverse direction');
 assert.equal(mouse.step(1,2,800,1700),1,'page units');
 const trackpad=new ShowcaseWheel();for(let t=0;t<5;t++)assert.equal(trackpad.step(8,0,800,t*10),0);
 assert.equal(trackpad.step(8,0,800,50),1,'small deltas accumulate');
 assert.equal(trackpad.step(30,0,800,1000),0);assert.equal(trackpad.step(-30,0,800,1010),0);assert.equal(trackpad.step(-20,0,800,1020),-1,'direction reversal discards the old direction');
 const idle=new ShowcaseWheel();assert.equal(idle.step(30,0,800,0),0);assert.equal(idle.step(30,0,800,300),0,'separate small gestures do not accumulate forever');
 for(const value of [NaN,Infinity,0])assert.equal(idle.step(value,0,800,400),0);
}finally{await server.close()}
