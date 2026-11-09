import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({server:{middlewareMode:true},appType:'custom',logLevel:'silent'});
try{
 const {curvatureRange,curvatureNumber}=await server.ssrLoadModule('/src/cad/analysis/shape-quality.ts');
 const {CurvatureOverlay}=await server.ssrLoadModule('/src/cad/rendering/curvature-overlay.ts');
 const sample=(value,surface)=>({position:[value,0,0],normal:[0,0,1],minimum:value,maximum:value,mean:value,gaussian:surface?value*value:0,surface});
 const report={documentId:'part',revisionId:'r',result:{complete:true,samples:[sample(0,false),sample(.2,false),sample(-.000001,true),sample(.000002,true)]}};
 assert.deepEqual(curvatureRange(report,'mean'),[-.000001,.000002]);assert.notEqual(curvatureNumber(.000001),'0','small engineering curvature must remain visible');
 const overlay=new CurvatureOverlay();overlay.show(report,'mean',1);assert.equal(overlay.root.children.length,2);
 let disposed=0;for(const object of overlay.root.children)object.geometry.addEventListener('dispose',()=>disposed++);
 overlay.clear();assert.equal(disposed,2);assert.equal(overlay.root.children.length,0);
 overlay.show({...report,result:{...report.result,complete:false}},'mean',1);assert.equal(overlay.root.children.length,0);overlay.dispose();
 console.log('curvature units/ranges, display ownership and disposal: PASS');
}finally{await server.close()}
