import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(new URL("../../../package.json", import.meta.url));
const { createServer } = await import(require.resolve("vite"));
const server = await createServer({appType:"custom",logLevel:"silent",server:{middlewareMode:true}});
try {
  const {LineSketchTool, CircleSketchTool, RectangleSketchTool, RegularPolygonSketchTool, ArcSketchTool,
    PolylineSketchTool, SplineSketchTool, PointSketchTool,ControlSplineSketchTool,ThreePointCircleSketchTool,ThreePointArcSketchTool,CenterRectangleSketchTool,OrientedRectangleSketchTool,EllipseSketchTool,EllipticalArcSketchTool} = await server.ssrLoadModule("/src/cad/tool/cad-tool.ts");
  const {ToolManager} = await server.ssrLoadModule("/src/cad/tool/tool-manager.ts");
  const operations=[], previews=[], prompts=[];
  let completions=0, snap;
  const context={viewport:{hasActiveSketch:()=>true,sketchPoint:(x,y)=>[x,y],sketchSnapReference:()=>snap,
    showPolylinePreview:(points,closed)=>previews.push({points,closed}),showPointPreview:()=>{},showReferenceDimensions:()=>{},
    clearToolPreview:()=>{},setToolPrompt:value=>prompts.push(value),commitSketchOperations:value=>operations.push(value),
    finishToolUse:()=>completions++}};
  const pointer=(x,y,phase="down",button=0)=>({x,y,phase,pointerId:1,button,state:{buttons:{left:phase==="down"&&button===0,middle:false,right:button===2}},originalEvent:{timeStamp:Date.now()}});
  const click=(tool,x,y)=>{tool.pointerDown(pointer(x,y),context);tool.pointerUp(pointer(x,y,"up"),context);};
  const key=(tool,value)=>tool.keyDown({key:value},context);
  const close=(actual,expected)=>assert(Math.abs(actual-expected)<1e-10,`${actual} != ${expected}`);

  const line=new LineSketchTool();line.activate(context);
  click(line,5,7);line.pointerMove(pointer(13,11,"move"),context);
  const before=operations.length;
  for(const value of ["2","0","Tab","9","0"])assert.equal(key(line,value),"consumed");
  assert.equal(operations.length,before,"typing and pointer motion are preview only");
  key(line,"Enter");
  const first=operations.at(-1)[0].entity;
  close(first.start.x,5);close(first.start.y,7);close(first.end.x,5);close(first.end.y,27);
  assert.equal(completions,1,"confirmed creation publishes completion while mode owns continuation");
  click(line,30,10);click(line,40,10);
  assert.notEqual(operations.at(-1)[0].entity.id,first.id,"continuous creations use unique identities");
  assert.equal(operations.length,before+2);

  snap={target:"ENTITY",entityId:"boundary",subElement:"END"};
  click(line,0,0);line.pointerMove(pointer(12,0,"move"),context);
  key(line,"5");key(line,"Enter");
  assert.equal(operations.at(-1).filter(op=>op.constraint).length,1,"numeric endpoint cannot silently inherit cursor snap");
  assert.equal(operations.at(-1)[1].constraint.references[0].subElement,"START");snap=undefined;

  click(line,0,0);const validCount=operations.length;
  for(const value of ["-","2","Enter"])key(line,value);
  assert.equal(operations.length,validCount,"invalid numeric creation must not commit");
  assert(prompts.at(-1).includes("退化"));key(line,"Escape");
  click(line,0,0);click(line,1,1);assert.equal(operations.length,validCount+1,"cancel clears invalid input");

  for(const [Tool,points] of [[LineSketchTool,[[0,0],[4,3]]],[CircleSketchTool,[[0,0],[4,0]]],
    [RectangleSketchTool,[[0,0],[4,3]]],[RegularPolygonSketchTool,[[0,0],[4,0]]],
    [ArcSketchTool,[[0,0],[4,0],[0,4]]],[PointSketchTool,[[2,3]]]]){
    const tool=new Tool();tool.activate(context);assert.equal(key(tool,"c"),"consumed");
    for(const point of points)click(tool,...point);
    const entities=operations.at(-1).filter(op=>op.entity).map(op=>op.entity);
    assert(entities.length>0);assert(entities.every(entity=>entity.role==="CONSTRUCTION"));
    if(Tool===RectangleSketchTool){assert.equal(entities.length,4);assert.equal(operations.at(-1).filter(op=>["HORIZONTAL","VERTICAL"].includes(op.constraint?.kind)).length,4);}
  }
  for(const Tool of [PolylineSketchTool,SplineSketchTool]){
    const tool=new Tool();tool.activate(context);key(tool,"c");
    for(const point of [[0,0],[20,10],[40,0]])click(tool,...point);
    const previous=operations.length;
    assert.equal(tool.pointerDown(pointer(40,0,"down",2),context),"consumed");
    assert.equal(operations.length,previous+1,"right click atomically confirms current curve");
    assert(operations.at(-1).filter(op=>op.entity).every(op=>op.entity.role==="CONSTRUCTION"));
  }
  const contour=new PolylineSketchTool();click(contour,0,0);click(contour,10,0);
  key(contour,"t");click(contour,20,10);key(contour,"t");click(contour,0,10);click(contour,0,0);
  const contourOps=operations.at(-1),contourEntities=contourOps.filter(op=>op.entity).map(op=>op.entity);
  assert.deepEqual(contourEntities.map(entity=>entity.kind),["LINE","ARC","LINE","LINE"]);
  const tangentArc=contourEntities[1];close(tangentArc.center.x,10);close(tangentArc.center.y,10);close(tangentArc.radius,10);
  close(tangentArc.radius*Math.cos(tangentArc.startAngle)+tangentArc.center.x,10);
  close(tangentArc.radius*Math.sin(tangentArc.startAngle)+tangentArc.center.y,0);
  close(tangentArc.radius*Math.cos(tangentArc.endAngle)+tangentArc.center.x,20);
  close(tangentArc.radius*Math.sin(tangentArc.endAngle)+tangentArc.center.y,10);
  assert.equal(contourOps.filter(op=>op.constraint?.kind==="TANGENT").length,1);
  assert.equal(contourOps.filter(op=>op.constraint?.kind==="COINCIDENT").length,4,"mixed contour closure comes from model connections");
  const invalidContour=new PolylineSketchTool();click(invalidContour,0,0);click(invalidContour,10,0);key(invalidContour,"t");
  const contourCount=operations.length;click(invalidContour,20,0);
  assert.equal(operations.length,contourCount,"no finite tangent arc on tangent support line");
  key(invalidContour,"t");click(invalidContour,20,0);key(invalidContour,"Enter");
  assert.equal(operations.at(-1).filter(op=>op.entity).length,2,"invalid arc leaves prior contour intact");

  const controls=new ControlSplineSketchTool();for(const p of [[0,0],[10,10],[20,0]])click(controls,...p);key(controls,"Enter");
  const controlEntity=operations.at(-1)[0].entity;assert.equal(controlEntity.mode,"CONTROL");assert.equal(controlEntity.controlPoints,undefined);
  assert.deepEqual(controlEntity.poles,[{x:0,y:0},{x:10,y:10},{x:20,y:0}]);assert.deepEqual(controlEntity.knots,[0,1]);assert.deepEqual(controlEntity.multiplicities,[3,3]);
  assert.equal(new Set(controlEntity.poleIds).size,3);
  const {sampleSketchEntity}=await server.ssrLoadModule("/src/cad/sketch/sketch-geometry.ts");
  const controlSamples=sampleSketchEntity(controlEntity,32);assert.deepEqual(controlSamples[0],[0,0]);assert.deepEqual(controlSamples.at(-1),[20,0]);
  close(controlSamples[16][0],10);close(controlSamples[16][1],5);
  snap={target:"ENTITY",entityId:"boundary",subElement:"END"};
  const noAuto=new LineSketchTool();key(noAuto,"a");click(noAuto,0,0);click(noAuto,5,5);
  assert.equal(operations.at(-1).filter(op=>op.constraint).length,0,"automatic constraints can be disabled while temporary snap remains");
  snap=undefined;

  const circle=new ThreePointCircleSketchTool();
  for(const point of [[13,20],[10,23],[7,20]])click(circle,...point);
  const circleEntity=operations.at(-1)[0].entity;
  assert.equal(circleEntity.kind,"CIRCLE");close(circleEntity.center.x,10);close(circleEntity.center.y,20);close(circleEntity.radius,3);
  const arc=new ThreePointArcSketchTool();
  for(const point of [[3,0],[0,-3],[-3,0]])click(arc,...point);
  const arcEntity=operations.at(-1)[0].entity;
  assert.equal(arcEntity.kind,"ARC");close(arcEntity.center.x,0);close(arcEntity.center.y,0);close(arcEntity.radius,3);
  close(arcEntity.endAngle-arcEntity.startAngle,-Math.PI);
  const invalidCircle=new ThreePointCircleSketchTool();click(invalidCircle,0,0);click(invalidCircle,10,0);
  const countBeforeInvalid=operations.length;click(invalidCircle,20,0);
  assert.equal(operations.length,countBeforeInvalid,"collinear circle rejected without partial changes");
  click(invalidCircle,10,10);assert.equal(operations.length,countBeforeInvalid+1,"invalid third pick can be corrected");

  const centered=new CenterRectangleSketchTool();click(centered,10,20);click(centered,14,23);
  const centerOps=operations.at(-1),centerEntities=centerOps.filter(op=>op.entity).map(op=>op.entity);
  assert.equal(centerEntities.filter(entity=>entity.role==="PROFILE").length,4);
  assert(centerEntities.some(entity=>entity.kind==="POINT"&&entity.role==="CONSTRUCTION"&&entity.point.x===10&&entity.point.y===20));
  assert.equal(centerOps.filter(op=>op.constraint?.kind==="MIDPOINT").length,1,"center identity is tied to construction diagonal");
  close(centerEntities[0].start.x,6);close(centerEntities[0].start.y,17);
  const oriented=new OrientedRectangleSketchTool();
  for(const point of [[0,0],[3,4],[-1,7]])click(oriented,...point);
  const orientedOps=operations.at(-1),orientedEntities=orientedOps.filter(op=>op.entity).map(op=>op.entity);
  assert.equal(orientedEntities.length,4);
  const a=orientedEntities[0],b=orientedEntities[1];
  close((a.end.x-a.start.x)*(b.end.x-b.start.x)+(a.end.y-a.start.y)*(b.end.y-b.start.y),0);
  assert.equal(orientedOps.filter(op=>op.constraint?.kind==="PARALLEL").length,2);
  assert.equal(orientedOps.filter(op=>op.constraint?.kind==="PERPENDICULAR").length,1);

  const ellipse=new EllipseSketchTool();
  for(const point of [[10,20],[16,28],[6,23]])click(ellipse,...point);
  const ellipseEntity=operations.at(-1)[0].entity;
  assert.equal(ellipseEntity.kind,"ELLIPSE");close(ellipseEntity.majorRadius,10);close(ellipseEntity.minorRadius,5);close(ellipseEntity.rotation,Math.atan2(8,6));
  const ellipticalArc=new EllipticalArcSketchTool();
  for(const point of [[0,0],[10,0],[0,5],[10,0]])click(ellipticalArc,...point);
  key(ellipticalArc,"r");click(ellipticalArc,0,5);
  const ellipticalArcEntity=operations.at(-1)[0].entity;
  assert.equal(ellipticalArcEntity.kind,"ELLIPTICAL_ARC");close(ellipticalArcEntity.endAngle-ellipticalArcEntity.startAngle,-3*Math.PI/2);
  const numericEllipse=new EllipseSketchTool();click(numericEllipse,10,20);const beforeNumericEllipse=operations.length;
  for(const input of ["1","0","Tab","9","0","Enter","5"])key(numericEllipse,input);
  numericEllipse.pointerMove(pointer(200,300,"move"),context);assert.equal(operations.length,beforeNumericEllipse,"axis numeric steps and pointer motion stay temporary");key(numericEllipse,"Enter");
  const numericEllipseEntity=operations.at(-1)[0].entity;close(numericEllipseEntity.rotation,Math.PI/2);close(numericEllipseEntity.majorRadius,10);close(numericEllipseEntity.minorRadius,5);assert.deepEqual(numericEllipseEntity.center,{x:10,y:20});
  const numericArc=new EllipticalArcSketchTool();click(numericArc,0,0);
  for(const input of ["1","0","Enter","5","Enter","3","0","Enter"])key(numericArc,input);
  const beforeNumericArc=operations.length;for(const input of ["3","0","Enter"])key(numericArc,input);assert.equal(operations.length,beforeNumericArc,"zero numeric arc sweep cannot commit");
  for(const input of ["Backspace","Backspace","1","2","0","Enter"])key(numericArc,input);
  const numericArcEntity=operations.at(-1)[0].entity;close(numericArcEntity.startAngle,Math.PI/6);close(numericArcEntity.endAngle,2*Math.PI/3);
  const cancelNumeric=new EllipseSketchTool();click(cancelNumeric,0,0);key(cancelNumeric,"5");key(cancelNumeric,"Escape");click(cancelNumeric,0,0);for(const input of ["8","Enter","4","Enter"])key(cancelNumeric,input);close(operations.at(-1)[0].entity.majorRadius,8);
  const invalidEllipse=new EllipseSketchTool();click(invalidEllipse,0,0);click(invalidEllipse,10,0);
  const ellipseCount=operations.length;click(invalidEllipse,0,10);
  assert.equal(operations.length,ellipseCount,"equal ellipse axes explicitly rejected in favor of circle");
  click(invalidEllipse,0,5);assert.equal(operations.length,ellipseCount+1);

  const manager=new ToolManager(context);manager.register(new LineSketchTool());manager.register({id:"select"});manager.activate("sketch.line");
  manager.pointerDown(pointer(0,0));manager.pointerUp(pointer(0,0,"up"));
  assert.equal(manager.keyDown({key:"Escape"}),"consumed");assert.equal(manager.activeToolID,"sketch.line");
  manager.keyDown({key:"Escape"});assert.equal(manager.activeToolID,"select","second Escape exits idle creation");
  manager.activate("sketch.line");const count=operations.length;
  manager.pointerDown(pointer(0,0));manager.pointerCancel(pointer(0,0,"cancel"));
  assert.equal(operations.length,count,"lost capture/cancel cannot commit incomplete geometry");
  assert.equal(manager.keyDown({key:"c",editableTarget:true}),"ignored","editing an input cannot toggle construction");

  const heldArc=new ArcSketchTool();click(heldArc,0,0);click(heldArc,5,0);const reversePointer=pointer(0,5);reversePointer.state.modifiers={ctrl:true};heldArc.pointerDown(reversePointer,context);heldArc.pointerUp(pointer(0,5,'up'),context);close(operations.at(-1)[0].entity.endAngle,-3*Math.PI/2);
  const heldEllipse=new EllipticalArcSketchTool();for(const [x,y]of [[0,0],[10,0],[0,5],[10,0]])click(heldEllipse,x,y);
  const ellipseEnd=pointer(0,5);ellipseEnd.state.modifiers={ctrl:true};heldEllipse.pointerDown(ellipseEnd,context);heldEllipse.pointerUp(pointer(0,5,'up'),context);close(operations.at(-1)[0].entity.endAngle,-3*Math.PI/2);
  console.log("Sketch creation numeric input, continuous creation, construction, cancellation and role contracts passed");
} finally {await server.close();}
