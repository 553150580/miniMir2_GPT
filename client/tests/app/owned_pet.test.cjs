const {test}=require('node:test'),assert=require('node:assert/strict'),path=require('node:path'),fs=require('node:fs');
const {Long,load,root,entry}=require('./helpers/session_harness.cjs');
const model=name=>require(path.join(root,'.work/ui-tests/app',name+'.js'));
const {parseOwnedPet,ownedPetCanRecall,ownedPetLines}=model('OwnedPetModel');
const {BaselineAssembler}=model('BaselineAssembler'),{NavigationCancelAssembler}=model('NavigationCancelAssembler');
const training=(changes={})=>({trainingLevel:2,maxTrainingLevel:7,experience:Long.MAX_UNSIGNED_VALUE,nextLevelExperience:Long.fromInt(60000,true),...changes});
const pet=(changes={})=>({entityHandle:20,life:Long.UZERO,definitionId:1,mapId:Long.MAX_UNSIGNED_VALUE,hp:Long.fromInt(50),hpMax:Long.fromInt(100),petTraining:training(),recallSkillIds:[17],followSuppressed:false,...changes});
const chunk=(index,changes={})=>({sceneEpoch:Long.UONE,baselineId:Long.fromInt(2,true),stateSequence:Long.fromInt(3,true),chunkCount:2,chunkIndex:index,entities:[],navigationState:{status:4,revision:Long.UONE},ownedPet:pet(),...changes});

test('Owned只接原始Long和真实活宠，Life0/uint64地图/超额EXP精确保留，legacy不造训练',()=>{
 const value=pet(),parsed=parseOwnedPet(value);assert.equal(parsed.life.toString(),'0');assert.equal(parsed.mapId.toString(),'18446744073709551615');assert.match(ownedPetLines(parsed,Long.UONE).join(''),/异图驻留/);assert.match(ownedPetLines(parsed).join(''),/18446744073709551615/);assert(ownedPetCanRecall(parsed,17));assert(!ownedPetCanRecall(parsed,30));assert(!ownedPetCanRecall(null,17));
 value.recallSkillIds.length=0;value.petTraining.trainingLevel=3;assert(ownedPetCanRecall(parsed,17));assert.equal(parsed.petTraining.trainingLevel,2,'外部包后续变化不能污染已组装观察');
 const legacy=ownedPetLines(pet({petTraining:null,followSuppressed:true}),Long.UONE).join('');assert.match(legacy,/跟随已暂停/);assert.match(legacy,/未提供训练数据/);assert(!legacy.includes('训练等级'));
 for(const change of [{life:0},{mapId:Long.UZERO},{hp:Long.ZERO},{hp:Long.fromInt(101)},{hpMax:Long.MAX_UNSIGNED_VALUE},{definitionId:0},{recallSkillIds:[17,17]},{recallSkillIds:[0]},{recallSkillIds:['17']},{followSuppressed:1},{petTraining:training({experience:'18446744073709551615'})}])assert.throws(()=>parseOwnedPet(pet(change)));
});

test('Owned跨块完整同序列才可见，空值/训练/抑制/召回列表冲突拒绝，旧片不能清新活宠',()=>{
 const a=new BaselineAssembler(),scope=[Long.UONE,Long.fromInt(2,true),Long.UONE];assert.equal(a.accept(chunk(1),...scope),null);const completed=a.accept(chunk(0),...scope);assert.equal(completed.entities.length,0,'异图Owned不是当前body');assert.equal(completed.ownedPet.mapId.toString(),'18446744073709551615');
 for(const change of [null,pet({life:Long.UONE}),pet({mapId:Long.UONE}),pet({hp:Long.fromInt(49)}),pet({followSuppressed:true}),pet({recallSkillIds:[]}),pet({petTraining:training({experience:Long.UONE})})]){const b=new BaselineAssembler();b.accept(chunk(0),...scope);assert.throws(()=>b.accept(chunk(1,{ownedPet:change}),...scope),/活宠状态不一致/);}
 const next=chunk(0,{stateSequence:Long.fromInt(4,true),ownedPet:null});assert.equal(a.accept(next,...scope),null);assert.equal(a.accept(chunk(1),...scope),null);assert.equal(a.accept({...next,chunkIndex:1},...scope).ownedPet,null);assert.equal(a.accept(chunk(0,{sceneEpoch:Long.fromInt(5,true)}),...scope),null);
});

test('同图Owned与实体逐项对拍，异图不准出现幽灵body；退出ACK也拒绝冲突Owned元数据',()=>{
 const scope=[Long.UONE,Long.fromInt(2,true),Long.UONE],owned=pet({mapId:Long.UONE}),body={entityHandle:20,entityKind:2,definitionId:1,life:Long.UZERO,hp:Long.fromInt(50),hpMax:Long.fromInt(100),petTraining:training()};
 assert.equal(new BaselineAssembler().accept(chunk(0,{chunkCount:1,ownedPet:owned,entities:[body]}),...scope).ownedPet.entityHandle,20);
 for(const changes of [{entities:[]},{entities:[{...body,hp:Long.UONE}]},{entities:[{...body,petTraining:training({experience:Long.UONE})}]},{entities:[{...body,isPetCorpse:true,hp:Long.ZERO}]},{ownedPet:pet(),entities:[body]}])assert.throws(()=>new BaselineAssembler().accept(chunk(0,{chunkCount:1,ownedPet:owned,entities:[body],...changes}),...scope));
 const cancel=new NavigationCancelAssembler(...scope);assert.equal(cancel.accept(chunk(0)),null);assert.throws(()=>cancel.accept(chunk(1,{ownedPet:pet({followSuppressed:true})})),/活宠状态不一致/);
});

test('实际PB编码→完整Assembler→真实Session dispatcher：异图活宠Long与scene/role更换隔离，非真实WS',()=>{
 const codecs=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../../client/assets/scripts/generated/protobuf/client_pb.js'):id==='./NavigationCancelAssembler'?{NavigationCancelAssembler}:{});
 class Connection{constructor(){this.generation=1;this.binding=Long.UONE;}subscribe(fn){this.listener=fn;}onStatus(fn){this.status=fn;}deliver(id,value){this.listener({id,value,error:0,binding:this.binding});}}
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?{GameConnection:Connection,newOperationId:()=>new Uint8Array(16).fill(3)}:id==='./BaselineAssembler'?{BaselineAssembler}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:['./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel'].includes(id)?model(id.slice(2)):{}),session=h.session;
 session.role={job:2,key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(1)}};const e=entry();session.connection.deliver(20002,e);const c=index=>chunk(index,{sceneEpoch:e.sceneEpoch,baselineId:e.baselineId});const decode=value=>codecs.decodePacket(codecs.encodePacket(5,0,Long.UONE,value)).value;
 session.connection.deliver(5,decode(c(1)));assert.equal(session.baseline,null);session.connection.deliver(5,decode(c(0)));assert.equal(session.baseline.ownedPet.mapId.toString(),'18446744073709551615');assert.equal(session.baseline.ownedPet.petTraining.experience.toString(),'18446744073709551615');assert.equal(session.baseline.entities.length,0);
 session.connection.generation++;session.connection.binding=Long.fromInt(2,true);session.role={job:2,key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(2)}};const next=entry('9007199254740994');session.connection.deliver(20002,next);assert.equal(session.baseline,null);session.connection.deliver(5,decode(c(0)));assert.equal(session.baseline,null,'旧角色原scope碎片不能挂回新角色');session.connection.deliver(5,decode(chunk(0,{chunkCount:1,sceneEpoch:next.sceneEpoch,baselineId:next.baselineId,ownedPet:null})));assert.equal(session.baseline.ownedPet,null);
});

test('实际public17与关闭召唤候选合同：NoPets逐图显式，主legacy不暗开技能/书/护符或采用候选Count',()=>{
 const publicData=JSON.parse(fs.readFileSync(path.join(root,'client/assets/resources/development/gameplay.json'),'utf8')),fixture=JSON.parse(fs.readFileSync(path.join(root,'config/development/first-loop.json'),'utf8')),candidate=JSON.parse(fs.readFileSync(path.join(root,'config/development/summon-candidates.json'),'utf8'));
 assert(Number(publicData.rule_version)>=candidate.required_rule_version);assert.equal(candidate.required_rule_version,17);assert.equal(candidate.runtime_enabled,false);assert.equal(fixture.pet_runtime.bootstrap_mode,'legacy_untrained_pet');assert.equal(fixture.casting_supply.enabled,false);assert(!publicData.skills.some(s=>s.id===17));assert(!fixture.skills.some(s=>s.id===17));assert(!fixture.items.some(i=>i.id===25));assert(publicData.map_catalog.length>=2);for(const map of publicData.map_catalog){assert.equal(typeof map.no_pets,'boolean');assert.equal(typeof map.no_teleport,'boolean');}
 const definition=candidate.skills.find(s=>s.id===17);assert.equal(definition.behavior,'summon_pet');assert.equal(definition.target_kind,'friendly_self');assert.equal(definition.material_item_id,187);assert.equal(definition.material_quantity,1);assert.equal(definition.auto_enabled,false);assert.equal(candidate.books.find(b=>b.id===25).book_skill_id,17);
 const {skillCost}=model('SkillModel');for(let level=0;level<=definition.max_level;level++)assert.equal(skillCost({kind:definition.behavior,base_cost:definition.mp_base_cost,level_cost:definition.mp_level_cost},level),definition.mp_base_cost+definition.mp_level_cost*level);
});
