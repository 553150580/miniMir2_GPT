// 只在真实Web Mobile构建中做来源地图渲染诊断；不注册账号、不发RPC/WS、不提交切图或ACK。
const assert=require('node:assert/strict'),crypto=require('node:crypto'),fs=require('node:fs'),path=require('node:path');
const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
const root=path.resolve(__dirname,'../..'),args=process.argv.slice(2),option=(key,fallback)=>{const index=args.indexOf(key);return index<0?fallback:args[index+1];};
if(args.includes('--help')){console.log('用法：.\\tools\\dev.ps1 node tools/client/verify_source_map_browser.cjs [--report-dir 项目内目录] [--base-url http://127.0.0.1:18080/] [--headed]\n需要当前成功的Creator构建和本机静态服务；只运行客户端渲染诊断，不调用账号/RPC/WS或服务端切图。');process.exit(0);}
const out=path.resolve(root,option('--report-dir','.work/source-town-browser-20261029')),baseUrl=option('--base-url','http://127.0.0.1:18080/');
const url=new URL(baseUrl);assert(['127.0.0.1','localhost'].includes(url.hostname)&&url.protocol==='http:','诊断只允许本机HTTP构建');
assert(out.startsWith(root+path.sep),'报告目录须在项目内');fs.mkdirSync(out,{recursive:true});
const build=JSON.parse(fs.readFileSync(path.join(root,'docs/客户端构建检查.json'),'utf8'));
assert(build.success&&build.scene_prefab_layout_preserved&&build.source_stable_during_build,'须有当前成功且源码稳定的Creator实构');
const sha=bytes=>crypto.createHash('sha256').update(bytes).digest('hex');
for(const [file,expected]of Object.entries({...build.asset_hashes,...build.script_hashes}))assert.equal(sha(fs.readFileSync(path.join(root,file))),expected,'构建后源/保存布局已变：'+file);
const sourceBase=path.join(root,'client/assets/resources/source-maps/map-2'),manifestBytes=fs.readFileSync(path.join(sourceBase,'manifest.json')),manifest=JSON.parse(manifestBytes),bits=Buffer.from(manifest.collision.runtime_bits,'base64');
assert.equal(manifest.id,'2');assert.deepEqual(manifest.minimap,{package:'mmap',raw_frame:100,frame:'mmap:100'});
const walkable=(x,y)=>x>=0&&y>=0&&x<700&&y<700&&!(bits[Math.floor((y*700+x)/8)]&(1<<((y*700+x)%8)));
function nearest(x,y,radius=4){x=Math.max(0,Math.min(699,x));y=Math.max(0,Math.min(699,y));for(let r=0;r<=radius;r++)for(let dy=-r;dy<=r;dy++)for(let dx=-r;dx<=r;dx++)if(Math.max(Math.abs(dx),Math.abs(dy))===r&&walkable(x+dx,y+dy))return{x:x+dx,y:y+dy};return{x,y};}
const torches=[],tall=[];
for(const info of manifest.chunks){const chunk=JSON.parse(fs.readFileSync(path.join(sourceBase,info.path),'utf8'));for(const draw of chunk.objects){const frame=manifest.frames[draw[3]];if(draw[4])torches.push({owner:info.id,draw});if(frame.height===987)tall.push({owner:info.id,draw});}}
assert.equal(torches.length,45);assert(tall.length);
torches.sort((a,b)=>Math.abs(a.draw[0]-331)+Math.abs(a.draw[1]-267)-Math.abs(b.draw[0]-331)-Math.abs(b.draw[1]-267));
const fire=torches[0],high=tall.find(value=>value.draw[1]>=25)??tall[0];
const plans=[
 {id:'01-silver',label:'银杏新人村安全区中心',x:650,y:631},
 {id:'02-boundary',label:'边界村安全区中心',x:289,y:618},
 {id:'03-bich',label:'比奇城安全区中心',x:331,y:267},
 {id:'04-torch',label:'来源加色火把',...nearest(fire.draw[0],fire.draw[1]),torch:{owner:fire.owner,x:fire.draw[0],y:fire.draw[1]}},
 {id:'05-high-front',label:'987像素高前景跨owner块',...nearest(high.draw[0],high.draw[1]-24,2),high:{owner:high.owner,x:high.draw[0],y:high.draw[1]}},
 {id:'06-top-left',label:'全图左上边缘',...nearest(0,0,16)},
 {id:'07-top-right',label:'全图右上边缘',...nearest(699,0,16)},
 {id:'08-bottom-right',label:'全图右下边缘',...nearest(699,699,16)},
 {id:'09-bottom-left',label:'全图左下边缘',...nearest(0,699,16)},
 {id:'10-lru-north',label:'远距LRU北部观察点',...nearest(350,50)},
 {id:'11-lru-west',label:'远距LRU西部观察点',...nearest(100,350)},
 {id:'12-lru-east',label:'远距LRU东部观察点',...nearest(600,350)},
 {id:'13-silver-return',label:'远距视口往返与LRU',x:650,y:631},
];
const report={passed:false,scope:'实际Creator/Chrome客户端来源地图渲染诊断；实体/坐标为仅内存展示输入，未做服务端城镇切换、绑定、瞬移、NPC或战斗验收',
 build:{creator:build.creator,seconds:build.seconds,sourceStable:build.source_stable_during_build,layoutsPreserved:build.scene_prefab_layout_preserved},
 sourceManifestSha256:sha(manifestBytes),physicalMobileVerified:false,serverTownTransitionVerified:false,checks:[],cases:[],errors:[],warnings:[],networkAttempts:[],frameReferenceViolations:[]};
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:!args.includes('--headed'),args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
 const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1}),page=await context.newPage();
 async function bounded(promise,label){let timer;try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('渲染诊断等待超时：'+label)),120000);})]);}finally{clearTimeout(timer);}}
 page.on('pageerror',error=>report.errors.push(error.stack||error.message));
 page.on('console',message=>{if(['warning','error'].includes(message.type()))report.warnings.push({type:message.type(),text:message.text().slice(0,1000)});});
 page.on('websocket',socket=>report.networkAttempts.push({kind:'websocket',url:socket.url()}));
 await page.route('**/account/rpc',route=>{report.networkAttempts.push({kind:'rpc',url:route.request().url()});return route.abort();});
 await page.route('**/game/ws',route=>{report.networkAttempts.push({kind:'ws-http',url:route.request().url()});return route.abort();});
 try{
  await page.goto(baseUrl);await page.waitForFunction(()=>window.cc?.director?.getScene()?.name==='Login',null,{timeout:60000});
  const prepared=await bounded(page.evaluate(async()=>{
   const system=window.System;if(!system?.import)throw Error('实际构建没有System.import');
   // window.cc只公开部分兼容命名空间；组件类型必须取与生产模块相同的cc模块。
   const cc=await system.import('cc');
   const [{WorldAssets},{GameWorldView}]=await Promise.all([system.import('chunks:///_virtual/WorldAssets.ts'),system.import('chunks:///_virtual/GameWorldView.ts')]);
   const bundle=cc.assetManager.getBundle('main');if(!bundle)throw Error('实际main bundle尚未加载');
   const asset=await new Promise((resolve,reject)=>bundle.loadScene('Game',(error,value)=>error?reject(error):resolve(value)));
   const gameScene=asset.scene,roots=gameScene.getComponentsInChildren('AppRoot');if(!roots.length)throw Error('保存Game场景缺少AppRoot定位');
   for(const app of roots)app.enabled=false;
   for(const component of gameScene.getComponentsInChildren('MapTouchNavigator'))component.enabled=false;
   for(const component of gameScene.getComponentsInChildren('TouchJoystick'))component.enabled=false;
   cc.director.runSceneImmediate(gameScene);await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
   const safe=cc.find('Canvas/Safe'),viewport=safe?.getChildByName('MapViewport'),world=viewport?.getChildByName('World'),entities=world?.getChildByName('Entities');
   if(!safe||!viewport||!world||!entities)throw Error('保存GameUI/world层级不完整');
   const hud=safe.getComponent('GameHUD');if(!hud)throw Error('保存Safe缺少GameHUD');
   const assets=await WorldAssets.prepare(2,()=>{},'2');if(!assets.sourceMap)throw Error('WorldAssets未选择来源类型');
   if(assets.sourceMap.model.manifest.width!==700||assets.sourceMap.model.manifest.minimap?.frame!=='mmap:100'||!assets.background)throw Error('实际构建缺少完整700地图及显式小地图');
   const view=new GameWorldView(viewport,world,entities,'渲染道士',2,'9000000000000001',1);view.prepare(assets,()=>{});hud.setMapFrame(assets.background,assets);
   const qa={scope:'source_render_diagnostic_only',cc,safe,viewport,world,entities,hud,assets,source:assets.sourceMap,view,sequence:0,baseline:null,running:true,errors:[],violations:[],instrumented:new Set(),visitedOwners:new Set(),lastPlan:null};
   const number=value=>({toString:()=>String(value),isZero:()=>value===0,lessThan:other=>BigInt(value)<BigInt(other.toString())});
   qa.state=(handle,kind,x,y)=>({entityHandle:handle,entityKind:kind,definitionId:kind===2?2:0,position:{x,y},direction:2,moveIntervalMs:kind===1?400:2000,
    attackSequence:0,castSequence:0,action:0,life:String(qa.sequence),movementRevision:'0',movementKind:0,hp:number(100),hpMax:number(100),mp:number(100),mpMax:number(100),statuses:[]});
   qa.instrument=()=>{for(const [key,entry]of Array.from(qa.source.frameEntries)){const frame=entry.asset;if(!frame||qa.instrumented.has(frame))continue;qa.instrumented.add(frame);const original=frame.decRef;
    frame.decRef=function(...args){if(this.refCount<=1){const bound=[];for(const visual of Array.from(qa.view.sourceView?.visuals?.values()??[]))if(visual.sprite.spriteFrame===this&&cc.isValid(visual.node,true))bound.push(visual.node.name);
     const mini=qa.safe.getChildByPath('TopHUD/MiniMap/Map/Surface')?.getComponent(cc.Sprite);if(mini?.spriteFrame===this&&cc.isValid(mini,true))bound.push('HUD/MiniMap');if(bound.length)qa.violations.push({key,bound,before:this.refCount});}return original.apply(this,args);};}};
   qa.refresh=()=>{if(!qa.baseline)return;qa.hud.refresh({playerHandle:1,role:{name:'渲染道士',job:2,level:1},wallet:{gold:'0',boundGold:'0',yuanbaoUnits:'0',boundYuanbaoUnits:'0'},inventory:{level:1,experience:'0',items:[]},baseline:qa.baseline,ready:false,auto:false,count:0,gameplay:qa.assets.gameplay,map:qa.assets.map});
    const text=qa.safe.getChildByPath('MapViewport/Mission/Text')?.getComponent(cc.Label);if(text)text.string='来源渲染诊断 · 无服务端切图';};
   qa.focus=async plan=>{qa.sequence++;qa.lastPlan=plan;qa.baseline={stateSequence:String(qa.sequence),entities:[qa.state(1,1,plan.x,plan.y),qa.state(2,2,Math.min(699,plan.x+1),Math.min(699,plan.y+1))]};qa.view.apply(qa.baseline);
    await qa.view.prepareViewport();if(!qa.view.isMapReady)throw Error('prepareViewport返回后当前地图仍未就绪');qa.refresh();qa.instrument();
    await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));};
   const tick=()=>{if(!qa.running)return;try{qa.view.update(0,2,false);qa.refresh();qa.instrument();}catch(error){qa.errors.push(error.stack||String(error));qa.running=false;return;}qa.raf=requestAnimationFrame(tick);};qa.raf=requestAnimationFrame(tick);
   window.__minimirSourceRenderQA=qa;
   return{scene:cc.director.getScene().name,disabledAppRoots:roots.length,mapId:assets.mapId,width:assets.map.width,height:assets.map.height,minimap:assets.sourceMap.model.manifest.minimap,
    minimapOriginal:{width:assets.background.originalSize.width,height:assets.background.originalSize.height},integrity:assets.sourceMap.integrity,cache:qa.source.stats};
  }),'保存场景与来源资源准备');
  assert.equal(prepared.mapId,'2');assert.deepEqual(prepared.minimapOriginal,{width:1052,height:699});report.prepared=prepared;report.checks.push('真实构建System.import与保存Game.scene；AppRoot/Input组件禁用；完整来源地图及真实小地图准备');
  async function snapshot(){return page.evaluate(()=>{
   const q=window.__minimirSourceRenderQA,c=q.cc,w=q.view,s=w.sourceView;w.update(0,2,false);q.refresh();q.instrument();
   const visuals=Array.from(s.visuals.values()),actors=Array.from(w.visuals.values()),cache=q.source.chunkCache;
   for(const hold of Array.from(s.chunks.values()))q.visitedOwners.add(hold.id);
   const badNative=visuals.filter(v=>v.node.scale.x!==1||v.node.scale.y!==1||v.sprite.trim!==false||v.transform.width!==v.draw.frame.width||v.transform.height!==v.draw.frame.height||v.transform.anchorPoint.x!==0||v.transform.anchorPoint.y!==1||v.node.position.x!==v.draw.bounds[0]||v.node.position.y!==-v.draw.bounds[1]).map(v=>v.id);
   const renderClock=s.nowMs;
   const torch=visuals.filter(v=>v.draw.blend===1).map(v=>{const target=v.sprite.getRenderMaterial(0)?.passes?.[0]?.blendState.targets[0];return{id:v.id,owner:v.owner,x:v.draw.x,y:v.draw.y,key:v.key,renderClock,expected:'Objects:'+(2723+Math.floor(renderClock/100)%10),blendSrc:target?.blendSrc,blendDst:target?.blendDst,width:v.transform.width,height:v.transform.height};});
   const expected=visuals.filter(v=>v.draw.layer>=3).map(v=>({node:v.node,row:v.draw.y,kind:0,x:v.draw.x,layer:v.draw.layer,id:v.id}));
   for(const a of actors)expected.push({node:a.node,row:a.state.position.y,kind:1,x:a.motion.sample(c.game.totalTime).x,layer:0,id:'entity_'+a.state.entityHandle});
   expected.sort((a,b)=>a.row-b.row||a.kind-b.kind||a.x-b.x||a.layer-b.layer||a.id.localeCompare(b.id));const actual=q.entities.children.filter(node=>expected.some(v=>v.node===node));
   const surface=q.safe.getChildByPath('TopHUD/MiniMap/Map/Surface'),mini=surface.getComponent(c.Sprite),miniUI=surface.getComponent(c.UITransform),dot=q.safe.getChildByPath('TopHUD/MiniMap/Map/PlayerDot');
   const names=actors.map(a=>{const n=a.node.getChildByName('Name'),label=n.getComponent(c.Label),hp=a.node.getChildByName('HP');return{handle:a.state.entityHandle,name:label.string,fontSize:label.fontSize,scaleX:n.scale.x,scaleY:n.scale.y,nameY:n.position.y,hpY:hp.position.y,position:{x:a.node.position.x,y:a.node.position.y},direction:w.projection.visualDirection(a.state.direction)};});
   const viewUI=q.viewport.getComponent(c.UITransform),box=viewUI.getBoundingBoxToWorld(),size=c.view.getVisibleSize(),canvas=document.querySelector('canvas').getBoundingClientRect();
   return{ready:w.isMapReady,view:s.stats,assets:q.source.stats,viewport:s.viewport,cachedOwners:Array.from(cache.entries.keys()),visitedOwners:Array.from(q.visitedOwners),badNative,torch,names,
    orderCorrect:actual.every((node,index)=>node===expected[index].node),minimap:{enabled:mini.enabled,nativeWidth:mini.spriteFrame?.originalSize.width,nativeHeight:mini.spriteFrame?.originalSize.height,width:miniUI.width,height:miniUI.height,dot:{x:dot.position.x,y:dot.position.y,active:dot.active}},
    fixedUI:{top:q.safe.getChildByName('TopHUD').getComponent(c.UITransform).height,bottom:q.safe.getChildByName('BottomHUD').getComponent(c.UITransform).height,mapHeight:viewUI.height},
    clip:{x:canvas.x+box.x/size.width*canvas.width,y:canvas.y+(size.height-box.y-box.height)/size.height*canvas.height,width:box.width/size.width*canvas.width,height:box.height/size.height*canvas.height},
    violations:q.violations.slice(),errors:q.errors.slice()};
  });}
  for(const plan of plans){
   await bounded(page.evaluate(plan=>window.__minimirSourceRenderQA.focus(plan),plan),'视口 '+plan.id);const data=await snapshot();
   const record={plan,...data,caseChecksPassed:false};report.cases.push(record);
   await page.screenshot({path:path.join(out,plan.id+'.png')});
   assert(data.ready&&data.view.visibleNodes>0,'实际视口无可视节点');assert.equal(data.badNative.length,0,'原生帧尺寸/锚点/偏移变化');assert(data.orderCorrect,'来源逐行物件与实体排序不符');
   assert(data.assets.chunks.entries<=49&&data.assets.requests.active<=4,'显式缓存/并发预算越界');assert(data.names.every(n=>n.fontSize===24&&Math.abs(n.scaleX-.7)<1e-5&&Math.abs(n.scaleY-.7)<1e-5&&n.nameY>n.hpY),'实体名字/血条保存比例或头顶顺序不符');
   assert(data.minimap.enabled&&data.minimap.nativeWidth===1052&&data.minimap.nativeHeight===699,'HUD未使用真实来源小地图');assert(Math.abs(data.minimap.width/data.minimap.height-1052/699)<1e-6,'小地图被非等比拉伸');
   assert.equal(data.violations.length,0,'资源引用归零时Sprite尚未解绑');assert.equal(data.errors.length,0,data.errors.join('\n'));
   if(plan.torch){const id=`${plan.torch.owner}/${plan.torch.x}/${plan.torch.y}/4`,entry=data.torch.find(v=>v.id===id);assert(entry,'目标火把未进入真实视口');assert.equal(entry.blendSrc,2);assert.equal(entry.blendDst,1);assert.equal(entry.width,100);assert.equal(entry.height,100);}
   if(plan.high){const observed=await page.evaluate(plan=>{const s=window.__minimirSourceRenderQA.view.sourceView,id=`${plan.high.owner}/${plan.high.x}/${plan.high.y}/4`,v=s.visuals.get(id);return{visible:!!v,ownerBelow:plan.high.y*32>=s.viewport[3],height:v?.draw.frame.height};},plan);assert(observed.visible&&observed.ownerBelow&&observed.height===987,'高前景未从视口外owner块正确入场');record.high=observed;}
   const clip=data.clip,screen=page.viewportSize(),x=Math.max(0,clip.x),y=Math.max(0,clip.y),width=Math.min(screen.width,x+clip.width)-x,height=Math.min(screen.height,y+clip.height)-y;
   if(width>0&&height>0)await page.screenshot({path:path.join(out,plan.id+'-map.png'),clip:{x,y,width,height}});
   record.caseChecksPassed=true;console.log(JSON.stringify({case:plan.id,ready:data.ready,nodes:data.view.visibleNodes,chunks:data.view.chunks,cached:data.assets.chunks.entries,sharedFrames:data.assets.sharedFrames}));
   if(plan.torch){const samples=[];for(let i=0;i<12;i++){await page.waitForTimeout(105);const sample=await snapshot();const torch=sample.torch.find(v=>v.owner===plan.torch.owner&&v.x===plan.torch.x&&v.y===plan.torch.y);assert(torch);assert.equal(torch.key,torch.expected);samples.push(torch.key);}assert(new Set(samples).size>=3,'火把时钟未推进');report.torchSamples=samples;}
  }
  const before=report.cases.at(-1);await page.setViewportSize({width:720,height:1440});await page.waitForTimeout(160);await bounded(page.evaluate(async()=>{const q=window.__minimirSourceRenderQA;q.view.refitCamera();await q.view.prepareViewport();}),'加高地图视口');const taller=await snapshot();
  assert.equal(taller.fixedUI.top,before.fixedUI.top);assert.equal(taller.fixedUI.bottom,before.fixedUI.bottom);assert.equal(taller.fixedUI.mapHeight-before.fixedUI.mapHeight,160);assert.equal(taller.badNative.length,0);await page.screenshot({path:path.join(out,'14-taller-silver.png')});report.taller=taller;
  const lru=await page.evaluate(()=>{const q=window.__minimirSourceRenderQA,entries=Array.from(q.source.chunkCache.entries.keys()),visited=Array.from(q.visitedOwners);return{visited:visited.length,cache:entries.length,evicted:visited.filter(id=>!entries.includes(id)).length};});assert(lru.visited>49&&lru.cache<=49&&lru.evicted>0,'远距视口诊断未覆盖真实LRU淘汰');report.lru=lru;
  const disposal=await page.evaluate(async()=>{const q=window.__minimirSourceRenderQA;q.running=false;cancelAnimationFrame(q.raf);const s=q.view.sourceView;q.view.dispose();await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));return{view:s.stats,assets:q.source.stats,violations:q.violations,errors:q.errors,minimapBound:!!q.safe.getChildByPath('TopHUD/MiniMap/Map/Surface').getComponent(q.cc.Sprite).spriteFrame};});
  assert.equal(disposal.view.visibleNodes,0);assert(disposal.view.disposed&&disposal.assets.disposed);assert.equal(disposal.assets.retainedFrames,0);assert.equal(disposal.assets.requests.active,0);assert.equal(disposal.minimapBound,false);assert.equal(disposal.violations.length,0);report.disposal=disposal;
  assert.equal(report.networkAttempts.length,0,'纯渲染诊断触发了业务网络');assert.equal(report.errors.length,0,report.errors.join('\n'));report.frameReferenceViolations=disposal.violations;
  report.checks.push('银杏/边界/比奇真实原格画面与保存HUD','100ms原帧火把及SRC_ALPHA/ONE加色','987px高前景来自视口外owner块','四角边缘、远距往返与49块LRU淘汰','Label24/保存0.7比例及原生逐行遮挡','真实小地图1052×699等比显示','加高160像素仅增地图视口','销毁前Sprite/材质解绑及资源引用归零','RPC/WS请求为0');report.passed=true;
 }catch(error){report.error=error.stack||String(error);try{await page.screenshot({path:path.join(out,'failure.png')});const state=await page.evaluate(()=>{const q=window.__minimirSourceRenderQA;return q?{errors:q.errors,violations:q.violations,view:q.view.sourceView?.stats,assets:q.source.stats}:null;});report.failureState=state;}catch{}throw error;}
 finally{fs.writeFileSync(path.join(out,'report.json'),JSON.stringify(report,null,2)+'\n','utf8');await browser.close();}
 console.log(JSON.stringify({passed:report.passed,cases:report.cases.length,checks:report.checks.length,reportDir:out,warnings:report.warnings.length}));
})().catch(error=>{console.error(error.stack||error.message);process.exitCode=1;});
