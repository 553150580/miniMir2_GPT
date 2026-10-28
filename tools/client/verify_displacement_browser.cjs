// 新账号、原生UI购书/学习/施放；只为新角色准备等级与余额，凭据仅保留在本进程内存。
const assert=require('node:assert/strict'),crypto=require('node:crypto'),fs=require('node:fs'),path=require('node:path');
const {execFileSync}=require('node:child_process');
const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
const pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const out='outputs/development-displacement-skills-20261028/browser';fs.mkdirSync(out,{recursive:true});
const build=JSON.parse(fs.readFileSync('docs/客户端构建检查.json','utf8'));
assert(build.success&&build.scene_prefab_layout_preserved&&build.source_stable_during_build);
for(const [file,expected]of Object.entries({...build.asset_hashes,...build.script_hashes}))assert.equal(crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex'),expected,'构建后源文件变化：'+file);
function dev(args,input){return JSON.parse(execFileSync(process.env.MINIMIR_PYTHON,['tools/dev',...args],{encoding:'utf8',input,windowsHide:true,maxBuffer:1024*1024}))}
function packet(payload){const b=Buffer.from(payload);if(b.length<6)return null;let offset=4;function variable(){let value=0n,shift=0n;for(let i=0;i<10;i++){const n=b[offset++];value|=BigInt(n&127)<<shift;if(!(n&128))return value;shift+=7n}throw Error('无效帧')}const seq=Number(variable());variable();const error=b[1]&1?Number(variable()):0;return{id:b.readUInt16BE(2),seq,error,body:b.subarray(offset)}}
(async()=>{
 const browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
 const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1}),page=await context.newPage();
 const errors=[],warnings=[],traces=[],checks=[],casts=[];
 let preconditions=null;
 page.on('pageerror',error=>errors.push(error.stack||error.message));
 page.on('console',message=>{if(['warning','error'].includes(message.type()))warnings.push({type:message.type(),text:message.text().slice(0,400)})});
 page.on('websocket',socket=>socket.on('framereceived',event=>{try{
  const p=packet(event.payload);if(!p||p.error)return;
  if(p.id===30006){const v=pb.CastAccepted.decode(p.body);if([8,27].includes(v.skill_id)){casts.push(v);traces.push({kind:'cast',skill:v.skill_id,rank:v.skill_level,cast:v.cast_id.toString(),input:v.input_sequence})}}
  if(p.id===30013){const v=pb.EntityDisplacement.decode(p.body);traces.push({kind:'movement',skill:v.source_skill_id,cast:v.cast_id.toString(),entity:v.entity_handle,movement:v.kind,revision:v.movement_revision.toString(),origin:v.origin,path:v.path.map(p=>({x:p.x,y:p.y}))})}
  if(p.id===30008){const v=pb.CombatImpact.decode(p.body);if([8,27].includes(v.skill_id))traces.push({kind:'impact',skill:v.skill_id,amount:v.amount.toString()})}
 }catch(error){errors.push(error.message)}}));
 const app='cc.director.getScene().getComponentsInChildren("AppRoot")[0]';
 async function scene(name){await page.waitForFunction(name=>window.cc?.director?.getScene()?.name===name,name,{timeout:45000})}
 async function idle(){await page.waitForFunction(()=>!cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.uiActionBusy,null,{timeout:15000})}
 async function position(expression,argument){return page.evaluate(({expression,argument})=>{
  const node=Function('argument','return ('+expression+')')(argument);if(!node||!node.activeInHierarchy)throw Error('UI节点不可交互');
  const box=node._uiProps.uiTransformComp.getBoundingBoxToWorld(),size=cc.view.getVisibleSize(),canvas=document.querySelector('canvas').getBoundingClientRect();
  return{x:canvas.x+(box.x+box.width/2)/size.width*canvas.width,y:canvas.y+(size.height-box.y-box.height/2)/size.height*canvas.height};
 },{expression,argument})}
 async function tap(suffix){const point=await position('cc.find("Canvas/Safe/"+argument)',suffix);await page.touchscreen.tap(point.x,point.y)}
 async function edit(suffix,value){await tap(suffix);const input=page.locator('input:visible');await input.waitFor();await input.fill(value)}
 async function popup(name){await page.waitForFunction(name=>cc.find('Canvas/Safe/PopupView/Panels/'+name)?.activeInHierarchy,name,{timeout:15000})}
 async function dismiss(){await page.touchscreen.tap(10,180);await page.waitForTimeout(70)}
 async function list(suffix,field,value){const index=await page.evaluate(({suffix,field,value})=>{
  const list=cc.find('Canvas/Safe/'+suffix).getComponent('List'),index=list.items.findIndex(item=>field.split('.').reduce((o,key)=>o?.[key],item)===value);
  if(index<0)throw Error('列表无对应条目：'+value);list.scrollToIndex(index);return index;
 },{suffix,field,value});await page.waitForTimeout(80);const point=await position('cc.find("Canvas/Safe/"+argument.suffix).getComponent("List").rows.get(argument.index)',{suffix,index});await page.touchscreen.tap(point.x,point.y)}
 async function state(){return page.evaluate(()=>{const a=cc.director.getScene().getComponentsInChildren('AppRoot')[0],s=a.skillSnapshot(),player=a.world.visuals.get(a.world.playerHandle).state;
  return{position:player.position,hp:player.hp.toString(),mp:player.mp.toString(),revision:player.movementRevision.toString(),life:player.life.toString(),learned:s.inventory.learnedSkills.map(v=>({id:v.skillId,level:v.level})),nav:a.nav.state.target,pose:a.world.visuals.get(a.world.playerHandle).pose}})}
 async function ground(x,y){const p=await page.evaluate(({x,y})=>{const w=cc.director.getScene().getComponentsInChildren('AppRoot')[0].world,local=w.projection.project(x,y),world=w.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(local.x,local.y)),size=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+world.x/size.width*r.width,y:r.y+(size.height-world.y)/size.height*r.height}},{x,y});assert(p.x>0&&p.x<720&&p.y>128&&p.y<936,'施放目标不在可触摸战斗区');await page.touchscreen.tap(p.x,p.y)}
 async function until(predicate,message){const end=Date.now()+10000;while(Date.now()<end){if(await predicate())return;await page.waitForTimeout(50)}throw Error(message)}
 try{
  await page.goto('http://127.0.0.1:18080/?renderQA=1');await scene('Login');
  const account='dqa'+crypto.randomBytes(5).toString('hex'),password=crypto.randomBytes(8).toString('hex'),names=['DM'+crypto.randomBytes(4).toString('hex'),'DW'+crypto.randomBytes(4).toString('hex')];
  await edit('Account',account);await edit('Password',password);await tap('Register');await scene('RoleSelect');await idle();
  for(let i=0;i<2;i++){await tap('JobTabs/Job'+(i===0?1:0));await edit('Name',names[i]);await tap('Create');await page.waitForFunction(name=>cc.find('Canvas/Safe').getComponentsInChildren(cc.Label).some(l=>l.string.includes(name)),names[i]);await idle()}
  checks.push('正常注册账号，分别在保存的选角场景创建法师与战士');
  dev(['python3','tools/server/dev.py','stop']);preconditions=dev(['python3','tools/client/prepare_displacement_qa.py'],JSON.stringify({account_name:account,role_names:names}));dev(['python3','tools/server/dev.py','start']);
  await page.reload();await scene('Login');await edit('Account',account);await edit('Password',password);await tap('Login');await scene('RoleSelect');await idle();
  for(const [index,id,book,offer]of [[0,8,8,25],[1,27,5,26]]){
   await page.waitForFunction(name=>cc.find('Canvas/Safe').getComponentsInChildren(cc.Label).some(l=>l.string.includes(name)),names[index]);
   await tap('Role'+index);await tap('Enter');await scene('Game');await page.waitForFunction(()=>cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.reconciliationStarted,null,{timeout:30000});
   if(await page.evaluate(()=>cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked))await tap('MapViewport/Actions/Auto');await idle();
   await tap('BottomHUD/Skills/SkillsPanel');await list('UIMgr/Panels/Skills/List','definition.id',id);await popup('StatusDetails');await dismiss();
   await tap('UIMgr/Panels/Skills/Shop');await list('UIMgr/Panels/SkillAssets/OffersList','offer_id',offer);await tap('UIMgr/Panels/SkillAssets/Preview');await popup('SkillQuote');
   await page.screenshot({path:path.join(out,id+'-01-purchase.png')});await tap('PopupView/Panels/SkillQuote/ConfirmButton');await idle();
   await page.waitForFunction(book=>cc.director.getScene().getComponentsInChildren('AppRoot')[0].skillSnapshot().inventory.items.some(i=>i.itemId===book),book);await dismiss();
   await tap('UIMgr/Panels/Skills/Learn');await popup('Confirm');await tap('PopupView/Panels/Confirm/ConfirmButton');await idle();
   await page.waitForFunction(id=>cc.director.getScene().getComponentsInChildren('AppRoot')[0].skillSnapshot().inventory.learnedSkills.some(s=>s.skillId===id&&s.level===0),id);
   checks.push(`技能${id}经真实书店、报价确认和原书学习；没有预置技能书或技能等级`);
   await page.screenshot({path:path.join(out,id+'-02-learned.png')});
   if(id===8)await page.waitForFunction(()=>{
    const w=cc.director.getScene().getComponentsInChildren('AppRoot')[0].world,p=w.visuals.get(w.playerHandle).state.position;
    return Array.from(w.visuals.values()).some(v=>v.state.entityKind===3&&BigInt(v.state.hp.toString())>0n&&Math.max(Math.abs(v.state.position.x-p.x),Math.abs(v.state.position.y-p.y))<=1);
   },null,{timeout:30000});
   const before=await state(),castCount=casts.length;
   await tap('UIMgr/Panels/Skills/Use');
   if(id===27){const point=before.position;await ground(point.x<14?point.x+2:point.x-2,point.y)}else await dismiss();
   await until(()=>casts.length>castCount,'没有收到新技能受理');const cast=casts[casts.length-1];
   await until(()=>traces.some(v=>v.kind==='movement'&&v.skill===id&&v.cast===cast.cast_id.toString()&&v.path.length>0),'没有观察到成功位移');
   await page.screenshot({path:path.join(out,id+'-03-movement.png')});
   await until(async()=>BigInt((await state()).mp)===BigInt(before.mp)-BigInt(id===8?2:4),'新技能耗蓝不符');const after=await state();
   if(id===8)assert(!traces.some(v=>v.kind==='impact'&&v.skill===8),'抗拒错误产生怪物伤害事件');
   if(id===27)assert.notDeepEqual(after.position,before.position);
   checks.push(`技能${id}真实触摸施放、MP只扣一次，并消费对应cast的权威位移路径`);
   await page.waitForTimeout(1000);await tap('BottomHUD/Footer/ChangeRole');await scene('RoleSelect');await idle();
  }
  assert.equal(errors.length,0,errors.join('\n'));
  fs.writeFileSync(path.join(out,'report.json'),JSON.stringify({passed:true,checks,errors,warnings,preconditions,touchInput:true,physicalMobileVerified:false,scope:'限定本机两项新技能正常购书/学习/施放；等级和余额是明确隔离QA前态，不证明自然养成、全阶GUI或真机'},null,2));
  console.log(JSON.stringify({passed:true,checks:checks.length,errors:errors.length,warnings:warnings.length}));
 }catch(error){fs.writeFileSync(path.join(out,'report.json'),JSON.stringify({passed:false,checks,errors,warnings,preconditions,error:error.stack},null,2));await page.screenshot({path:path.join(out,'failure.png')});throw error}
 finally{fs.writeFileSync(path.join(out,'trace.json'),JSON.stringify(traces,null,2));await browser.close()}
})().catch(error=>{console.error(error.message);process.exitCode=1});
