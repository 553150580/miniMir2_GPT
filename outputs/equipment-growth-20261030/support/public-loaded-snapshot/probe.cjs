// 仅打开保存Login并读取公开JsonAsset；不登录、触屏、创建角色或调用私有业务。
'use strict';
const assert=require('node:assert/strict'),crypto=require('node:crypto'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'../../..'),out=__dirname,self=__filename;
const sha=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const read=p=>JSON.parse(fs.readFileSync(p,'utf8'));
const expectedLoaded='44da4016f03227d1b89d0fa30dbbfb623193d02f2b1a6472be068489fa5109bf';
const qaFile=path.join(root,'tools/client/verify_equipment_browser.cjs');
const expectedQa='dc1e28b0cf2f826108bc853d94b969beb69fd80ac97fb4c6f69782cddd24fee5';
const rawPublic=path.join(root,'client/assets/resources/development/gameplay.json');
const fixtureFile=path.join(root,'config/development/first-loop.json');
const creatorFile=path.join(root,'docs/客户端构建检查.json');
const guiFile=path.join(root,'.work/equipment-growth-20261030/browser/run-02/report.json');
const clientFile=path.join(root,'.work/equipment-growth-20261030/client/full-1/report.json');
const cutoverFile=path.join(root,'.work/equipment-growth-20261030/runtime-control/report.json');
const reportFile=path.join(out,'report.json');

async function health(){const values=[];for(const port of[18080,18100,18101,18102]){const response=await fetch(`http://127.0.0.1:${port}/healthz`,{signal:AbortSignal.timeout(5000)});assert(response.ok);values.push({port,...await response.json()});}return values;}

async function main(){
 assert(!fs.existsSync(reportFile),'不覆盖已有探针报告');
 assert(!fs.existsSync(path.join(out,'loaded-public.json')),'不覆盖原始资源快照');
 const gui=read(guiFile),client=read(clientFile),creator=read(creatorFile),fixture=read(fixtureFile);
 const report={passed:false,started_at_utc:new Date().toISOString(),scope:'只读加载公开JsonAsset的原始JSON.stringify字节，用于绑定原run02既有loaded摘要；不是重新执行6项装备UI',
  tool_path:path.relative(root,self).replaceAll('\\','/'),tool_sha256:sha(self),node_version:process.version,
  execution_argv:process.argv,gui_report_path:path.relative(root,guiFile).replaceAll('\\','/'),gui_report_sha256:sha(guiFile),
  original_gui_passed:gui.passed,expected_loaded_sha256:expectedLoaded,
  frozen_qa_tool_sha256:sha(qaFile),current_public_raw_sha256:sha(rawPublic),fixture_sha256:sha(fixtureFile),
  creator_report_sha256:sha(creatorFile),creator_build_root:creator.build_root,
  client_report_sha256:sha(clientFile),cutover_report_sha256:sha(cutoverFile),
  no_login:true,no_database_writes:true,no_private_business:true,route_override:false,touch_input:false,
  account_request_count:0,websocket_count:0,page_errors:[],console_errors:[],warnings:[]};
 let browser,page;
 try{
  assert.equal(report.frozen_qa_tool_sha256,expectedQa);assert(gui.passed);assert.equal(gui.loadedPublicCanonicalSha256,expectedLoaded);
  assert.equal(report.current_public_raw_sha256,gui.publicSha256);assert.equal(report.fixture_sha256,gui.fixtureSha256);
  assert.equal(report.creator_report_sha256,gui.creatorReportSha256);assert.equal(fixture.rule_version,22);
  assert(creator.success&&creator.source_stable_during_build&&creator.scene_prefab_layout_preserved);
  assert.deepStrictEqual(client.before,client.after);assert(client.passed&&client.stable);
  const snapshot=()=>Object.fromEntries(Object.keys(client.before).map(file=>[file,sha(path.join(root,file))]));
  report.source_before=snapshot();assert.deepStrictEqual(report.source_before,client.after);
  report.core_source_before=Object.fromEntries(['client/assets/scripts/app/WorldAssets.ts','client/assets/scripts/app/GameWorldView.ts'].map(file=>[file,sha(path.join(root,file))]));
  report.health_before=await health();
  for(const h of report.health_before.filter(h=>h.port!==18080)){assert.equal(h.rule_version,22);assert.equal(h.fixture_hash,report.fixture_sha256);assert.equal(h.status,'ok');assert.equal(h.instance,gui.healthAfter.find(q=>q.port===h.port).instance);}
  const serving=report.health_before.find(h=>h.port===18080);assert(serving.webMobileBuild);assert.equal(path.resolve(serving.build_root),path.join(root,'client/build/web-mobile'));
  report.served_index_before_sha256=sha(path.join(serving.build_root,'index.html'));
  const {chromium}=require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT,'browser-tools/node_modules/playwright-core'));
  browser=await chromium.launch({channel:'chrome',headless:true,args:['--no-sandbox','--use-angle=swiftshader','--enable-unsafe-swiftshader']});
  const context=await browser.newContext({viewport:{width:720,height:1280},hasTouch:true,isMobile:true,deviceScaleFactor:1});
  page=await context.newPage();
  page.on('request',request=>{const u=new URL(request.url());if(u.pathname.startsWith('/account/'))report.account_request_count++;});
  page.on('websocket',()=>report.websocket_count++);
  page.on('pageerror',error=>report.page_errors.push(error.message));
  page.on('console',message=>{if(message.type()==='error')report.console_errors.push(message.text());if(message.type()==='warning')report.warnings.push(message.text());});
  await page.goto('http://127.0.0.1:18080/');
  await page.waitForFunction(()=>Boolean(window.cc?.director?.getScene()?.name==='Login'&&cc.find('Canvas/Safe')?.activeInHierarchy&&!cc.find('Canvas/Loading')?.activeInHierarchy),null,{timeout:90000});
  const captured=await page.evaluate(async()=>{
   const asset=await new Promise((resolve,reject)=>cc.resources.load('development/gameplay',cc.JsonAsset,(error,value)=>error?reject(error):resolve(value)));
   const bytes=JSON.stringify(asset.json);
   const digest=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(bytes))),b=>b.toString(16).padStart(2,'0')).join('');
   return{json:bytes,sha256:digest,scene:cc.director.getScene().name,resource:'development/gameplay',assetType:'cc.JsonAsset'};
  });
  const blob=path.join(out,'loaded-public.json');fs.writeFileSync(blob,Buffer.from(captured.json,'utf8'),{flag:'wx'});
  report.snapshot_path=path.relative(root,blob).replaceAll('\\','/');report.snapshot_sha256=sha(blob);report.browser_snapshot_sha256=captured.sha256;
  report.observed_scene=captured.scene;report.resource_key=captured.resource;report.asset_type=captured.assetType;
  assert.equal(report.snapshot_sha256,expectedLoaded,'实际loaded字节必须严格等于原run02摘要，不能替换原证据');
  assert.equal(report.snapshot_sha256,report.browser_snapshot_sha256);
  assert.deepStrictEqual(JSON.parse(fs.readFileSync(blob,'utf8')),JSON.parse(fs.readFileSync(rawPublic,'utf8')));
  report.node_deep_strict_equal_all_public_fields=true;
  report.comparison_method='本脚本在记录的实际Node版本中对JSON.parse(snapshot)与JSON.parse(currentRawPublic)执行assert.deepStrictEqual，不手写ECMA数字或顺序模拟';
  report.raw_public_node_stringify_sha256=crypto.createHash('sha256').update(JSON.stringify(read(rawPublic))).digest('hex');
  report.source_after=snapshot();assert.deepStrictEqual(report.source_after,report.source_before);
  report.core_source_after=Object.fromEntries(Object.keys(report.core_source_before).map(file=>[file,sha(path.join(root,file))]));assert.deepStrictEqual(report.core_source_after,report.core_source_before);
  report.tool_sha256_after=sha(self);assert.equal(report.tool_sha256_after,report.tool_sha256);
  report.frozen_qa_tool_sha256_after=sha(qaFile);assert.equal(report.frozen_qa_tool_sha256_after,expectedQa);
  report.current_public_raw_sha256_after=sha(rawPublic);assert.equal(report.current_public_raw_sha256_after,report.current_public_raw_sha256);
  report.creator_report_sha256_after=sha(creatorFile);assert.equal(report.creator_report_sha256_after,report.creator_report_sha256);
  report.gui_report_sha256_after=sha(guiFile);assert.equal(report.gui_report_sha256_after,report.gui_report_sha256);
  report.health_after=await health();assert.deepStrictEqual(report.health_after,report.health_before);
  report.served_index_after_sha256=sha(path.join(serving.build_root,'index.html'));assert.equal(report.served_index_after_sha256,report.served_index_before_sha256);
  assert.equal(report.account_request_count,0);assert.equal(report.websocket_count,0);assert.equal(report.observed_scene,'Login');
  assert.deepStrictEqual(report.page_errors,[]);assert.deepStrictEqual(report.console_errors,[]);
  report.passed=true;
 }catch(error){report.error={name:error.name,message:error.message,stack:error.stack};throw error;}
 finally{await browser?.close();report.finished_at_utc=new Date().toISOString();fs.writeFileSync(reportFile,JSON.stringify(report,null,2)+'\n',{flag:'wx'});}
 console.log(JSON.stringify({passed:report.passed,snapshot_sha256:report.snapshot_sha256,deep_equal:report.node_deep_strict_equal_all_public_fields,account_requests:report.account_request_count,websockets:report.websocket_count,report_sha256:sha(reportFile)}));
}
if(require.main===module)main().catch(error=>{console.error(error.stack||error.message);process.exitCode=1;});
