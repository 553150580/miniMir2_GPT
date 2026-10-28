const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'../../..'),source=fs.readFileSync(path.join(root,'tools/client/verify_town_travel_browser.cjs'),'utf8');
const start=source.indexOf(' async function groundSample(point)'),end=source.indexOf(' async function ground(point,requiredZone)',start);assert(start>=0&&end>start);
const helpers=source.slice(source.indexOf('function clearGroundSample('),source.indexOf('function packet('));
const {clearGroundSample,selectSameCenterGround}=new Function('assert',helpers+'\nreturn {clearGroundSample,selectSameCenterGround};')(assert);
const prefab=JSON.parse(fs.readFileSync(path.join(root,'client/assets/resources/prefabs/Entity.prefab'),'utf8')),rootNode=prefab.find(n=>n.__type__==='cc.Node'&&n._name==='Entity'),size=prefab[rootNode._components.find(r=>prefab[r.__id__].__type__==='cc.UITransform').__id__];
function fixture(){
 const owner={x:650,y:631},w={visuals:new Map(),map:{safe_zones:[{id:'106',center:{x:650,y:631},radius:8}]},isMapReady:true};
 const project=(x,y)=>({x:(x-owner.x)*48+360,y:748-(y-owner.y)*32});
 const transform=(x,y)=>({width:size._contentSize.width,height:size._contentSize.height,anchorPoint:size._anchorPoint,convertToNodeSpaceAR:s=>{const foot=project(x,y);return{x:s.x-foot.x,y:s.y-foot.y};}});
 const populate=(x,y,petX,petY)=>{Object.assign(owner,{x,y});w.visuals=new Map([[1,{node:{activeInHierarchy:true,_uiProps:{uiTransformComp:transform(x,y)}},state:{entityHandle:1,entityKind:1,position:{x,y}}}],[2,{node:{activeInHierarchy:true,_uiProps:{uiTransformComp:transform(petX,petY)}},state:{entityHandle:2,entityKind:2,position:{x:petX,y:petY}}}]]);};populate(650,631,652,631);
 w.projection={project:(x,y)=>({x:x*48+24,y:-y*32-16})};w.world={position:{x:0,y:0},_uiProps:{uiTransformComp:{convertToWorldSpaceAR:p=>project((p.x-24)/48,(-p.y-16)/32)}}};
 w.viewport={_uiProps:{uiTransformComp:{width:720,height:808,anchorPoint:{x:.5,y:.5},convertToNodeSpaceAR:p=>({x:p.x-360,y:p.y-748})}}};w.screenToGrid=p=>({x:Math.round((p.x-360)/48)+owner.x,y:Math.round((748-p.y)/32)+owner.y});
 const a={world:w,canSendInput:()=>true,popup:{top:null},ui:{top:null}},cc={director:{getScene:()=>({getComponentsInChildren:()=>[a]})},Vec3:class{constructor(x=0,y=0){Object.assign(this,{x,y});}},view:{getVisibleSize:()=>({width:720,height:1280})}};
 const page={async evaluate(fn,arg){const previousCC=global.cc,previousDocument=global.document;global.cc=cc;global.document={querySelector:()=>({getBoundingClientRect:()=>({x:0,y:0,width:720,height:1280})})};try{return fn(arg);}finally{global.cc=previousCC;global.document=previousDocument;}}};
 return{sample:new Function('page',source.slice(start,end)+'\nreturn groundSample;')(page),populate};
}
test('实际QA采样重现北一格可走却命中本人；根尺寸取保存prefab，横向四格及回程中心避开本人宠物',async()=>{
 const f=fixture(),north=await f.sample({x:650,y:630});assert.equal(north.walkable,true);assert.deepEqual(north.occupiedBy,[]);assert(north.entityHits.some(hit=>hit.handle===1));assert.equal(north.entityHits[0].width,size._contentSize.width);assert.equal(clearGroundSample(north,'106'),false);
 const east=await f.sample({x:654,y:631});assert.equal(clearGroundSample(east,'106'),true);assert.deepEqual(east.entityHits,[]);
 f.populate(654,631,652,631);const back=await f.sample({x:650,y:631});assert.equal(clearGroundSample(back,'106'),true);assert.deepEqual(back.entityHits,[]);
});
test('实际同区地面选择排除静态阻挡、宠物占格、实体命中、UI覆盖与视口/安全区外，均不发导航',()=>{
 const base={point:{x:654,y:631},walkable:true,withinViewport:true,ready:true,overlay:'',entityHits:[],occupiedBy:[],zone:'106'},bad=[{...base,walkable:false},{...base,occupiedBy:[2]},{...base,entityHits:[{handle:2}]},{...base,overlay:'StatusDetails'},{...base,withinViewport:false},{...base,zone:'0'}];
 assert.deepEqual(selectSameCenterGround(bad.concat(base),'106'),base.point);for(const sample of bad)assert.equal(clearGroundSample(sample,'106'),false);assert.throws(()=>selectSameCenterGround(bad,'106'),/没有真实可轻触地面/);
});
