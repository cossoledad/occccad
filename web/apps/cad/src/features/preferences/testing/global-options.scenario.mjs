import assert from "node:assert/strict";
import {createServer} from "vite";
const server=await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
 const {useUIPreferences,migrateUIPreferences:migrate}=await server.ssrLoadModule("/src/state/ui-preferences.ts");
 const {globalOptionsSnapshot}=await server.ssrLoadModule("/src/features/preferences/global-options-model.ts");
 const {sceneGridColor,sceneBackgroundIsLight,DEFAULT_SCENE_SETTINGS,engineeringScenePreset}=await server.ssrLoadModule("/src/cad/rendering/scene-settings.ts");
 const before=globalOptionsSnapshot(useUIPreferences.getState()),draft=globalOptionsSnapshot(before);
 draft.sceneSettings.backgroundTop="#001122";draft.creationDefaults.solid.color="#ff7700";
 assert.deepEqual(globalOptionsSnapshot(useUIPreferences.getState()),before,"preview draft must not persist");
 useUIPreferences.getState().applyGlobalOptions(draft);
 const saved=globalOptionsSnapshot(useUIPreferences.getState());
 assert.equal(saved.sceneSettings.backgroundTop,"#001122");assert.equal(saved.creationDefaults.solid.color,"#ff7700");
 assert.equal(before.creationDefaults.solid.color,"#8e9faf","creation defaults are copied, not shared with open drafts");
 assert.equal(typeof migrate,"function");
 if(migrate){
  const old={sceneSettings:{...DEFAULT_SCENE_SETTINGS,azimuth:-126.87,elevation:54.46},creationDefaults:{solid:{color:'#98a8b8',roughness:.5,metalness:.04}}};
  old.sketchLabelPositions={'["part","sketch","constraint"]':{x:20,y:35,anchor:{x:2,y:4}}};
  const next=await migrate(old,13);assert.deepEqual(next.sketchLabelPositions,old.sketchLabelPositions);
  assert.equal(next.sceneSettings.azimuth,DEFAULT_SCENE_SETTINGS.azimuth);assert.equal(next.creationDefaults.solid.color,'#8e9faf');assert.equal(next.creationDefaults.solid.metalness,.02);
  const custom=structuredClone(old);custom.sceneSettings.azimuth=22;custom.sceneSettings.background='SOLID';custom.sceneSettings.backgroundTop='#123456';custom.creationDefaults.solid.color='#aabbcc';
  const kept=await migrate(custom,13);assert.equal(kept.sceneSettings.azimuth,22);assert.equal(kept.sceneSettings.backgroundTop,'#123456');assert.deepEqual(kept.creationDefaults,custom.creationDefaults);
 }
 if(migrate){const migrated=await migrate({documentSceneSettings:{a:{backgroundTop:"#ff0000"}},displayLengthUnit:"cm"},9);assert.deepEqual(migrated.sceneSettings,DEFAULT_SCENE_SETTINGS);assert.equal(migrated.displayLengthUnit,"cm")}
 assert.notEqual(sceneGridColor({...DEFAULT_SCENE_SETTINGS,...engineeringScenePreset("DARK")}),sceneGridColor(DEFAULT_SCENE_SETTINGS),"light and dark backgrounds need distinct grid colors");
 assert.equal(sceneGridColor({...DEFAULT_SCENE_SETTINGS,gridAutoColor:false,gridColor:"#112233"}),"#112233");
 assert.equal(sceneBackgroundIsLight({...DEFAULT_SCENE_SETTINGS,background:"SOLID",backgroundTop:"#ffffff",gridAutoColor:false,gridColor:"#ffffff"}),true,"tree contrast follows the background even with a custom grid colour");
 const {structureMenu}=await server.ssrLoadModule("/src/features/workbench/tree-context-menu.ts");
 const menu=structureMenu([{key:"edit"},{key:"visibility"},{key:"delete",disabled:true},{key:"properties"}],"Body.1");
 assert.deepEqual(menu.map(i=>i.key??i.type),["visibility","divider","object-actions","divider","properties"]);
 assert.equal(menu.find(i=>i.key==="object-actions").children[0].key,"edit");
 assert.deepEqual(structureMenu([{key:"edit"},{key:"delete"},{key:"properties"}],"Body.1",true).map(i=>i.key??i.type),["delete","divider","properties"],"batch menus must not edit only the clicked member");
 const {propertyEditorTarget}=await server.ssrLoadModule("/src/features/workbench/property-target.ts");
 const view={document:{id:"product",versionId:"r1",type:"PRODUCT"},product:{instances:[],kinematics:{mechanisms:[{id:"m",name:"Mechanism",joints:[]}],drivers:[],studies:[]}}};
 const target=propertyEditorTarget(view,view,{kind:"kinematic-object",id:"m",documentId:"product"},"",true,false);
 assert.equal(target.target.kind,"KINEMATIC_OBJECT");assert.equal(target.name,"Mechanism");assert.equal(target.canEdit,false,"inspection must not grant frozen rename permission");
} finally {await server.close()}
