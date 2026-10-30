// 正常触屏建角+真实提交后丢响应；消息列表采用显式隔离UI前态，不能据此认证业务消息来源。
'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {packet}=require('./verify_warehouses_browser.cjs');
const root=path.resolve(__dirname,'../..'),pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const arg=name=>{const i=process.argv.indexOf(name);return i<0?null:process.argv[i+1];};
const sha=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex'),read=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const secrets=[],redact=value=>secrets.reduce((text,secret)=>text.replaceAll(secret,'[已隐去测试凭据]'),value),hex=bytes=>Buffer.from(bytes).toString('hex');
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
function creationRequest(raw){const p=packet(raw);if(!p||p.id!==10005)return null;const q=pb.CreateRoleRequest.decode(p.body);return{op:hex(q.op_id),origin:q.origin_server_id.toString(),slot:q.slot,job:q.job,name:q.name};}

async function main(){
 const out=path.resolve(root,arg('--report-dir')||'.work/p2-fixes-20261030/browser/run-01');
 assert(out.startsWith(path.join(root,'.work/p2-fixes-20261030/browser')+path.sep));assert(!fs.existsSync(path.join(out,'report.json')),'保留既有报告，不覆盖');fs.mkdirSync(out,{recursive:true});
 const base=arg('--url')||'http://127.0.0.1:18080/',url=new URL(base);assert.equal(url.hostname,'127.0.0.1');assert.equal(url.protocol,'http:');assert.equal(url.pathname,'/');assert(!url.search&&!url.hash&&!url.username&&!url.password);
 const creatorPath=path.resolve(arg('--creator-report')||'docs/客户端构建检查.json'),creator=read(creatorPath),fixturePath=path.join(root,'config/development/first-loop.json');
 assert(creator.success&&creator.creator==='3.8.8'&&creator.exit_code===36&&creator.source_stable_during_build);
 const frozen={...creator.asset_hashes,...creator.script_hashes};assert(Object.keys(frozen).some(p=>p.endsWith('/PendingRoleCreation.ts')),'候选构建必须包含建角修复');
 const snapshot=()=>Object.fromEntries(Object.keys(frozen).map(p=>[p,sha(path.join(root,p))]));assert.deepEqual(snapshot(),frozen);
 const report={passed:false,startedAt:new Date().toISOString(),url:base,creatorReportPath:creatorPath,creatorReportSha256:sha(creatorPath),fixtureSha256:sha(fixturePath),toolSha256:sha(__filename),sourceBefore:snapshot(),
  touchInput:true,privateBusinessInvoked:false,credentialsWritten:false,databaseSeeded:false,isolatedUiPrestateInjected:false,businessMessageSourcesVerified:false,physicalMobileVerified:false,
  checks:[],errors:[],warnings:[],expectedTransportErrors:[],createRequests:[],createResponses:[],messageObservations:[],screenshots:[]};
 let browser,page,observed,abortArmed=true;
 try{
  async function health(){const rows=[],s=await fetch(base+'healthz',{signal:AbortSignal.timeout(5000)});assert(s.ok);const h=await s.json();assert(h.webMobileBuild&&h.port===Number(url.port));rows.push({port:Number(url.port),...h});
   for(const port of[h.account_port,h.game_port,Number(arg('--gm-port')||(url.port==='18080'?'18102':'18112'))]){const r=await fetch(`http://127.0.0.1:${port}/healthz`,{signal:AbortSignal.timeout(5000)});assert(r.ok);const v=await r.json();assert(v.status==='ok'&&v.rule_version===read(fixturePath).rule_version&&v.fixture_hash===report.fixtureSha256);rows.push({port,...v});}return rows;}
  report.healthBefore=await health();
  const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
  browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
  const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1});page=await context.newPage();
  page.on('pageerror',e=>report.errors.push(e.stack||e.message));
  page.on('console',m=>{if(m.type()==='warning')report.warnings.push(m.text());if(m.type()==='error'){
   const location=m.location();if(report.abortedAfterCommit&&m.text().includes('net::ERR_FAILED')&&location.url===url.origin+'/account/rpc')report.expectedTransportErrors.push(m.text());else report.errors.push(m.text());}});
  // 只解析10005，认证正文、请求头、口令与票据绝不写报告。
  await page.route(url.origin+'/account/rpc',async route=>{
   const request=route.request(),raw=request.postDataBuffer();if(!raw||raw.length<6||raw.readUInt16BE(2)!==10005){await route.continue();return;}
   // Playwright故障堆栈可能含HTTP调用日志；仅在内存登记遮蔽，不输出请求头。
   const authorization=request.headers().authorization;if(authorization){secrets.push(authorization);if(authorization.startsWith('Bearer '))secrets.push(authorization.slice(7));}
   const q=creationRequest(raw);report.createRequests.push(q);
   const response=await route.fetch(),body=await response.body(),p=packet(body);assert(response.ok()&&p&&p.id===10006&&!p.error);const role=pb.RoleSummary.decode(p.body);
   const summary={role:hex(role.key.role_id),origin:role.key.origin_server_id.toString(),slot:role.slot,job:role.job,name:role.name};report.createResponses.push(summary);
   assert.equal(summary.origin,q.origin);assert.equal(summary.slot,q.slot);assert.equal(summary.job,q.job);assert.equal(summary.name,q.name.normalize('NFKC'));
   if(abortArmed){abortArmed=false;report.abortedAfterCommit={...summary,scope:'真实10005已成功返回给代理；仅向浏览器丢弃该响应'};await route.abort('failed');}
   else await route.fulfill({response,body});
  });
  const frames=()=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  async function scene(name){await page.waitForFunction(name=>Boolean(window.cc?.director?.getScene()?.name===name&&cc.find('Canvas/Safe')?.activeInHierarchy&&!cc.find('Canvas/Loading')?.activeInHierarchy),name,{timeout:90000});await frames();}
  async function tap(suffix){await page.waitForFunction(suffix=>{const n=cc.find('Canvas/Safe/'+suffix),a=cc.director.getScene()?.getComponentsInChildren('AppRoot')[0],b=n?.getComponent(cc.Button)??n?.getComponent(cc.Toggle);return Boolean(n?.activeInHierarchy&&(!b||b.interactable&&b.enabledInHierarchy)&&(suffix.startsWith('PopupView/')||!a?.uiActionBusy));},suffix,{timeout:30000});
   const p=await page.evaluate(suffix=>{const t=cc.find('Canvas/Safe/'+suffix)._uiProps.uiTransformComp,w=t.convertToWorldSpaceAR(new cc.Vec3((.5-t.anchorPoint.x)*t.width,(.5-t.anchorPoint.y)*t.height)),v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();return{x:r.x+w.x/v.width*r.width,y:r.y+(v.height-w.y)/v.height*r.height};},suffix);await page.touchscreen.tap(p.x,p.y);await frames();}
  async function edit(suffix,value){await tap(suffix);const input=page.locator('input:visible');await input.waitFor({timeout:10000});await input.fill(value);await frames();}
  async function shot(name){await page.screenshot({path:path.join(out,name+'.png')});report.screenshots.push(name+'.png');}
  async function idle(){await page.waitForFunction(()=>{const a=cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];return a&&!a.uiActionBusy&&!a.roleCreationTask;},null,{timeout:30000});await frames();}
  await page.goto(base);await scene('Login');const account='p2qa'+crypto.randomBytes(5).toString('hex'),password=crypto.randomBytes(9).toString('hex');secrets.push(account,password);
  await edit('Account',account);await edit('Password',password);await tap('Register');await scene('RoleSelect');await tap('JobTabs/Job2');
  const name='核对'+crypto.randomBytes(3).toString('hex');await edit('Name',name);await tap('Create');await idle();assert(report.abortedAfterCommit);
  observed=await page.evaluateHandle(async()=>(await System.import('chunks:///_virtual/GameSession.ts')).session);
  report.afterLostResponse=await page.evaluate(s=>{const q=s.roleCreation.get();return{roles:s.roles.length,pending:q?{op:Array.from(q.opId,b=>b.toString(16).padStart(2,'0')).join(''),origin:q.originServerId.toString(),slot:q.slot,job:q.job,name:q.name}:null};},observed);
  assert.equal(report.afterLostResponse.roles,0);assert.deepEqual(report.afterLostResponse.pending,report.createRequests[0]);await shot('01-committed-response-lost');
  await edit('Name','不替换原请求');await tap('JobTabs/Job0');await tap('Create');await idle();
  report.afterRetry=await page.evaluate(s=>({pending:s.roleCreation.get(),roles:s.roles.map(r=>({role:Array.from(r.key.roleId,b=>b.toString(16).padStart(2,'0')).join(''),slot:r.slot,job:r.job,name:r.name}))}),observed);
  assert.equal(report.createRequests.length,2);assert.deepEqual(report.createRequests[1],report.createRequests[0]);assert.deepEqual(report.createResponses[1],report.createResponses[0]);assert.equal(report.afterRetry.pending,null);assert.equal(report.afterRetry.roles.length,1);assert.equal(report.afterRetry.roles[0].name,name);await shot('02-original-operation-confirmed');
  report.checks.push('10005真实提交后丢响应，修改表单后触屏重试仍为原op/原槽/职业/姓名，目录仅一个原角色且journal清除');
  await tap('Role0');await tap('Enter');await scene('Game');
  await page.waitForFunction(s=>{const a=cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];return s.ready&&s.inventory&&s.wallet&&a?.world?.isMapReady&&a.reconciliationStarted&&!a.attaching&&!a.inputBarrier&&!a.uiActionBusy;},observed,{timeout:120000});
  if(await page.evaluate(()=>cc.find('Canvas/Safe/MapViewport/Actions/Auto').getComponent(cc.Toggle).isChecked))await tap('MapViewport/Actions/Auto');await idle();
  // 唯一注入是会话消息模型的明确隔离UI样本，不发送协议、不写数据库、不产生收益。
  report.messagePrestate=await page.evaluate(()=>{const m=cc.director.getScene().getComponentsInChildren('AppRoot')[0].messages,start=m.log.serial;
   const rows=Array.from({length:40},(_,i)=>({id:start+i+1,title:'隔离UI样本',message:(i<30?'筛选项':'其它项')+' '+String(i).padStart(2,'0')}));m.log.entries=rows;m.log.serial=start+40;m.pending=true;
   return{count:rows.length,ids:rows.map(r=>r.id),scope:'仅隔离消息列表前态；非服务端业务或自然消息来源'};});report.isolatedUiPrestateInjected=true;
  await tap('BottomHUD/Chat');
  async function listState(){return page.evaluate(()=>{const list=cc.find('Canvas/Safe/UIMgr/Panels/Messages/List').getComponent('List'),view=list.viewport._uiProps.uiTransformComp,mask=view.getBoundingBoxToWorld(),rows=Array.from(list.rows,([index,node])=>{const t=node._uiProps.uiTransformComp,b=t.getBoundingBoxToWorld();return{index,id:list.items[index]?.id,text:node.getChildByName('Text').getComponent(cc.Label).string,active:node.activeInHierarchy,bottom:b.y,top:b.y+b.height};});
   return{offset:list.scrollView.getScrollOffset().y,max:list.scrollView.getMaxScrollOffset().y,rowHeight:list.rowHeight,itemCount:list.itemCount,rows,visible:rows.filter(r=>r.active&&r.top>mask.y&&r.bottom<mask.y+mask.height),mask:{bottom:mask.y,top:mask.y+mask.height},items:list.items.map(x=>({id:x.id,title:x.title,message:x.message})),scrolling:list.scrollView.isScrolling()||list.scrollView.isAutoScrolling()};});}
  const beforeSearch=await listState();assert(beforeSearch.offset>beforeSearch.rowHeight*10);report.messageObservations.push({name:'before-search',...beforeSearch});
  await edit('UIMgr/Panels/Messages/Search','筛选项');await tap('UIMgr/Panels/Messages/Summary');await frames();
  const afterSearch=await listState();assert.equal(afterSearch.itemCount,30);assert(Math.abs(afterSearch.offset)<1);assert(afterSearch.visible.some(row=>row.index===0&&row.text.includes('筛选项 00')));assert(afterSearch.visible.every(row=>row.text.includes('筛选项')));report.messageObservations.push({name:'after-search',...afterSearch});await shot('03-search-first-rows-visible');
  async function swipe(){const p=await page.evaluate(()=>{const list=cc.find('Canvas/Safe/UIMgr/Panels/Messages/List').getComponent('List'),t=list.viewport._uiProps.uiTransformComp,v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect(),point=f=>{const w=t.convertToWorldSpaceAR(new cc.Vec3((.5-t.anchorPoint.x)*t.width,(f-t.anchorPoint.y)*t.height));return{x:r.x+w.x/v.width*r.width,y:r.y+(v.height-w.y)/v.height*r.height};};return{start:point(.2),end:point(.8)};});const cdp=await context.newCDPSession(page);await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[p.start]});for(let i=1;i<=8;i++){await cdp.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:p.start.x+(p.end.x-p.start.x)*i/8,y:p.start.y+(p.end.y-p.start.y)*i/8}]});await sleep(60);}await cdp.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});await cdp.detach();await page.waitForFunction(()=>{const s=cc.find('Canvas/Safe/UIMgr/Panels/Messages/List').getComponent('List').scrollView;return!s.isScrolling()&&!s.isAutoScrolling();},null,{timeout:10000});await frames();}
  for(let i=0;i<12;i++){if((await listState()).offset>=afterSearch.rowHeight*8)break;await swipe();}
  const beforeAnchor=await listState(),anchorIndex=Math.floor(beforeAnchor.offset/beforeAnchor.rowHeight),drop=Math.min(18,anchorIndex-2);assert(drop>=1);assert(beforeAnchor.max-beforeAnchor.offset>beforeAnchor.rowHeight,'须在非底部检查锚点恢复');
  const anchorId=beforeAnchor.items[anchorIndex].id,expectedOffset=(anchorIndex-drop)*beforeAnchor.rowHeight+beforeAnchor.offset%beforeAnchor.rowHeight;
  await page.evaluate(drop=>{const m=cc.director.getScene().getComponentsInChildren('AppRoot')[0].messages;m.log.entries=m.log.entries.slice(drop);m.pending=true;},drop);
  // 点击真实Mask内条目；触摸结束让组件刷新隔离前态，并打开保存的详情覆盖层。
  const selected=await page.evaluate(()=>{const list=cc.find('Canvas/Safe/UIMgr/Panels/Messages/List').getComponent('List'),view=list.viewport._uiProps.uiTransformComp,v=cc.view.getVisibleSize(),r=document.querySelector('canvas').getBoundingClientRect();for(const[index,node]of list.rows){const t=node._uiProps.uiTransformComp,w=t.convertToWorldSpaceAR(new cc.Vec3((.5-t.anchorPoint.x)*t.width,(.5-t.anchorPoint.y)*t.height)),p=view.convertToNodeSpaceAR(w);if(node.activeInHierarchy&&p.y>-view.anchorPoint.y*view.height+10&&p.y<(1-view.anchorPoint.y)*view.height-10)return{x:r.x+w.x/v.width*r.width,y:r.y+(v.height-w.y)/v.height*r.height,id:list.items[index].id};}throw Error('未找到Mask内可触摸消息');});
  await page.touchscreen.tap(selected.x,selected.y);await page.waitForFunction(()=>cc.find('Canvas/Safe/PopupView/Panels/Details')?.activeInHierarchy);await frames();
  const afterAnchor=await listState();assert(Math.abs(afterAnchor.offset-expectedOffset)<1);assert.equal(afterAnchor.items[Math.floor(afterAnchor.offset/afterAnchor.rowHeight)].id,anchorId);assert(afterAnchor.visible.length>0);assert(afterAnchor.visible.every(row=>row.text.includes(afterAnchor.items[row.index].message)));
  await shot('04-anchor-and-details');await page.touchscreen.tap(10,180);await page.waitForFunction(()=>!cc.find('Canvas/Safe/PopupView/Panels/Details')?.activeInHierarchy);await frames();
  const afterBack=await listState();assert(Math.abs(afterBack.offset-afterAnchor.offset)<1);assert(afterBack.visible.length>0);report.messageObservations.push({name:'anchor-preserved',before:beforeAnchor,drop,anchorId,expectedOffset,after:afterAnchor,afterBack});await shot('05-message-list-preserved');
  report.checks.push('明确隔离40条UI前态，触屏搜索30项立即显示Mask内首行；淘汰旧样本后触屏保持原消息锚点，详情返回保留下层列表和滚动');
  await page.touchscreen.tap(10,180);await tap('BottomHUD/Footer/ChangeRole');await scene('RoleSelect');await idle();
  report.afterReentry=await page.evaluate(s=>({count:s.roles.length,pending:s.roleCreation.get()}),observed);assert.equal(report.afterReentry.count,1);assert.equal(report.afterReentry.pending,null);assert.equal(report.createRequests.length,2);await shot('06-role-directory-reentry');
  report.checks.push('真实保存切角入口重新列出同一个角色，不重发已终态10005');
  report.healthAfter=await health();assert.deepEqual(report.healthAfter,report.healthBefore);report.sourceAfter=snapshot();assert.deepEqual(report.sourceAfter,report.sourceBefore);assert.equal(sha(__filename),report.toolSha256);assert.deepEqual(report.errors,[]);report.passed=true;
 }catch(error){report.error=redact(error.stack||error.message);if(page)try{if(await page.evaluate(()=>['RoleSelect','Game'].includes(window.cc?.director?.getScene()?.name)))await page.screenshot({path:path.join(out,'failure.png')});}catch{}throw error;
 }finally{report.finishedAt=new Date().toISOString();await observed?.dispose();await browser?.close();fs.writeFileSync(path.join(out,'report.json'),redact(JSON.stringify(report,null,2))+'\n');}
 console.log(JSON.stringify({passed:report.passed,checks:report.checks.length,errors:report.errors.length,warnings:report.warnings.length,isolatedUiPrestateInjected:report.isolatedUiPrestateInjected}));
}
module.exports={creationRequest};
if(require.main===module)main().catch(error=>{console.error(redact(error.stack||error.message));process.exitCode=1;});
