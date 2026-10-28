const {test}=require('node:test');
const assert=require('node:assert/strict');
const {Long,load,harness,entry,ready,context}=require('./helpers/session_harness.cjs');
const {MapTravelModel,travelInteger}=require('../../../.work/ui-tests/app/MapTravelModel.js');
const {MapAttachmentCoordinator,StaleMapAttachmentError}=require('../../../.work/ui-tests/app/MapAttachmentCoordinator.js');
const id='0123456789abcdef0123456789abcdef';
const source={connection:1,binding:'9007199254740993',role:'角色甲',scene:'9007199254740993',baseline:'9007199254740994',life:'3',rule:'13'};
const plan={travelId:id,fromSceneEpoch:source.scene,targetMapId:'2',targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:'2000'};
const identity={...source,scene:'9007199254740994',baseline:'9007199254740995',map:'2',travelId:id};
const state=(phase,scene=source.scene)=>({travelId:id,fromSceneEpoch:source.scene,sceneEpoch:scene,phase,targetMapId:'2'});
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};};
function prepared(){const m=new MapTravelModel();m.begin(source,id,1);m.acceptPlan(plan,source,1000);m.prepared(source,1500);return m;}

test('旅行保留完整uint64及原connection/binding/role/scene/baseline/life/rule，Number不能冒充身份',()=>{
    assert.equal(travelInteger('18446744073709551615'),'18446744073709551615');
    for(const value of [1,'01','-1','18446744073709551616'])assert.throws(()=>travelInteger(value));
    for(const key of ['connection','binding','role','scene','baseline','life','rule']){const m=prepared(),changed={...source,[key]:key==='connection'?2:key==='role'?'角色乙':'4'};assert.equal(m.matches(changed),false);assert.throws(()=>m.commit(changed,1501));}
    const m=new MapTravelModel();m.begin(source,id,1);assert.throws(()=>m.acceptPlan({...plan,targetSceneKind:1},source,1000));assert.throws(()=>m.acceptPlan({...plan,position:{x:-1,y:0}},source,1000));assert.throws(()=>m.acceptPlan(plan,source,2000));
});
test('提交前计划可过期，提交后旧Plan时限不导致本地回滚，ACK和Ready严格匹配新附着',()=>{
    const expired=prepared();assert.throws(()=>expired.commit(source,2000));assert.equal(expired.blocksInput,false);
    const m=prepared();m.commit(source,1999);assert.equal(m.blocksInput,true);assert.equal(m.acceptState(state(1),source),true);
    assert.equal(m.acceptState(state(2,'1'),source),false);assert.equal(m.acceptState(state(2,'9007199254740995'),source),false);
    assert.equal(m.acceptState(state(2,identity.scene),source),true);assert.throws(()=>m.cancelling());assert.equal(m.acceptState(state(4),source),false);
    assert.equal(m.acceptsEntry({...identity,map:'1'}),false);m.attach(identity);assert.equal(m.ready(identity),false);assert.equal(m.acknowledge({...identity,baseline:'8'}),false);
    assert.equal(m.acknowledge(identity),true);assert.equal(m.acknowledge(identity),false);assert.equal(m.ready({...identity,connection:2}),false);assert.equal(m.ready(identity),true);assert.equal(m.blocksInput,false);
    assert.equal(m.acceptState(state(2,identity.scene),source),true);assert.equal(m.phase,'complete');
});
test('准备失败取消与提交未知结果保留原旅行身份；旧连接状态、旧id不能解锁',()=>{
    const m=prepared();m.cancelling();m.uncertain();assert.equal(m.active,true);assert.equal(m.blocksInput,false);
    assert.equal(m.acceptState(state(3),{...source,binding:'8'}),false);assert.equal(m.acceptState({...state(3),travelId:'f'.repeat(32)},source),false);assert.equal(m.acceptState(state(3),source),true);
    const next=prepared();next.commit(source,1900);next.uncertain();assert.equal(next.blocksInput,true);assert.equal(next.acceptState(state(1),source),true);assert.equal(next.acceptState(state(4),source),true);assert.equal(next.blocksInput,false);
});
test('真实Session：无副作用拒绝可撤回当前op推测，原command1继续可用',()=>{
    const h=harness(),s=h.session;ready(h,entry());const op=h.operation();assert.equal(op.commandSeq.toString(),'1');assert.equal(s.command.toString(),'2');
    s.connection.deliver(40005,{opId:op.opId,pending:false,errorCode:1001,commandSeq:Long.UZERO});assert.equal(s.command.toString(),'1');assert.equal(h.operation().commandSeq.toString(),'1');
});
test('真实Session：旧scene/旧op拒绝不降低新Entry或SyncReady确认水位，Pending与新推测保留',()=>{
    const h=harness(),s=h.session;ready(h,entry());const old=h.operation();s.pending={原永久操作:true};
    const next=entry('9007199254740994','2',2,'8');ready(h,next);const current=h.operation();assert.equal(current.commandSeq.toString(),'8');assert.equal(s.command.toString(),'9');
    s.connection.deliver(40005,{opId:old.opId,pending:false,errorCode:1001,commandSeq:Long.UZERO});assert.equal(s.command.toString(),'9');assert.deepEqual(s.pending,{原永久操作:true});
    s.connection.deliver(6,{...next,nextEconomicCommandSeq:Long.fromInt(12,true)});s.connection.deliver(40005,{opId:current.opId,pending:false,errorCode:1001,commandSeq:Long.UZERO});assert.equal(s.command.toString(),'12');
});
test('真实Session：Pending收据不撤销推测，同场景新op拒绝不能复用已确认序号',()=>{
    const h=harness(),s=h.session;ready(h,entry());const a=h.operation();s.connection.deliver(40005,{opId:a.opId,pending:true,commandSeq:Long.UZERO});assert.equal(s.command.toString(),'2');
    s.connection.deliver(40005,{opId:a.opId,pending:false,commandSeq:Long.UONE});assert.equal(s.command.toString(),'2');const b=h.operation();
    s.connection.deliver(40005,{opId:a.opId,pending:false,commandSeq:Long.UZERO});assert.equal(s.command.toString(),'3');s.connection.deliver(40005,{opId:b.opId,pending:false,commandSeq:Long.UONE});assert.equal(s.command.toString(),'2');
});
test('真实Session：同连接旅行先收束旧动作，再接受新Entry；重复提交不撤销新ready，旧Entry不能回挂',()=>{
    const h=harness(),s=h.session,original=ready(h,entry());s.pending={保留:true};const source=context(h),m=s.travel;
    m.begin(source,id,1);m.acceptPlan({...plan,fromSceneEpoch:original.sceneEpoch},source,1000);m.prepared(source,1200);m.commit(source,1300);
    let attacks=0;s.attacks.add(()=>attacks++);s.connection.deliver(20010,{...state(1),fromSceneEpoch:original.sceneEpoch,sceneEpoch:original.sceneEpoch});assert.equal(s.ready,false);
    s.connection.deliver(6,original);assert.equal(s.ready,false);
    s.connection.deliver(30007,{sceneEpoch:original.sceneEpoch});assert.equal(attacks,1);
    const next=entry('9007199254740994','2',2);s.connection.deliver(20010,{...state(2,next.sceneEpoch),fromSceneEpoch:original.sceneEpoch});s.connection.deliver(20002,next);
    assert.equal(s.entry,next);assert.equal(s.ready,false);assert.deepEqual(s.pending,{保留:true});assert.equal(s.travel.phase,'attaching');assert.equal(h.isCurrentEntry(h.currentEntryIdentity()),true);s.connection.deliver(6,next);assert.equal(s.ready,false);
    s.connection.deliver(30007,{sceneEpoch:original.sceneEpoch});assert.equal(attacks,1);assert.equal(s.travel.acknowledge(h.currentEntryIdentity()),true);s.connection.deliver(6,next);assert.equal(s.travel.phase,'complete');
    s.connection.deliver(20010,{...state(2,next.sceneEpoch),fromSceneEpoch:original.sceneEpoch});assert.equal(s.ready,true);s.connection.deliver(20002,next);assert.equal(s.ready,true);
    s.connection.deliver(20002,original);assert.equal(s.entry,next);assert.equal(s.ready,true);
});
test('真实Session：收束取消只恢复原连接Ready，重连Town根据真实Entry保留地图2',()=>{
    const h=harness(),s=h.session,original=ready(h,entry()),c=context(h);s.travel.begin(c,id,1);s.travel.acceptPlan({...plan,fromSceneEpoch:c.scene},c,1000);s.travel.prepared(c,1100);s.travel.commit(c,1200);
    s.connection.deliver(20010,{...state(1),fromSceneEpoch:c.scene,sceneEpoch:c.scene});assert.equal(s.ready,false);s.connection.deliver(20010,{...state(3),fromSceneEpoch:c.scene,sceneEpoch:c.scene});assert.equal(s.ready,true);assert.equal(s.entry,original);
    s.connection.status('断线');s.connection.binding=Long.fromInt(2,true);const town=entry('1','2',2);s.connection.deliver(20002,town);assert.equal(s.entry.roomId.toString(),'2');assert.equal(s.travel.phase,'idle');assert.equal(s.ready,false);
});
test('附着协调器同身份复用；旧资源迟到只释放自己，不替换新world或发送旧ACK',async()=>{
    const c=new MapAttachmentCoordinator(),a=deferred(),log=[],asset=n=>({dispose(){log.push('asset:'+n);}}),view=n=>({dispose(){log.push('view:'+n);}});
    const one=c.attach(identity,{load:()=>a.promise,current:()=>true,mount:()=>{throw Error('旧资源不能挂');},prepare:async()=>{},acknowledge:()=>log.push('oldACK')});
    assert.equal(c.attach(identity,{}),one);const two=c.attach({...identity,scene:'9007199254740995'},{load:async()=>asset('new'),current:()=>true,mount:()=>view('new'),prepare:async()=>{},acknowledge:()=>log.push('newACK')});
    await two;a.resolve(asset('old'));await assert.rejects(one,StaleMapAttachmentError);assert.deepEqual(log,['newACK','asset:old']);c.dispose();assert.deepEqual(log,['newACK','asset:old','view:new']);
});
test('旧首视口迟到不重复销毁已退役world；失效附着绝不ACK',async()=>{
    const c=new MapAttachmentCoordinator(),a=deferred(),log=[];const steps=(n,prepare)=>({load:async()=>({dispose(){log.push('asset:'+n);}}),current:()=>true,mount:()=>({dispose(){log.push('view:'+n);}}),prepare,acknowledge:()=>log.push('ACK:'+n)});
    const one=c.attach(identity,steps('old',()=>a.promise));await new Promise(r=>setImmediate(r));await c.attach({...identity,scene:'99'},steps('new',async()=>{}));a.resolve();await assert.rejects(one,StaleMapAttachmentError);
    assert.deepEqual(log,['view:old','ACK:new']);c.dispose();assert.deepEqual(log,['view:old','ACK:new','view:new']);
});
test('纯资源首视口准备匹配原生中心、边界镜头与空间桶，只持有租约且取消/失败全部归还',async()=>{
    const {MapViewportPreparation}=load('client/assets/scripts/app/MapViewportPreparation.ts',()=>{}),draw=[],leases=[],gate=deferred();
    const assets={map:{width:700,height:700},sourceMap:{model:{manifest:{pixel_width:33600,pixel_height:22400},project:(x,y)=>({x:x*48,y:y*32}),queryChunks:rect=>{draw.push(rect);return [{id:'a'},{id:'b'}];}},acquireChunk:id=>{const l={id,ready:gate.promise,released:0,release(){this.released++;}};leases.push(l);return l;}}};
    const p=new MapViewportPreparation(assets),task=p.prepare({x:0,y:0},768,512);assert.deepEqual(draw[0],[0,0,768,512]);assert.deepEqual(leases.map(l=>l.released),[0,0]);p.release();gate.resolve();await assert.rejects(task,/取消/);p.release();assert.deepEqual(leases.map(l=>l.released),[1,1]);
    const q=new MapViewportPreparation(assets);await q.prepare({x:699,y:699},768,512);assert.deepEqual(draw[1],[32832,21888,33600,22400]);q.release();
    const fail=new MapViewportPreparation({...assets,sourceMap:{...assets.sourceMap,acquireChunk:()=>({ready:Promise.reject(Error('加载失败')),release(){draw.push('released');}})}});await assert.rejects(fail.prepare({x:633,y:613},768,512),/加载失败/);assert.equal(draw.filter(x=>x==='released').length,2);
});
