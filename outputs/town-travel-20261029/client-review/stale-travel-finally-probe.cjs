const fs=require('node:fs'),path=require('node:path'),{createRequire}=require('node:module'),assert=require('node:assert/strict');
const file=path.resolve(__dirname,'../../client/tests/app/map_travel_runtime.test.cjs');
const prefix=fs.readFileSync(file,'utf8').split("\ntest(")[0].replace('new Uint8Array(16).fill(7)','new Uint8Array(16).fill(++operationSerial)');
const holder={exports:{}};new Function('require','module','exports','let operationSerial=6;\n'+prefix+'\nmodule.exports=appHarness;')(createRequire(file),holder,holder.exports);
const {Long,entry,ready}=require('../../client/tests/app/helpers/session_harness.cjs');
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve};};
const flush=()=>new Promise(r=>setImmediate(r));
(async()=>{
 const savedNow=Date.now;let now=1000;Date.now=()=>now;
 try{
 const t=holder.exports(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();s.pending={get:()=>null};
 const oldLoad=deferred(),confirm=deferred();t.pendingLoads.push(oldLoad);t.a.popup={confirm:()=>confirm.promise};
 s.connection.request=async(code,value)=>{if(code===20007)return{travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(now+1000)};const state={travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:3};s.connection.deliver(20010,state);return state;};
 const old=t.a.travelMap();const oldRejected=assert.rejects(old,/失效/);await flush();const oldID=s.travel.id;assert.equal(s.travel.phase,'preparing');
 now=2100;s.connection.deliver(20010,{travelId:oldID,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:4});t.a.handleSessionUpdate();assert.equal(s.travel.phase,'expired');
 const current=t.a.travelMap();await flush();const currentID=s.travel.id,prepared=t.a.preparedTravel;assert.notEqual(oldID,currentID);assert.equal(s.travel.phase,'confirming');assert(prepared);assert.equal(prepared.assets.disposed,0);
 oldLoad.resolve(t.asset('2',true));await oldRejected;
 assert.equal(s.travel.id,currentID);assert.equal(s.travel.phase,'confirming');assert.equal(prepared.assets.disposed,1);assert.equal(t.a.preparedTravel,undefined);
 console.log(JSON.stringify({scope:'真实冻结AppRoot.travelMap及Session，复用现有runtime harness；受控时钟推进服务端准备过期，同连接同scene新计划',current_phase:s.travel.phase,old_id:oldID,new_id:currentID,new_prepared_assets_disposed:prepared.assets.disposed,new_prepared_slot_cleared:t.a.preparedTravel===undefined}));
 confirm.resolve(false);await current;t.a.attachment.dispose();
 }finally{Date.now=savedNow;}
})().catch(error=>{console.error(error);process.exitCode=1;});
