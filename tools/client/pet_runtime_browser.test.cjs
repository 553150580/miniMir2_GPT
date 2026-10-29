const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'verify_pet_runtime_browser.cjs'),'utf8');
function extract(start,end){const a=source.indexOf(start),b=source.indexOf(end,a);assert(a>=0&&b>a);return source.slice(a,b);}
test('实际尸体GUI until等待false Promise，只有显式true结束；异常不被吞成成功',async()=>{
 const until=new Function('sleep',extract('async function until(','(async()=>{')+';return until;')(()=>Promise.resolve());let count=0;
 await until(async()=>++count===3,'假状态',1000);assert.equal(count,3);await assert.rejects(until(async()=>{throw Error('观察失败');},'失败',1000),/观察失败/);
});
test('实际ready只读JSHandle轮询同步Boolean，Ready及渲染/附着缺一不得把Promise当真',async()=>{
 const previous=global.cc,observed={ready:false,entry:{roomId:{toString:()=> '1'}}},a={world:{isMapReady:true},reconciliationStarted:true,canSendInput:()=>true};global.cc={director:{getScene:()=>({getComponentsInChildren:()=>[a]})}};let checked=false;
 try{const page={async waitForFunction(fn,arg){assert.equal(fn.constructor.name,'Function');assert.equal(arg,observed);assert.equal(fn(observed),false);observed.ready=true;assert.equal(fn(observed),true);a.world.isMapReady=false;assert.equal(fn(observed),false);a.world.isMapReady=true;checked=true;}};
 const ready=new Function('page','observed','readState','assert','expectedRuleVersion',extract(' async function ready()',' async function readState()')+';return ready;')(page,observed,async()=>({resourceVersion:'16'}),assert,'16');await ready();assert(checked);
 }finally{global.cc=previous;}
});


test('实际QA健康核验拒绝相同fixtureHash但不同run的三节点instance',async()=>{
 const evidence={ports:{account:18100,game:18101,gm:18102},fixture_sha256:'same-fixture',health_instances:{account:'new-account',game:'new-game',gm:'new-gm'}};let stale=false;
 const fetch=async url=>{const mode=Object.keys(evidence.ports).find(key=>url.includes(':'+evidence.ports[key]+'/'));return{ok:true,async json(){return{fixture_hash:'same-fixture',mode,rule_version:16,instance:stale&&mode==='game'?'old-game':evidence.health_instances[mode]};}};};
 const verify=new Function('assert','evidence','fetch','expectedRuleVersion',extract('async function verifyQAHealth()','(async()=>{')+';return verifyQAHealth;')(assert,evidence,fetch,'16');assert.deepEqual(await verify(),evidence.health_instances);stale=true;await assert.rejects(verify(),/本轮节点instance/);
});


test('实际Cocos采样算法能避开两邻实体覆盖的中心九点，从尸体根矩形外角正常触屏命中',()=>{
 const oldCC=global.cc,oldDocument=global.document;
 class Transform{constructor(x,y){this.x=x;this.y=y;this.width=64;this.height=96;this.anchorPoint={x:.5,y:.5};}convertToWorldSpaceAR(p){return{x:p.x+this.x,y:p.y+this.y};}convertToNodeSpaceAR(p){return{x:p.x-this.x,y:p.y-this.y};}}
 const visual=(handle,x,y,order)=>({state:{entityHandle:handle,entityKind:2,life:'0',position:{x:8,y:6}},node:{activeInHierarchy:true,position:{x,y},_uiProps:{uiTransformComp:new Transform(x,y)},getSiblingIndex:()=>order},pose:{action:'die',frame:9}}),visuals=new Map([[2,visual(2,360,640,1)],[1,visual(1,328,624,2)],[10,visual(10,392,656,3)]]);
 global.cc={director:{getScene:()=>({getComponentsInChildren:()=>[{world:{visuals}}]})},view:{getVisibleSize:()=>({width:720,height:1280})},Vec3:class{constructor(x,y){Object.assign(this,{x,y});}},isValid:()=>true};global.document={querySelector:()=>({getBoundingClientRect:()=>({x:0,y:0,width:720,height:1280})})};
 try{const sample=new Function(extract('function sampleCorpseTouch(','async function verifyQAHealth()')+';return sampleCorpseTouch;')()({handle:2,life:'0'});assert(sample.found);assert.equal(sample.sample.hits[0].handle,2);assert(sample.candidates.filter(p=>Math.abs(p.offset.x)<=.2&&Math.abs(p.offset.y)<=.2).every(p=>p.hits[0].handle!==2),'原9点确实全部被挡');assert(Math.abs(sample.sample.offset.y)>=.35);assert.equal(sample.visualDiagnostics.length,3);
 }finally{global.cc=oldCC;global.document=oldDocument;}
});

test('实际创角前只读核道士已选中且保存Loading关闭，场景名已切换仍不能提前创建',()=>{
 const oldCC=global.cc,app={selectedJob:0},loading={activeInHierarchy:true},status={string:''},button={activeInHierarchy:true,getComponent:()=>({interactable:true})};
 global.cc={director:{getScene:()=>({name:'RoleSelect',getComponentsInChildren:()=>[app]})},Button:class{},Label:class{},find:path=>path==='Canvas/Loading'?loading:path==='Canvas/Safe/JobTabs/Job2'?button:{getComponent:()=>status}};
 try{const read=new Function(extract('function readTaoSelection()','async function verifyQAHealth()')+';return readTaoSelection;')();assert.equal(read().ready,false);app.selectedJob=2;status.string='创建职业：男道士';assert.equal(read().ready,false,'选职值正确但Loading仍遮挡');loading.activeInHierarchy=false;assert.equal(read().ready,true);assert.equal(read().selectedJob,2);assert.equal(read().status,'创建职业：男道士');app.selectedJob=0;assert.equal(read().selectedJob,0,'普通触屏未生效不能假定职业');}
 finally{global.cc=oldCC;}
});

test('实际场景交互门槛拒绝仅director已切换但AppRoot未绑定或Loading仍遮挡',()=>{
 const oldCC=global.cc,oldWindow=global.window;let apps=[],loading=true;const scene={name:'Login',getComponentsInChildren:()=>apps};global.cc={director:{getScene:()=>scene},find:path=>({activeInHierarchy:path==='Canvas/Loading'?loading:true})};global.window={cc:global.cc};
 try{const read=new Function(extract('function sceneInteractive(','function readTaoSelection()')+';return sceneInteractive;')();assert.equal(read('Login'),false);apps=[{}];assert.equal(read('Login'),false);loading=false;assert.equal(read('Login'),true);assert.equal(read('RoleSelect'),false);}
 finally{global.cc=oldCC;global.window=oldWindow;}
});

test('实际EditBox正常触屏必须先切换目标，已有Account输入框可见不能把Password仍写入旧框',async()=>{
 let active='Account';const taps=[],values=[],page={locator:()=>({isVisible:async()=>true,fill:async value=>values.push({field:active,value})})};
 const edit=new Function('page','tap','until',extract(' async function edit(',' async function observe()')+';return edit;')(page,async field=>{taps.push(field);active=field;},async predicate=>assert.equal(await predicate(),true));
 await edit('Password','仅内存测试值');assert.deepEqual(taps,['Password']);assert.deepEqual(values,[{field:'Password',value:'仅内存测试值'}]);
});
