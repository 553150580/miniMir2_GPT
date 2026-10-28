const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const runtime=require('../support/world_runtime.cjs'),{cc,UITransform,Sprite,Vec3,Long,state,event}=runtime;
const root=path.resolve(__dirname,'../../..'),resourceRoot=path.join(root,'client/assets/resources');
const primary=runtime.makeWorld(1),GameWorldView=primary.view.constructor,WorldAssets=primary.assets.constructor;primary.view.dispose();
cc.Node.prototype.setScale=function(x,y,z){this.scale=new Vec3(x,y,z);};
function ancestry(node){const nodes=[];while(node){nodes.push(node);node=node.parent;}return nodes;}
UITransform.prototype.convertToWorldSpaceAR=function(point){const value=new Vec3(point.x,point.y,point.z??0);for(const node of ancestry(this.node)){value.x=value.x*node.scale.x+node.position.x;value.y=value.y*node.scale.y+node.position.y;}return value;};
UITransform.prototype.convertToNodeSpaceAR=function(point){const value=new Vec3(point.x,point.y,point.z??0);for(const node of ancestry(this.node).reverse()){value.x=(value.x-node.position.x)/node.scale.x;value.y=(value.y-node.position.y)/node.scale.y;}return value;};
class Graphics{clear(){}rect(){}fill(){}stroke(){}}
cc.Graphics=Graphics;cc.Color=class{constructor(r,g,b,a){Object.assign(this,{r,g,b,a});}};
const materials=[];cc.Material=class{constructor(){materials.push(this);}copy(_base,options){this.states=options?.states;}overridePipelineStates(){this.illegalRuntimeOverride=true;}destroy(){this.destroyed=true;}};
cc.gfx={BlendFactor:{SRC_ALPHA:2,ONE:1,ONE_MINUS_SRC_ALPHA:4}};Sprite.prototype.getSharedMaterial=function(){return this.customMaterial??{};};
const originalInstantiate=cc.instantiate,allSprites=[];
cc.instantiate=function(prefab){const node=originalInstantiate(prefab?.kind??prefab);for(const child of [node,...node.children]){const sprite=child.getComponent(Sprite);if(sprite)allSprites.push(sprite);}return node;};
function fakeResources(options={}){
 const pool=new Map(),requested=[],pending=[],callbacks=[],sourceActive={now:0,max:0};
 const gameplay=JSON.parse(fs.readFileSync(path.join(resourceRoot,'development/gameplay.json'),'utf8'));
 options.changeGameplay?.(gameplay);
 const load=(key,_type,callback)=>{
  requested.push(key);const source=key.startsWith('source-maps/');if(source){sourceActive.now++;sourceActive.max=Math.max(sourceActive.max,sourceActive.now);}
  const finish=()=>{
   let asset=pool.get(key),error=null;
   try{if(!asset){const json=key==='development/gameplay'?gameplay:fs.existsSync(path.join(resourceRoot,key+'.json'))?JSON.parse(fs.readFileSync(path.join(resourceRoot,key+'.json'),'utf8')):undefined;
    options.changeResource?.(key,json);
    asset={key,json,kind:key.startsWith('prefabs/')?({Entity:'entity',Effect:'effect',MapProp:'prop',Beam:'beam',StatusBadge:'status'})[key.split('/')[1]]:undefined,refs:0,addRef(){this.refs++;},decRef(){assert(this.refs>0,`${key}重复释放`);this.refs--;if(!this.refs)for(const sprite of allSprites)if(sprite.spriteFrame===this&&!ancestry(sprite.node).some(node=>node.destroyed))assert.fail(`${key}引用归零时仍绑定可用Sprite`);}};pool.set(key,asset);}
   }catch(failure){error=failure;}
   if(source)sourceActive.now--;callback(error,asset);
  };
  if(options.delayChunks&&key.includes('/chunks/'))pending.push(finish);else queueMicrotask(finish);
 };
 cc.resources={load};return{pool,requested,pending,gameplay,sourceActive,flush(){for(const finish of pending.splice(0))finish();}};
}
function scene(assets){
 const viewport=new cc.Node('Viewport'),world=new cc.Node('World'),entities=new cc.Node('Entities');viewport.getComponent(UITransform).setContentSize(720,808);world.parent=viewport;entities.parent=world;
 for(const [name,type]of [['Surface',Sprite],['Soil',Graphics],['GridLines',Graphics],['TargetMarker',Sprite]]){const node=new cc.Node(name);node.add(type,new type());node.parent=world;}
 const diamond=new cc.Node('Diamond');diamond.add(Graphics,new Graphics());diamond.parent=world.getChildByName('TargetMarker');new cc.Node('Effects').parent=world;
 const view=new GameWorldView(viewport,world,entities,'来源测试',0,Long.UONE,1);view.prepare(assets,()=>{});return{view,viewport,world,entities};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
async function settle(resources,view,promise){for(let i=0;i<100&&!view.isMapReady;i++){resources.flush();await flush();if(view.mapPreparationError)throw view.mapPreparationError;}await promise;assert.equal(view.isMapReady,true);}
function assertReleased(resources){for(const asset of resources.pool.values())assert.equal(asset.refs,0,asset.key);}

test('WorldAssets无mapId选择目录首个primary；旧profile仍兼容first-map，显式uint64不经Number',async()=>{
 const resources=fakeResources(),assets=await WorldAssets.prepare(0,()=>{});assert.equal(assets.mapId,'1');assert.equal(assets.sourceMap,undefined);assert.equal(assets.map.width,17);assert(assets.background);assert(!resources.requested.some(key=>key.startsWith('source-maps/')));assets.dispose();assertReleased(resources);
 const old=fakeResources({changeGameplay:g=>delete g.map_catalog}),legacy=await WorldAssets.prepare(0,()=>{},Long.UONE);assert.equal(legacy.mapId,'1');assert(old.requested.includes('development/maps/first-map'));legacy.dispose();assertReleased(old);
 const unsupported=fakeResources();await assert.rejects(WorldAssets.prepare(0,()=>{},'9007199254740993'),/没有请求/);assert(!unsupported.requested.includes('development/maps/first-map'));assertReleased(unsupported);
 const number=fakeResources();await assert.rejects(WorldAssets.prepare(0,()=>{},2),/64位对象/);assertReleased(number);
});
test('WorldAssets来源分支使用完整700目录与独立SourceMapAssets，不加载个人图JSON/背景或预载1936块',async()=>{
 const resources=fakeResources(),assets=await WorldAssets.prepare(0,()=>{},Long.fromString('2',true));
 assert.equal(assets.mapId,'2');assert.equal(assets.map.id,'2');assert.equal(assets.map.width,700);assert.equal(assets.map.height,700);assert.equal(assets.map.projection,'source_rectangular');
 assert.equal(assets.background,assets.sourceMap.minimap);assert.equal(assets.mapBackground,assets.background);assert.equal(assets.propPrefab,assets.sourceMap.propPrefab);
 assert(resources.requested.includes('source-maps/map-2/manifest'));assert(!resources.requested.some(key=>key.includes('development/maps/first-map')));assert(!resources.requested.some(key=>key.includes('/chunks/')));
 assert.equal(resources.requested.filter(key=>key==='prefabs/MapProp').length,1);assert.equal(assets.sourceMap.stats.chunks.entries,0);assert(assets.frames.size>0);assets.dispose();assertReleased(resources);
});
test('目录选择与来源资源身份完整保留最大uint64，拒绝数字资源ID冒充另一张图',async()=>{
 const id='18446744073709551615',resources=fakeResources({changeGameplay:g=>g.map_catalog[1].id=id,changeResource:(key,json)=>{if(key==='source-maps/map-2/manifest')json.id=id;}});
 const assets=await WorldAssets.prepare(0,()=>{},Long.MAX_UNSIGNED_VALUE);assert.equal(assets.mapId,id);assert.equal(assets.map.id,id);assets.dispose();assertReleased(resources);
 const wrong=fakeResources({changeResource:(key,json)=>{if(key==='development/maps/first-map')json.id=2;}});await assert.rejects(WorldAssets.prepare(0,()=>{}),/目录身份/);assertReleased(wrong);
});
test('真实跨层来源视口先等待原生块，矩形点击/镜头/方向/位移沿对应几何，换代同步撤销ready及解绑',async()=>{
 const resources=fakeResources({delayChunks:true}),assets=await WorldAssets.prepare(0,()=>{},'2'),{view,viewport,world,entities}=scene(assets);cc.game.totalTime=100;
 assert.equal(view.isMapReady,false);await assert.rejects(view.prepareViewport(),/基线位置/);assert.equal(resources.pending.length,0);
 view.apply({entities:[state(1,1,650,631)]});const preparation=view.prepareViewport();assert.equal(view.isMapReady,false);assert(resources.pending.length>0);
 await settle(resources,view,preparation);assert.deepEqual(view.grid(650,631),new Vec3(31224,-20208));assert.deepEqual(world.position,new Vec3(-31224,20208));
 assert.equal(view.projection.geometry.projection,'source_rectangular');assert.equal(view.logicalDirection(0,32),0);assert.equal(view.logicalDirection(48,0),2);
 const screen=world.getComponent(UITransform).convertToWorldSpaceAR(view.grid(650,631));assert.deepEqual(view.screenToGrid(screen),{x:650,y:631});
 let blocked;for(let y=623;y<=639&&!blocked;y++)for(let x=642;x<=658;x++)if(!assets.sourceMap.model.isWalkable(x,y)){blocked={x,y};break;}
 assert(blocked);const blockedScreen=world.getComponent(UITransform).convertToWorldSpaceAR(view.grid(blocked.x,blocked.y));assert.equal(view.screenToGrid(blockedScreen),null);assert.deepEqual(view.screenToGrid(blockedScreen,false),blocked);
 assert(view.sourceView.stats.visibleNodes>0&&view.sourceView.stats.visibleNodes<2000);assert(view.sourceView.stats.chunks<=49);assert(resources.sourceActive.max<=4);
 assert.equal(view.props.length,0);assert.equal(world.getChildByName('Surface').getComponent(Sprite).enabled,false);assert.equal(world.getChildByName('Soil').active,false);
 view.acceptDisplacement(event({origin:{x:650,y:631},path:[{x:651,y:631}],facingDirection:2,movementDirection:2}));cc.game.totalTime=250;view.update(0,0,true);
 assert.deepEqual(view.visuals.get(1).node.position,view.grid(650.5,631));assert.equal(view.visuals.get(1).movement.position.x,651);
 const invalidations=[];assets.onBeforeInvalidation(reason=>invalidations.push(reason));
 assets.sourceMap.replaceGeneration();assert.deepEqual(invalidations,['source']);assert.equal(view.isMapReady,false);assert.equal(view.sourceView.stats.visibleNodes,0);view.update(0,0,true);await settle(resources,view,view.prepareViewport());
 viewport.getComponent(UITransform).setContentSize(1000,900);view.refitCamera();assert.equal(view.isMapReady,false);await settle(resources,view,view.prepareViewport());
 view.dispose();assert.deepEqual(invalidations,['source','dispose']);assert.equal(view.isMapReady,false);resources.flush();await flush();assertReleased(resources);assert.equal(world.destroyed,undefined);assert.equal(entities.destroyed,undefined);
 // 复用相同保存容器切回个人图，来源分支改过的运行anchor/active必须恢复。
 const personal=await WorldAssets.prepare(0,()=>{}),personalView=new GameWorldView(viewport,world,entities,'个人复入',0,Long.UONE,1);personalView.prepare(personal,()=>{});
 assert.deepEqual(world.getComponent(UITransform).anchorPoint,{x:.5,y:.5});assert.deepEqual(world.getChildByName('Surface').getComponent(UITransform).anchorPoint,{x:.5,y:.5});assert.equal(world.getChildByName('Surface').getComponent(Sprite).enabled,true);assert.equal(world.getChildByName('Soil').active,true);assert.equal(world.getChildByName('GridLines').active,true);assert.equal(personalView.isMapReady,true);await personalView.prepareViewport();personalView.dispose();assertReleased(resources);
});
test('来源流式预算缺失或目录与资产尺寸不符均拒绝，已取得资源成对释放',async()=>{
 const missing=fakeResources({changeGameplay:g=>delete g.source_map_streaming});await assert.rejects(WorldAssets.prepare(0,()=>{},'2'),/显式流式/);assert(!missing.requested.some(key=>key.startsWith('source-maps/')));assertReleased(missing);
 const wrong=fakeResources({changeGameplay:g=>g.map_catalog[1].width=17});await assert.rejects(WorldAssets.prepare(0,()=>{},'2'),/完整尺寸/);assertReleased(wrong);
});
