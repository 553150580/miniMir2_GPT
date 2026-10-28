// 真实保存UI、正常道士、触屏到访安全区与永久绑定观察；不预置数据库或改写权威位置，21仍关闭。
const assert=require('node:assert/strict'),crypto=require('node:crypto'),fs=require('node:fs'),path=require('node:path');
const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
const root=path.resolve(__dirname,'../..'),flag=process.argv.indexOf('--report-dir'),out=path.resolve(root,flag<0?'.work/safe-zone-binding-20261029/browser-town-ground-fixed':process.argv[flag+1]);
assert(out.startsWith(root+path.sep));fs.mkdirSync(out,{recursive:true});
const build=JSON.parse(fs.readFileSync(path.join(root,'docs/客户端构建检查.json'),'utf8'));
assert(build.success&&build.scene_prefab_layout_preserved&&build.source_stable_during_build,'需要当前成功Creator实构');
for(const [file,expected]of Object.entries({...build.asset_hashes,...build.script_hashes}))assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(root,file))).digest('hex'),expected,'构建后源码或保存布局已变化：'+file);
const pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const configuredRuleVersion=JSON.parse(fs.readFileSync(path.join(root,'config/development/first-loop.json'),'utf8')).rule_version;
assert(typeof configuredRuleVersion==='string'?/^[1-9]\d*$/.test(configuredRuleVersion):Number.isSafeInteger(configuredRuleVersion)&&configuredRuleVersion>0,'配置规则版本必须保持整数精度');
const expectedRuleVersion=String(configuredRuleVersion);
const manifest=JSON.parse(fs.readFileSync(path.join(root,'client/assets/resources/source-maps/map-2/manifest.json'),'utf8')),bits=Buffer.from(manifest.collision.runtime_bits,'base64');
const walkable=(x,y)=>x>=0&&y>=0&&x<700&&y<700&&!(bits[(y*700+x)>>3]&(1<<((y*700+x)&7)));
function nearby(x,y){for(let r=0;r<=2;r++)for(let dy=-r;dy<=r;dy++)for(let dx=-r;dx<=r;dx++)if(Math.max(Math.abs(dx),Math.abs(dy))===r&&walkable(x+dx,y+dy))return{x:x+dx,y:y+dy};throw Error('观察路径没有来源可走落点');}
function clearGroundSample(sample,zone){return Boolean(sample.walkable&&sample.withinViewport&&sample.ready&&!sample.overlay&&!sample.entityHits.length&&!sample.occupiedBy.length&&(zone===undefined||sample.zone===zone));}
function selectSameCenterGround(samples,zone){const found=samples.find(sample=>clearGroundSample(sample,zone));assert(found,'同区候选没有真实可轻触地面：'+JSON.stringify(samples));return found.point;}
function packet(value){const b=Buffer.from(value);if(b.length<6)return null;let offset=4;const variable=()=>{let n=0n,s=0n;for(let i=0;i<10;i++){const x=b[offset++];n|=BigInt(x&127)<<s;if(!(x&128))return n;s+=7n;}throw Error('invalid header');};const request=Number(variable()),binding=variable().toString(),error=b[1]&1?Number(variable()):0;return{id:b.readUInt16BE(2),request,binding,error,body:b.subarray(offset)};}
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
 const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1}),page=await context.newPage();
 const report={passed:false,scope:'真实Creator、正常WS道士触屏安全区绑定提交显示与健康重入；不预置角色数值或改权威坐标；21继续关闭，冷重建由独立WS验收，不作真机/容量认证',expectedRuleVersion,readyRuleChecks:[],checks:[],errors:[],warnings:[],entries:[],travel:[],moves:[],groundChecks:[],bindingStates:[],clientBindingPackets:0,physicalMobileVerified:false,bindingVerified:false,coldRebuildVerified:false,teleport21Verified:false};
 page.on('pageerror',e=>report.errors.push(e.stack||e.message));page.on('console',m=>{if(['warning','error'].includes(m.type()))report.warnings.push({type:m.type(),text:m.text().slice(0,700)});});
 page.on('websocket',socket=>{
  socket.on('framesent',event=>{try{if(packet(event.payload)?.id===20011)report.clientBindingPackets++;}catch(e){report.errors.push('QA发送观察：'+e.message);}});
  socket.on('framereceived',event=>{try{
   const p=packet(event.payload);if(!p||p.error)return;
   if(p.id===20002){const q=pb.EnterLineResult.decode(p.body);report.entries.push({scene:q.scene_epoch.toString(),baseline:q.baseline_id.toString(),map:q.room_id.toString(),kind:q.scene_kind,player:q.player_handle,x:q.position.x,y:q.position.y,resourceVersion:q.resource_version.toString()});assert.equal(q.resource_version.toString(),expectedRuleVersion,'真实EnterLine规则版本与当前配置不一致');}
   if(p.id===20010){const q=pb.MapTravelState.decode(p.body);report.travel.push({id:q.travel_id,from:q.from_scene_epoch.toString(),scene:q.scene_epoch.toString(),phase:q.phase,map:q.target_map_id.toString()});}
   if(p.id===20011){const q=pb.BindingState.decode(p.body);report.bindingStates.push({transportBinding:p.binding,role:q.role.origin_server_id.toString()+':'+Buffer.from(q.role.role_id).toString('hex'),scene:q.scene_epoch.toString(),revision:q.role_revision.toString(),sourceSeq:q.source_seq.toString(),binding:q.binding?{map:q.binding.map_id.toString(),center:{x:q.binding.center.x,y:q.binding.center.y}}:null,cause:q.cause,safeZoneId:q.safe_zone_id.toString(),trusted:q.trusted});}
  }catch(e){report.errors.push('QA协议观察：'+e.message);}});
 });
 async function scene(name){await page.waitForFunction(name=>window.cc?.director?.getScene()?.name===name,name,{timeout:60000});}
 async function tap(suffix){const p=await page.evaluate(suffix=>{const n=cc.find('Canvas/Safe/'+suffix);if(!n||!n.activeInHierarchy)throw Error('节点不可交互：'+suffix);const b=n._uiProps.uiTransformComp.getBoundingBoxToWorld(),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+(b.x+b.width/2)/v.width*r.width,y:r.y+(v.height-b.y-b.height/2)/v.height*r.height};},suffix);await page.touchscreen.tap(p.x,p.y);}
 async function edit(suffix,value){await tap(suffix);const e=page.locator('input:visible');await e.waitFor();await e.fill(value);}
 async function ready(map){
  await page.waitForFunction(map=>{const a=cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];return a?.attached?.map?.toString()===map&&a.world?.map?.id?.toString()===map&&a.reconciliationStarted&&a.world.isMapReady&&a.canSendInput();},map,{timeout:120000});
  const resourceVersion=await page.evaluate(async()=>{const {session}=await System.import('chunks:///_virtual/GameSession.ts');return session.entry.resourceVersion.toString();});
  report.readyRuleChecks.push({map,resourceVersion});assert.equal(resourceVersion,expectedRuleVersion,'真实就绪场景规则版本与当前配置不一致');
 }
 async function observedBinding(trusted=false){
  // 本机Playwright先判predicate返回值真伪，Promise对象会提前结束轮询；先取得只读引用，再同步判Boolean。
  const observed=await page.evaluateHandle(async()=>{const {session}=await System.import('chunks:///_virtual/GameSession.ts');return session;});
  try{await page.waitForFunction(({observed,trusted})=>{const q=observed.bindingState;return Boolean(q&&(!trusted||(q.trusted&&q.binding?.mapId==='2'&&q.binding.center.x===650&&q.binding.center.y===631&&BigInt(q.sourceSeq)>0n&&observed.inventory&&BigInt(observed.inventory.revision.toString())>=BigInt(q.roleRevision))));},{observed,trusted},{timeout:30000});}
  finally{await observed.dispose();}
 }
 async function state(){return page.evaluate(async()=>{const {session}=await System.import('chunks:///_virtual/GameSession.ts'),a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],player=session.baseline.entities.find(e=>e.entityHandle===session.entry.playerHandle),pet=session.baseline.entities.find(e=>e.entityKind===2);return{map:session.entry.roomId.toString(),scene:session.entry.sceneEpoch.toString(),baseline:session.entry.baselineId.toString(),resourceVersion:session.entry.resourceVersion.toString(),binding:session.connection.binding.toString(),role:session.role.name,ready:session.ready,travel:session.travel.phase,position:{x:player.position.x,y:player.position.y},life:player.life.toString(),hp:player.hp.toString(),mp:player.mp.toString(),pet:pet&&{life:pet.life.toString(),hp:pet.hp.toString(),mp:pet.mp.toString()},zone:session.baseline.safeZoneId.toString(),projection:a.world.map.projection,nav:a.nav.state,render:a.world.isMapReady,roleBinding:session.bindingState,inventoryRevision:session.inventory?.revision?.toString(),economicCommand:session.command.toString(),roleText:cc.find('Canvas/Safe/UIMgr/Panels/Role/Message').getComponent(cc.Label).string,bindingMessages:a.messages.log.messages.filter(m=>m.title==='绑定已确认').map(m=>m.message)};});}
 async function snapshot(name){await page.screenshot({path:path.join(out,name+'.png')});return state();}
 async function travel(map){await tap('BottomHUD/Footer/Map');await page.waitForFunction(()=>cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy,null,{timeout:120000});await tap('PopupView/Panels/Confirm/ConfirmButton');await ready(map);}
 async function groundSample(point){
  return page.evaluate(({x,y})=>{
   const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],w=a.world,q=w.projection.project(x,y),s=w.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(q.x,q.y)),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect(),roundTrip=w.screenToGrid(s);
   const inside=(t,p,margin)=>p.x>=-t.anchorPoint.x*t.width-margin&&p.x<=((1-t.anchorPoint.x)*t.width)+margin&&p.y>=-t.anchorPoint.y*t.height-margin&&p.y<=((1-t.anchorPoint.y)*t.height)+margin;
   const viewport=w.viewport._uiProps.uiTransformComp,viewPoint=viewport.convertToNodeSpaceAR(s),entityHits=[],occupiedBy=[];
   for(const visual of Array.from(w.visuals.values())){
    if(!visual.node.activeInHierarchy)continue;const t=visual.node._uiProps.uiTransformComp,p=t.convertToNodeSpaceAR(s),state=visual.state;
    // 与引擎UITransform.hitTest相同的根局部矩形；留8像素余量，不能拿图片透明区当可点击地面。
    if(inside(t,p,8))entityHits.push({handle:state.entityHandle,kind:state.entityKind,local:{x:p.x,y:p.y},width:t.width,height:t.height});
    if(state.position.x===x&&state.position.y===y)occupiedBy.push(state.entityHandle);
   }
   const zone=(w.map.safe_zones??[]).find(z=>Math.max(Math.abs(x-z.center.x),Math.abs(y-z.center.y))<=z.radius);
   return{point:{x,y},x:r.x+s.x/v.width*r.width,y:r.y+(v.height-s.y)/v.height*r.height,world:{x:w.world.position.x,y:w.world.position.y},projected:{x:q.x,y:q.y},visible:{width:v.width,height:v.height},walkable:roundTrip?.x===x&&roundTrip?.y===y,withinViewport:inside(viewport,viewPoint,-12),ready:w.isMapReady&&a.canSendInput(),overlay:a.popup?.top?.name||a.ui?.top?.name||'',zone:zone?String(zone.id):'0',entityHits,occupiedBy};
  },point);
 }
 async function ground(point,requiredZone){
  const until=Date.now()+10000;let p;
  do{p=await groundSample(point);if(clearGroundSample(p,requiredZone))break;await new Promise(resolve=>setTimeout(resolve,100));}while(Date.now()<until);
  report.groundChecks.push(p);assert(clearGroundSample(p,requiredZone),'QA落点不是当前可轻触地面：'+JSON.stringify(p));
  await page.touchscreen.tap(p.x,p.y);
  await page.waitForFunction(({x,y})=>{const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],w=a?.world,p=w?.visuals.get(w.playerHandle)?.state,display=w?.playerPosition;return p?.position.x===x&&p?.position.y===y&&!a.nav.state.target&&display&&Math.abs(display.x-x)<.01&&Math.abs(display.y-y)<.01;},point,{timeout:25000});
  report.moves.push(point);
 }
 try{
  await page.goto('http://127.0.0.1:18080/?townTravelQA=1');await scene('Login');const account='tqa'+crypto.randomBytes(5).toString('hex'),password=crypto.randomBytes(9).toString('hex');
  await edit('Account',account);await edit('Password',password);await tap('Register');await scene('RoleSelect');await tap('JobTabs/Job2');await edit('Name','Town'+crypto.randomBytes(3).toString('hex'));await tap('Create');await page.waitForFunction(()=>cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l=>l.string.includes('Town')));await tap('Enter');await scene('Game');await ready('1');
  if(await page.evaluate(()=>cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked))await tap('MapViewport/Actions/Auto');
  await observedBinding();report.before=await snapshot('01-personal');assert(report.before.pet,'正常道士入场必须有现存宠物');assert.equal(report.before.roleBinding.binding,null);assert.equal(report.before.roleBinding.sourceSeq,'0');assert.equal(report.before.roleBinding.trusted,false);assert.match(report.before.roleText,/尚未绑定/);report.checks.push('正常注册道士与个人图入场，新角色已提交快照为nil绑定；无随机新生或数据库预置');
  await travel('2');report.town=await snapshot('02-town-entry');assert.equal(report.town.projection,'source_rectangular');assert.deepEqual(report.town.position,{x:633,y:613});assert.equal(report.town.zone,'0');assert.equal(report.town.life,report.before.life);assert.equal(report.town.pet.life,report.before.pet.life);report.checks.push('保存地图按钮及确认面板进入真实城镇；新scene/首视口/ACK就绪，角色和宠物Life保留');
  for(const point of [nearby(639,619),nearby(645,625),{x:650,y:631}])await ground(point);
  await observedBinding(true);report.safe=await snapshot('03-safe-zone');assert.equal(report.safe.zone,'106');assert.equal(report.safe.roleBinding.cause,2);assert.equal(report.safe.roleBinding.safeZoneId,'106');assert.equal(report.safe.economicCommand,report.town.economicCommand);assert.equal(report.safe.bindingMessages.length,1);assert.match(report.safe.bindingMessages[0],/650,631/);assert(report.bindingStates.some(q=>q.cause===2&&q.trusted&&q.sourceSeq===report.safe.roleBinding.sourceSeq));report.checks.push('真实触屏进入安全区106后等待独立可信receipt提交；只报一次绑定确认，不推进经济命令');
  await tap('BottomHUD/QuickNav/Role');await page.waitForFunction(()=>cc.find('Canvas/Safe/UIMgr/Panels/Role')?.activeInHierarchy);report.rolePanel=await snapshot('03a-role-binding');assert.match(report.rolePanel.roleText,/绑定：.*\(650,631\)/);assert(!report.rolePanel.roleText.includes('历史记录'));await page.touchscreen.tap(12,200);await page.waitForFunction(()=>!cc.find('Canvas/Safe/UIMgr/Panels/Role')?.activeInHierarchy);report.checks.push('保存Role面板原Message显示已确认地图中心，未新建或修改固定布局');
  const center={x:650,y:631},candidates=[{x:654,y:631},{x:646,y:631},{x:655,y:631},{x:645,y:631}].filter(q=>walkable(q.x,q.y));
  const samples=await Promise.all(candidates.map(groundSample)),side=selectSameCenterGround(samples,report.safe.zone);report.sameCenterRoute={candidates:samples,outbound:side,return:center};
  await ground(side,report.safe.zone);await ground(center,report.safe.zone);report.sameCenter=await state();assert.equal(report.sameCenter.roleBinding.sourceSeq,report.safe.roleBinding.sourceSeq);assert.equal(report.sameCenter.bindingMessages.length,1);report.checks.push('同一安全区横向4–5格真实地面往返不产生新receipt或重复提示，避开本人/宠物根命中矩形');
  await tap('BottomHUD/Footer/ChangeRole');await scene('RoleSelect');await tap('Enter');await scene('Game');await ready('2');await observedBinding(true);report.reentered=await snapshot('04-town-reentry');assert.deepEqual(report.reentered.position,report.safe.position);assert.equal(report.reentered.life,report.safe.life);assert.equal(report.reentered.hp,report.safe.hp);assert.equal(report.reentered.mp,report.safe.mp);assert.notEqual(report.reentered.binding,report.safe.binding);assert.equal(report.reentered.roleBinding.sourceSeq,report.safe.roleBinding.sourceSeq);assert.equal(report.reentered.roleBinding.cause,1);assert.equal(report.reentered.bindingMessages.length,0);assert.match(report.reentered.roleText,/绑定：.*\(650,631\)/);report.checks.push('正常换角/新票健康重入保留Town现场与原绑定receipt；快照恢复静默');
  await travel('1');await observedBinding(true);report.returned=await snapshot('05-personal-return');assert.equal(report.returned.projection,'isometric_diamond');assert.equal(report.returned.life,report.before.life);assert.equal(report.returned.roleBinding.binding.mapId,'2');assert.deepEqual(report.returned.roleBinding.binding.center,{x:650,y:631});assert.equal(report.returned.roleBinding.sourceSeq,report.safe.roleBinding.sourceSeq);assert.match(report.returned.roleText,/绑定：.*\(650,631\)/);assert.equal(report.returned.bindingMessages.length,0);report.checks.push('返回保留的个人战斗图仍观察原Town绑定中心，未触发21瞬移');
  assert.equal(report.clientBindingPackets,0,'绑定只有S2C观察，不上传绑定坐标');report.bindingVerified=true;
  assert.equal(report.errors.length,0,report.errors.join('\n'));assert.equal(report.entries.filter(e=>e.map==='2').length,2);assert(report.travel.filter(e=>e.phase===2).length>=2);report.passed=true;
 }catch(error){report.error=error.stack||String(error);try{await page.screenshot({path:path.join(out,'failure.png')});}catch{}throw error;}
 finally{fs.writeFileSync(path.join(out,'report.json'),JSON.stringify(report,null,2));await browser.close();}
 console.log(JSON.stringify({passed:report.passed,checks:report.checks.length,errors:report.errors.length,warnings:report.warnings.length}));
})().catch(error=>{console.error(error.message);process.exitCode=1;});
