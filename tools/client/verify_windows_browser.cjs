const assert=require('node:assert/strict');
const crypto=require('node:crypto');
const fs=require('node:fs');
const path=require('node:path');
const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
const pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const out='outputs/windows-adaptation-20261028/browser';fs.mkdirSync(out,{recursive:true});
// 仅使用正常注册、创角及触摸入口；协议观察只记录导航元数据，不保存凭据或票据。
const build=JSON.parse(fs.readFileSync('docs/客户端构建检查.json','utf8'));
assert(build.success&&build.scene_prefab_layout_preserved&&build.source_stable_during_build,'需要通过当前Creator实构');
for(const [file,expected]of Object.entries({...build.asset_hashes,...build.script_hashes})){
 assert.equal(crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex'),expected,'构建后源码或保存布局有变化：'+file);
}
function packet(payload){const b=Buffer.from(payload);if(b.length<6)return null;let offset=4;function variable(){let n=0n,s=0n;for(let i=0;i<10;i++){const x=b[offset++];n|=BigInt(x&127)<<s;if(!(x&128))return n;s+=7n}throw Error('invalid header')}const seq=Number(variable()),binding=variable().toString(),error=b[1]&1?Number(variable()):0;return{id:b.readUInt16BE(2),seq,binding,error,body:b.subarray(offset)}}
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
 const errors=[],warnings=[],traces=[],checks=[];
 const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1});
 const page=await context.newPage();
 page.on('pageerror',e=>errors.push(e.stack||e.message));
 page.on('console',m=>{if(m.type()==='warning'||m.type()==='error')warnings.push({type:m.type(),text:m.text().slice(0,700)})});
 page.on('websocket',socket=>{
  const inspect=(direction,event)=>{try{const p=packet(event.payload);if(!p||p.error)return;
   if(direction==='send'&&p.id===20006){const v=pb.MoveIntent.decode(p.body);traces.push({direction,id:p.id,seq:p.seq,input:v.input_sequence,cancel:!!v.cancel_navigation,release:!!v.release_joystick,destination:v.destination&&{x:v.destination.x,y:v.destination.y}})}
   if(direction==='receive'&&p.id===5){const v=pb.SceneBaselineChunk.decode(p.body),n=v.navigation_state;traces.push({direction,id:5,seq:p.seq,scene:v.scene_epoch.toString(),navigation:n&&{revision:n.revision.toString(),input:n.input_sequence,status:n.status,destination:n.destination&&{x:n.destination.x,y:n.destination.y}}})}
  }catch(e){errors.push('QA协议观察:'+e.message)}};
  socket.on('framesent',event=>inspect('send',event));socket.on('framereceived',event=>inspect('receive',event));
 });
 async function scene(name){await page.waitForFunction(name=>window.cc?.director?.getScene()?.name===name,name,{timeout:60000})}
 async function tap(suffix){const point=await page.evaluate(suffix=>{const node=cc.find('Canvas/Safe/'+suffix);if(!node||!node.activeInHierarchy)throw Error('节点不可交互:'+suffix);const box=node._uiProps.uiTransformComp.getBoundingBoxToWorld(),visible=cc.view.getVisibleSize(),rect=document.querySelector('canvas').getBoundingClientRect();return{x:rect.x+(box.x+box.width/2)/visible.width*rect.width,y:rect.y+(visible.height-box.y-box.height/2)/visible.height*rect.height}},suffix);await page.touchscreen.tap(point.x,point.y)}
 async function edit(suffix,value){await tap(suffix);const input=page.locator('input:visible');await input.waitFor();await input.fill(value)}
 async function nav(){return page.evaluate(()=>{const app=cc.director.getScene().getComponentsInChildren('AppRoot')[0];return app?.nav?.state})}
 async function ground(x,y){const point=await page.evaluate(({x,y})=>{const app=cc.director.getScene().getComponentsInChildren('AppRoot')[0],w=app.world,local=w.projection.project(x,y),p=w.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(local.x,local.y)),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+p.x/v.width*r.width,y:r.y+(v.height-p.y)/v.height*r.height}}, {x,y});assert(point.x>=0&&point.x<=page.viewportSize().width&&point.y>=128&&point.y<=page.viewportSize().height-344,'目标地面不在可见战斗区');await page.touchscreen.tap(point.x,point.y);await page.waitForFunction(({x,y})=>{const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],t=a?.nav?.state?.target;return t?.x===x&&t?.y===y},{x,y},{timeout:10000})}
 try{
  for(const port of [18100,18101,18102]){
   const response=await fetch(`http://127.0.0.1:${port}/healthz`,{signal:AbortSignal.timeout(3000)}),health=await response.json();
   assert(response.ok&&health.status==='ok'&&health.development_only&&health.rule_version===10,`开发节点${port}未就绪或不是当前rule10`);
  }
  await page.goto('http://127.0.0.1:18080/?renderQA=1');await scene('Login');
  await page.screenshot({path:path.join(out,'01-login.png')});
  const account='wqa'+crypto.randomBytes(5).toString('hex'),password=crypto.randomBytes(9).toString('hex');
  await edit('Account',account);await edit('Password',password);await tap('Register');await scene('RoleSelect');
  await edit('Name','Win'+crypto.randomBytes(3).toString('hex'));await tap('Create');
  await page.waitForFunction(()=>cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(x=>x.string.includes('Win')),{timeout:20000});
  await tap('Enter');await scene('Game');await page.waitForFunction(()=>!!cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.reconciliationStarted,null,{timeout:30000});
  await page.screenshot({path:path.join(out,'02-game.png')});checks.push('通过正常注册和创建角色进入rule10；没有数据库预置');
  const auto=await page.evaluate(()=>cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked);
  if(auto)await tap('MapViewport/Actions/Auto');
  await ground(1,10);const initial=await nav();
  await tap('BottomHUD/Footer/Inventory');assert.deepEqual((await nav()).target,initial.target);checks.push('打开背包保留地面导航目标');
  await page.screenshot({path:path.join(out,'03-navigation-panel.png')});
  await page.touchscreen.tap(10,200);
  const before=await page.evaluate(()=>{const m=cc.find('Canvas/Safe/MapViewport'),top=cc.find('Canvas/Safe/TopHUD'),bottom=cc.find('Canvas/Safe/BottomHUD');return{mapHeight:m._uiProps.uiTransformComp.height,topHeight:top._uiProps.uiTransformComp.height,bottomHeight:bottom._uiProps.uiTransformComp.height}});
  await page.setViewportSize({width:720,height:1440});await page.waitForTimeout(120);
  const after=await page.evaluate(()=>{const m=cc.find('Canvas/Safe/MapViewport'),top=cc.find('Canvas/Safe/TopHUD'),bottom=cc.find('Canvas/Safe/BottomHUD');return{mapHeight:m._uiProps.uiTransformComp.height,topHeight:top._uiProps.uiTransformComp.height,bottomHeight:bottom._uiProps.uiTransformComp.height}});
  assert.equal(after.mapHeight-before.mapHeight,160);assert.equal(after.topHeight,before.topHeight);assert.equal(after.bottomHeight,before.bottomHeight);checks.push('同宽加高160像素只增加地图视口，顶底HUD高度保持');
  await page.screenshot({path:path.join(out,'04-taller-viewport.png')});
  await page.waitForFunction(()=>!cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.nav?.state?.target,{timeout:15000});checks.push('导航由服务端到达状态正常结束');
  await ground(8,8);await tap('BottomHUD/Footer/ChangeRole');await scene('RoleSelect');
  const cancels=traces.filter(x=>x.direction==='send'&&x.cancel);assert.equal(cancels.length,1);
  assert(traces.some(x=>x.direction==='receive'&&x.seq===cancels[0].seq&&x.navigation?.status===4));checks.push('换角发送一次显式取消，收到对应请求基线的status4确认后退出');
  await page.waitForFunction(()=>cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(x=>x.string.includes('Win')),null,{timeout:30000});
  await tap('Enter');await scene('Game');await page.waitForFunction(()=>!!cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.reconciliationStarted,null,{timeout:30000});
  assert.equal((await nav()).target,null);checks.push('健康重入恢复取消状态，没有重新提交旧地面目标');
  await page.screenshot({path:path.join(out,'05-healthy-reentry.png')});
  assert.equal(errors.length,0,errors.join('\n'));
  fs.writeFileSync(path.join(out,'report.json'),JSON.stringify({passed:true,checks,errors,warnings,viewportCases:[[720,1280],[720,1440]],touchInput:true,physicalMobileVerified:false,forcedMovementSkillsOpened:false,scope:'正常账号/角色、导航、面板、加高和退出确认；不作为完整技能、真机或容量验收'},null,2));
  console.log(JSON.stringify({passed:true,checks:checks.length,errors:errors.length,warnings:warnings.length}));
 }catch(error){fs.writeFileSync(path.join(out,'report.json'),JSON.stringify({passed:false,checks,errors,warnings,error:error.message},null,2));await page.screenshot({path:path.join(out,'failure.png')});throw error}
 finally{fs.writeFileSync(path.join(out,'navigation-trace.json'),JSON.stringify(traces,null,2));await browser.close()}
})().catch(error=>{console.error(error.message);process.exitCode=1});
