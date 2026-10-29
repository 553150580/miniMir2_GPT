const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long}=require('./helpers/session_harness.cjs');
const {validatePetObservation,petTrainingLines,isLivingPet}=require('../../../.work/ui-tests/app/PetTrainingModel.js');
const {BaselineAssembler}=require('../../../.work/ui-tests/app/BaselineAssembler.js');
const {NavigationCancelAssembler}=require('../../../.work/ui-tests/app/NavigationCancelAssembler.js');
const {selectSkillTarget}=require('../../../.work/ui-tests/app/SkillModel.js');
const training=(changes={})=>({trainingLevel:1,maxTrainingLevel:7,experience:Long.fromString('9007199254740993',true),nextLevelExperience:Long.fromInt(40000,true),...changes});
const pet=(handle=10,changes={})=>({entityHandle:handle,entityKind:2,hp:Long.fromInt(50),hpMax:Long.fromInt(100),life:Long.UONE,petTraining:training(),position:{x:8,y:8},direction:2,...changes});
const chunk=(index,changes={})=>({sceneEpoch:Long.UONE,baselineId:Long.fromInt(2,true),stateSequence:Long.fromInt(3,true),chunkCount:2,chunkIndex:index,navigationState:{status:4,revision:Long.UONE},entities:[pet(index+10)],...changes});
test('训练Long精确显示、单次升级超额合法；满级保留经验，legacy不编造等级',()=>{
 const entity=pet();validatePetObservation(entity);assert(petTrainingLines(entity).join('\n').includes('9007199254740993 / 40000'));assert.equal(entity.petTraining.trainingLevel,1);
 const max=pet(10,{petTraining:training({trainingLevel:7,nextLevelExperience:Long.UZERO})});assert.match(petTrainingLines(max).join('\n'),/已达训练等级上限/);assert.match(petTrainingLines(max).join('\n'),/9007199254740993/);
 assert.deepEqual(petTrainingLines(pet(10,{petTraining:null})),[]);for(const change of [{experience:9007199254740993},{experience:Long.NEG_ONE},{trainingLevel:8},{trainingLevel:1,nextLevelExperience:Long.UZERO},{trainingLevel:7,nextLevelExperience:Long.UONE}])assert.throws(()=>validatePetObservation(pet(10,{petTraining:training(change)})));
});
test('不同handle的128尸体与活宠完整分块，重复handle不同Life仍拒绝且非法活尸体不入基线',()=>{
 const corpses=Array.from({length:128},(_,i)=>pet(100+i,{hp:Long.ZERO,isPetCorpse:true})),assembler=new BaselineAssembler();
 assert.equal(assembler.accept(chunk(1,{entities:[pet(10),...corpses.slice(64)]}),Long.UONE,Long.fromInt(2,true)),null);
 const complete=assembler.accept(chunk(0,{entities:[{entityHandle:1},...corpses.slice(0,64)]}),Long.UONE,Long.fromInt(2,true));assert.equal(complete.entities.length,130);assert.equal(complete.entities.filter(e=>e.isPetCorpse).length,128);assert.equal(complete.entities.filter(e=>e.entityKind===2&&isLivingPet(e)).length,1);
 assembler.reset();assembler.accept(chunk(0,{entities:[pet()]}),Long.UONE,Long.fromInt(2,true));assert.throws(()=>assembler.accept(chunk(1,{entities:[pet(10,{life:Long.fromInt(2,true)})]}),Long.UONE,Long.fromInt(2,true)),/重复/);
 for(const entity of [pet(10,{isPetCorpse:true}),pet(10,{entityKind:3}),pet(4294967296)]){assembler.reset();assert.throws(()=>assembler.accept(chunk(0,{chunkCount:1,entities:[entity]}),Long.UONE,Long.fromInt(2,true)));}
});
test('实体友方治疗拒绝NoMaster尸体含错误活HP标识，自动选择不会把Kind2尸体当活宠',()=>{
 const row={learned:true,cost:0,definition:{active:true,kind:'healing',target_kind:'friendly_entity',range:10}},player={entityHandle:1,entityKind:1,hp:Long.fromInt(100),position:{x:8,y:8},direction:2};
 const corpse=pet(10,{hp:Long.ZERO,isPetCorpse:true}),live=pet(11);assert.equal(selectSkillTarget(row,[corpse,live,player],null,1),player);
 assert.throws(()=>selectSkillTarget(row,[corpse,live,player],10,1),/倒下/);assert.equal(selectSkillTarget(row,[corpse,live,player],11,1),live);assert.equal(isLivingPet({...corpse,hp:Long.UONE}),false);assert.throws(()=>selectSkillTarget(row,[{...corpse,hp:Long.UONE},player],10,1),/倒下/);
});
test('退出取消仅完整同scope同序列的匹配RPC块可确认，乱序合法且旧碎片不能补齐',()=>{
 const scope=()=>new NavigationCancelAssembler(Long.UONE,Long.fromInt(2,true)),a=scope();assert.equal(a.accept(chunk(1)),null);const complete=a.accept(chunk(0));assert.equal(complete.assembledNavigationCancel,true);assert.equal(complete.entities.length,2);assert.throws(()=>a.accept(chunk(0)),/重复/);
 for(const change of [{sceneEpoch:Long.fromInt(2,true)},{baselineId:Long.UONE},{stateSequence:Long.fromInt(4,true)},{chunkCount:3},{navigationState:{status:1,revision:Long.UONE}},{navigationState:{status:4,revision:Long.fromInt(2,true)}}]){const b=scope();b.accept(chunk(0));assert.throws(()=>b.accept(chunk(1,change)));}
 const b=scope();b.accept(chunk(0));assert.throws(()=>b.accept(chunk(0)),/重复/);assert.throws(()=>scope().accept(chunk(0,{chunkCount:65})),/越界/);
});


test('实体总2048与取消最多64个唯一块的上界实际执行，不把128尸体候选当承载验收',()=>{
 const a=new BaselineAssembler(),scope=[Long.UONE,Long.fromInt(2,true)];const entities=Array.from({length:2048},(_,i)=>({entityHandle:i+1}));assert.equal(a.accept(chunk(0,{chunkCount:1,entities}),...scope).entities.length,2048);
 a.reset();assert.throws(()=>a.accept(chunk(0,{chunkCount:1,entities:[...entities,{entityHandle:3000}]}),...scope),/越界/);a.reset();assert.equal(a.accept(chunk(0,{entities:entities.slice(0,1024)}),...scope),null);assert.throws(()=>a.accept(chunk(1,{entities:[...entities.slice(1024),{entityHandle:3000}]}),...scope),/总数越界/);
 const cancel=new NavigationCancelAssembler(...scope);for(let i=0;i<64;i++){const value=cancel.accept(chunk(i,{chunkCount:64,entities:[]}));assert.equal(value?.assembledNavigationCancel,i===63?true:undefined);}assert.throws(()=>cancel.accept(chunk(0,{chunkCount:64,entities:[]})),/完成/);
});
