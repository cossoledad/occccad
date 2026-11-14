import assert from 'node:assert/strict';
import {createServer} from 'vite';
const server=await createServer({appType:'custom',logLevel:'silent',server:{middlewareMode:true}});
try{
 const {builtinCatalog,validateCatalog}=await server.ssrLoadModule('/src/cad/command/workbench-catalog.ts');
 const {compileIconCatalog}=await server.ssrLoadModule('/src/cad/overlay/icon-catalog.ts');
 const {treeNodeIcon}=await server.ssrLoadModule('/src/features/workbench/tree-node-descriptors.tsx');
 const {treeData}=await server.ssrLoadModule('/src/features/workbench/workbench-tree-model.tsx');
 validateCatalog(builtinCatalog);
 const key=icon=>icon.props.children[0].props.children.props.name;
 for(const kind of ['PRODUCT','PART','INSTANCE','BODY','GEOMETRICAL_SET','SKETCH','PLANE','DATUM_POINT','REFERENCE_POINT','AXIS_SYSTEM','PARAMETER','PUBLICATION','IMPORT'])assert(builtinCatalog.icons[key(treeNodeIcon({kind}))],kind);
 for(const [type,command] of [['FILLET','part.fillet'],['HOLE','part.hole'],['SURFACE_EXTRUDE','geometry.surface-extrude'],['SURFACE_SWEEP','geometry.surface-sweep'],['SURFACE_TRIM','geometry.surface-trim']]){
  assert.equal(key(treeNodeIcon({kind:'FEATURE',entityType:type})),builtinCatalog.commands.find(c=>c.id===command).iconKey,'tree and toolbar share semantic artwork');
 }
 const leaf={id:'stable-row',kind:'FEATURE',name:'Arbitrary user name',entityId:'stable-feature',entityType:'GEOMETRY',documentId:'owner',versionId:'revision'};
 const view={document:{id:'owner',versionId:'revision',type:'PART',permission:'OWNER'},structureTree:{id:'root',kind:'PART',name:'Part',documentId:'owner',versionId:'revision',children:[leaf]},part:{features:[{id:'stable-feature',type:'GEOMETRY',geometry:{kind:'SURFACE_EXTRUDE'}}]}};
 const before=JSON.stringify(view),mapped=treeData(view)[0].children[0];assert.equal(key(mapped.icon),'surface-extrude');assert.equal(mapped.key,'stable-row');assert.equal(JSON.stringify(view),before,'presentation lookup never rewrites feature types, names or identities');
 leaf.versionId='older-revision';assert.equal(key(treeData(view)[0].children[0].icon),'body','a different Revision must not borrow a current geometry definition');
 assert.throws(()=>compileIconCatalog({schemaVersion:1,symbols:{},icons:{missing:{family:'solid',parts:['missing']}}}),/Missing icon symbol/);
 assert.throws(()=>compileIconCatalog({schemaVersion:1,symbols:{},icons:{empty:{family:'solid'}}}),/Empty icon/);
 assert.throws(()=>compileIconCatalog({schemaVersion:1,symbols:{},icons:{invalid:{family:'unknown'}}}),/Invalid icon/);
 const source={schemaVersion:1,symbols:{line:[{tag:'path',attributes:{d:'M3 3 17 17'}}]},icons:{composed:{family:'curve',parts:['line'],elements:[{tag:'circle',attributes:{cx:'3',cy:'3',r:'1'}}]}}};
 assert.equal(compileIconCatalog(source).composed.elements.length,2);assert.equal(source.symbols.line.length,1,'composition does not mutate reusable parts');
}finally{await server.close()}
