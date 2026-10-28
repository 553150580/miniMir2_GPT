// 真实保存UI、正常账号与角色、同owner城镇往返；不预置数据库或改写权威位置。
const assert=require('node:assert/strict'),crypto=require('node:crypto'),fs=require('node:fs'),path=require('node:path');
const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
const root=path.resolve(__dirname,'../..'),flag=process.argv.indexOf('--report-dir'),out=path.resolve(root,flag<0?'.work/town-travel-20261029/browser-town':process.argv[flag+1]);
assert(out.startsWith(root+path.sep));fs.mkdirSync(out,{recursive:true});
const build=JSON.parse(fs.readFileSync(path.join(root,'docs/客户端构建检查.json'),'utf8'));
assert(build.success&&build.scene_prefab_layout_preserved&&build.source_stable_during_build,'需要当前成功Creator实构');
for(const [file,expected]of Object.entries({...build.asset_hashes,...build.script_hashes}))assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(root,file))).digest('hex'),expected,'构建后源码或保存布局已变化：'+file);
const pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const manifest=JSON.parse(fs.readFileSync(path.join(root,'client/assets/resources/source-maps/map-2/manifest.json'),'utf8')),bits=Buffer.from(manifest.collision.runtime_bits,'base64');
const walkable=(x,y)=>x>=0&&y>=0&&x<700&&y<700&&!(bits[(y*700+x)>>3]&(1<<((y*700+x)&7)));
function nearby(x,y){for(let r=0;r<=2;r++)for(let dy=-r;dy<=r;dy++)for(let dx=-r;dx<=r;dx++)if(Math.max(Math.abs(dx),Math.abs(dy))===r&&walkable(x+dx,y+dy))return{x:x+dx,y:y+dy};throw Error('观察路径没有来源可走落点');}
function packet(value){const b=Buffer.from(value);if(b.length<6)return null;let offset=4;const variable=()=>{let n=0n,s=0n;for(let i=0;i<10;i++){const x=b[offset++];n|=BigInt(x&127)<<s;if(!(x&128))return n;s+=7n;}throw Error('invalid header');};const request=Number(variable()),binding=variable().toString(),error=b[1]&1?Number(variable()):0;return{id:b.readUInt16BE(2),request,binding,error,body:b.subarray(offset)};}
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
 const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1}),page=await context.newPage();
 const report={passed:false,scope:'真实Creator及正常WS触屏城镇往返；不预置角色数值、不直接改地图/坐标，不作为绑定、瞬移、真机或容量验收',checks:[],errors:[],warnings:[],entries:[],travel:[],moves:[],physicalMobileVerified:false,bindingVerified:false,teleport21Verified:false};
 page.on('pageerror',e=>report.errors.push(e.stack||e.message));page.on('console',m=>{if(['warning','error'].includes(m.type()))report.warnings.push({type:m.type(),text:m.text().slice(0,700)});});
 page.on('websocket',socket=>socket.on('framereceived',event=>{try{const p=packet(event.payload);if(!p||p.error)return;if(p.id===20002){const q=pb.EnterLineResult.decode(p.body);report.entries.push({scene:q.scene_epoch.toString(),baseline:q.baseline_id.toString(),map:q.room_id.toString(),kind:q.scene_kind,player:q.player_handle,x:q.position.x,y:q.position.y});}if(p.id===20010){const q=pb.MapTravelState.decode(p.body);report.travel.push({id:q.travel_id,from:q.from_scene_epoch.toString(),scene:q.scene_epoch.toString(),phase:q.phase,map:q.target_map_id.toString()});}}catch(e){report.errors.push('QA协议观察：'+e.message);}}));
 async function scene(name){await page.waitForFunction(name=>window.cc?.director?.getScene()?.name===name,name,{timeout:60000});}
 async function tap(suffix){const p=await page.evaluate(suffix=>{const n=cc.find('Canvas/Safe/'+suffix);if(!n||!n.activeInHierarchy)throw Error('节点不可交互：'+suffix);const b=n._uiProps.uiTransformComp.getBoundingBoxToWorld(),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+(b.x+b.width/2)/v.width*r.width,y:r.y+(v.height-b.y-b.height/2)/v.height*r.height};},suffix);await page.touchscreen.tap(p.x,p.y);}
 async function edit(suffix,value){await tap(suffix);const e=page.locator('input:visible');await e.waitFor();await e.fill(value);}
 async function ready(map){await page.waitForFunction(map=>{const a=cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];return a?.attached?.map?.toString()===map&&a.world?.map?.id?.toString()===map&&a.reconciliationStarted&&a.world.isMapReady&&a.canSendInput();},map,{timeout:120000});}
 async function state(){return page.evaluate(async()=>{const {session}=await System.import('chunks:///_virtual/GameSession.ts'),a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],player=session.baseline.entities.find(e=>e.entityHandle===session.entry.playerHandle),pet=session.baseline.entities.find(e=>e.entityKind===2);return{map:session.entry.roomId.toString(),scene:session.entry.sceneEpoch.toString(),baseline:session.entry.baselineId.toString(),binding:session.connection.binding.toString(),role:session.role.name,ready:session.ready,travel:session.travel.phase,position:{x:player.position.x,y:player.position.y},life:player.life.toString(),hp:player.hp.toString(),mp:player.mp.toString(),pet:pet&&{life:pet.life.toString(),hp:pet.hp.toString(),mp:pet.mp.toString()},zone:session.baseline.safeZoneId.toString(),projection:a.world.map.projection,nav:a.nav.state,render:a.world.isMapReady};});}
 async function snapshot(name){await page.screenshot({path:path.join(out,name+'.png')});return state();}
 async function travel(map){await tap('BottomHUD/Footer/Map');await page.waitForFunction(()=>cc.find('Canvas/Safe/PopupView/Panels/Confirm')?.activeInHierarchy,null,{timeout:120000});await tap('PopupView/Panels/Confirm/ConfirmButton');await ready(map);}
 async function ground(point){
  const p=await page.evaluate(({x,y})=>{const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],w=a.world,q=w.projection.project(x,y),s=w.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(q.x,q.y)),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+s.x/v.width*r.width,y:r.y+(v.height-s.y)/v.height*r.height,world:{x:w.world.position.x,y:w.world.position.y},projected:{x:q.x,y:q.y},visible:{width:v.width,height:v.height}};},point);
  assert(p.x>0&&p.x<720&&p.y>128&&p.y<936,'来源路径落点不在实际触屏视口：'+JSON.stringify({point,p}));
  await page.touchscreen.tap(p.x,p.y);
  await page.waitForFunction(({x,y})=>{const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],w=a?.world,p=w?.visuals.get(w.playerHandle)?.state,display=w?.playerPosition;return p?.position.x===x&&p?.position.y===y&&!a.nav.state.target&&display&&Math.abs(display.x-x)<.01&&Math.abs(display.y-y)<.01;},point,{timeout:25000});
  report.moves.push(point);
 }
 try{
  await page.goto('http://127.0.0.1:18080/?townTravelQA=1');await scene('Login');const account='tqa'+crypto.randomBytes(5).toString('hex'),password=crypto.randomBytes(9).toString('hex');
  await edit('Account',account);await edit('Password',password);await tap('Register');await scene('RoleSelect');await tap('JobTabs/Job2');await edit('Name','Town'+crypto.randomBytes(3).toString('hex'));await tap('Create');await page.waitForFunction(()=>cc.find('Canvas/Safe/Role0')?.getComponentsInChildren(cc.Label).some(l=>l.string.includes('Town')));await tap('Enter');await scene('Game');await ready('1');
  if(await page.evaluate(()=>cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked))await tap('MapViewport/Actions/Auto');
  report.before=await snapshot('01-personal');assert(report.before.pet,'正常道士入场必须有现存宠物');report.checks.push('正常注册、道士创角与个人图触屏入场，无数据库预置');
  await travel('2');report.town=await snapshot('02-town-entry');assert.equal(report.town.projection,'source_rectangular');assert.deepEqual(report.town.position,{x:633,y:613});assert.equal(report.town.zone,'0');assert.equal(report.town.life,report.before.life);assert.equal(report.town.pet.life,report.before.pet.life);report.checks.push('保存地图按钮及确认面板进入真实城镇；新scene/首视口/ACK就绪，角色和宠物Life保留');
  for(const point of [nearby(639,619),nearby(645,625),{x:650,y:631}])await ground(point);
  report.safe=await snapshot('03-safe-zone');assert.equal(report.safe.zone,'106');report.checks.push('真实触屏逐段寻路到来源安全区650631，权威ID106');
  await tap('BottomHUD/Footer/ChangeRole');await scene('RoleSelect');await tap('Enter');await scene('Game');await ready('2');report.reentered=await snapshot('04-town-reentry');assert.deepEqual(report.reentered.position,report.safe.position);assert.equal(report.reentered.life,report.safe.life);assert.equal(report.reentered.hp,report.safe.hp);assert.equal(report.reentered.mp,report.safe.mp);assert.notEqual(report.reentered.binding,report.safe.binding);report.checks.push('正常换角/新票健康重入仍在原Town，位置/生命/魔法保留');
  await travel('1');report.returned=await snapshot('05-personal-return');assert.equal(report.returned.projection,'isometric_diamond');assert.equal(report.returned.life,report.before.life);report.checks.push('真实返回保留的个人现场，恢复原菱形地图与操作');
  assert.equal(report.errors.length,0,report.errors.join('\n'));assert.equal(report.entries.filter(e=>e.map==='2').length,2);assert(report.travel.filter(e=>e.phase===2).length>=2);report.passed=true;
 }catch(error){report.error=error.stack||String(error);try{await page.screenshot({path:path.join(out,'failure.png')});}catch{}throw error;}
 finally{fs.writeFileSync(path.join(out,'report.json'),JSON.stringify(report,null,2));await browser.close();}
 console.log(JSON.stringify({passed:report.passed,checks:report.checks.length,errors:report.errors.length,warnings:report.warnings.length}));
})().catch(error=>{console.error(error.message);process.exitCode=1;});
