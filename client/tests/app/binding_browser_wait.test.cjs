const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const source=fs.readFileSync(path.resolve(__dirname,'../../../tools/client/verify_town_travel_browser.cjs'),'utf8');
const start=source.indexOf(' async function observedBinding('),end=source.indexOf(' async function state(',start);assert(start>=0&&end>start);
function actualWait(page){return new Function('page',source.slice(start,end)+'\nreturn observedBinding;')(page);}
test('实际绑定浏览器工具等待同步返回false直到可信receipt与完整Inventory均就绪，不能返回Promise抢先结束',async()=>{
 const observed={bindingState:null,inventory:null},handle={disposed:false,async dispose(){this.disposed=true;}},results=[];
 const page={async evaluateHandle(fn){assert.equal(fn.constructor.name,'AsyncFunction');return handle;},async waitForFunction(predicate,arg,options){
  assert.equal(arg.observed,handle);assert.equal(options.timeout,30000);assert.equal(predicate.constructor.name,'Function');
  const sample=()=>{const result=predicate({observed,trusted:arg.trusted});assert.equal(typeof result,'boolean');results.push(result);};sample();
  observed.bindingState={trusted:false,binding:{mapId:'2',center:{x:650,y:631}},sourceSeq:'0',roleRevision:'11'};sample();
  observed.bindingState={...observed.bindingState,trusted:true,sourceSeq:'9'};observed.inventory={revision:'10'};sample();observed.inventory.revision='11';sample();assert.equal(results.at(-1),true);
 }};
 await actualWait(page)(true);assert.deepEqual(results,[false,false,false,true]);assert.equal(handle.disposed,true);
});
test('实际绑定等待超时或页面错误仍释放只读JSHandle，不持有过期观察引用',async()=>{
 let disposed=0;const page={async evaluateHandle(){return{async dispose(){disposed++;}};},async waitForFunction(){throw Error('未就绪超时');}};
 await assert.rejects(actualWait(page)(true),/未就绪超时/);assert.equal(disposed,1);
});

test('实际浏览器ready在首次入场/旅行/健康重入均记录并核验当前配置版本，旧服务即使Ready也失败',async()=>{
 const begin=source.indexOf(' async function ready(map)'),finish=source.indexOf(' async function observedBinding(',begin);assert(begin>=0&&finish>begin);
 const expected=String(JSON.parse(fs.readFileSync(path.resolve(__dirname,'../../../config/development/first-loop.json'),'utf8')).rule_version),report={readyRuleChecks:[]};let version=expected;
 const page={async waitForFunction(){},async evaluate(){return version;}};
 const ready=new Function('page','assert','expectedRuleVersion','report',source.slice(begin,finish)+'\nreturn ready;')(page,assert,expected,report);
 for(const map of ['1','2','2','1'])await ready(map);assert.deepEqual(report.readyRuleChecks.map(row=>row.resourceVersion),[expected,expected,expected,expected]);
 version=(BigInt(expected)-1n).toString();await assert.rejects(ready('2'),/规则版本与当前配置不一致/);assert.equal(report.readyRuleChecks.at(-1).resourceVersion,version);
});
