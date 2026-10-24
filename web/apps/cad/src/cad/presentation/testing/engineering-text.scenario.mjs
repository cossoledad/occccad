import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url),{createServer}=await import(require.resolve('vite'));
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {engineeringLabel,objectName}=await server.ssrLoadModule('/src/cad/presentation/engineering-text.ts');
 const {describeAssemblyReference}=await server.ssrLoadModule('/src/cad/assembly/assembly-reference-presentation.ts');
 const {normalizeOperationFailure}=await server.ssrLoadModule('/src/cad/command/operation-failure.ts');
 const {motionPresentations}=await server.ssrLoadModule('/src/cad/assembly/motion-presentation.ts');
 const view={document:{id:'owning-document',versionId:'accepted-revision'},product:{instances:[{id:'one',name:'轴'},{id:'two',name:'轴'},{id:'three',name:'轴'}],constraints:[]}};
 const raw=JSON.stringify(view);
 assert.notEqual(objectName(view,'one'),objectName(view,'two'));assert.match(objectName(view,'unknown-uuid'),/丢失/);assert(!objectName(view,'unknown-uuid').includes('uuid'));
 const subset={...view,product:{...view.product,instances:view.product.instances.slice(1)}};
 const rows=motionPresentations(subset,undefined,'',view);assert.match(rows[0].name,/第 2 个实例/);assert.match(rows[1].name,/第 3 个实例/);
 assert.equal(engineeringLabel('UNRECOGNIZED_ENUM'),'待检查');
 const reference=describeAssemblyReference({instanceId:'missing',kind:'FACE',geometryId:'secret-geometry-id',persistentSelection:{anchor:{featureId:'feature-uuid',outputSlot:'secret-semantic'}}},view);
 assert.match(reference.primary,/丢失/);assert.doesNotMatch(reference.secondary,/secret|uuid/);
 const error={code:'VALIDATION_FAILED',message:'PROFILE_REGION_MISSING / uuid / rpc snapshot'};const display=normalizeOperationFailure(error);assert.match(display.message,/原轮廓区域.*重新选择/);assert.equal(display.details,error.message);
 assert.match(normalizeOperationFailure({message:'TypeError: unexplained internal-uuid'}).message,/原因尚无法确定/);
 assert.match(normalizeOperationFailure({diagnostic:'PROFILE_REGION_MISSING / exact reference'}).message,/重新选择区域/);
 assert.match(normalizeOperationFailure({message:{unexpected:'payload'}}).message,/原因尚无法确定/);
 assert.equal(JSON.stringify(view),raw,'labels never modify identity or input');
}finally{await server.close()}
