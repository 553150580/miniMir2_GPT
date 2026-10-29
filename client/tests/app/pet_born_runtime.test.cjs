const {test}=require('node:test');const assert=require('node:assert/strict');
const {makeWorld,state,Long,cc}=require('../support/world_runtime.cjs');
const {PetBornModel}=require('../../../.work/ui-tests/app/PetBornModel.js');
const scope={connection:1,binding:'1',role:'1:'+'09'.repeat(16),scene:'1',baseline:'1',map:'1',travelId:''};
const born=(changes={})=>({sceneEpoch:'1',baselineId:'1',stateSequence:'1',entityHandle:3,entityLife:'1',definitionId:1,mapId:'1',sourceCastId:'1',sourceSkillId:17,bornAtUnixMs:'1',movementRevision:'0',direction:5,role:{originServerId:'1',roleId:new Uint8Array(16).fill(9)},elapsedMs:0,...changes});
const pet=(changes={})=>state(3,2,8,8,{definitionId:1,life:Long.UONE,direction:5,...changes});
function fixture(audio={}){const f=makeWorld(1,{audio});f.model=new PetBornModel();f.model.bind(scope);f.view.bindPresentation(scope,()=>true,()=>f.model.discardPending());return f;}
function accept(f,event=born(),now=0){assert(f.model.accept(event,scope,'1',now));return f.model.take(scope,{sceneEpoch:'1',baselineId:'1',stateSequence:'1',entities:[pet()]},1000,now);}
test('真实GameWorldView无body绝不创建；明确事件裁到第3帧，加色材质并在第10帧后恢复普通身体',()=>{
 const f=fixture(),value=accept(f,born({elapsedMs:250}));assert.equal(f.view.presentPetBorn(value),false);assert.equal(f.view.visuals.size,0);
 f.apply([pet()]);assert.equal(f.view.petAppearanceAudit[0].action,'standing');assert(f.view.presentPetBorn(value));
 let audit=f.view.petAppearanceAudit[0];assert.equal(audit.action,'appear');assert.equal(audit.frame,2);assert.equal(audit.direction,5);assert.equal(audit.blend[0].destination,cc.gfx.BlendFactor.ONE);
 const visual=f.view.visuals.get(3),nameY=visual.node.getChildByName('Name').position.y;f.update(649);assert.equal(visual.pose.frame,8);f.apply([pet()],650);assert.equal(visual.pose.frame,9);assert.equal(visual.node.getChildByName('Name').position.y,nameY);
 f.update(750);assert.equal(visual.pose.action,'standing');assert.equal(visual.body[0].customMaterial,null);assert.equal(f.model.accept(born(),scope,'1',750),false);f.view.dispose();
});
test('新Life/尸体/同birth新位移取消旧出场，同attachment资源失效不能移交pending给重建view',()=>{
 for(const changes of [{life:Long.fromInt(2,true)},{hp:Long.ZERO,isPetCorpse:true},{movementRevision:Long.UONE,position:{x:9,y:8},movementKind:1}]){
  const f=fixture();f.apply([pet()]);assert(f.view.presentPetBorn(accept(f)));f.apply([pet(changes)],100);assert.notEqual(f.view.petAppearanceAudit[0].action,'appear');f.view.dispose();
 }
 const f=fixture();assert(f.model.accept(born(),scope,'1',0));f.assets.notifyBeforeInvalidation('source');assert.equal(f.model.pendingCount,0);assert.equal(f.model.accept(born(),scope,'1',1),false);f.view.dispose();
});
function clip(key,seconds){return {key,refs:0,addRef(){this.refs++;return this;},decRef(){assert(this.refs>0);this.refs--;},getDuration(){return seconds;}};}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
test('正常触屏解锁，真实AudioSource两声道与完整WAV时长；动画结束不会截断新生原声',async()=>{
 const cast=clip('cast',1.136),birth=clip('birth',1.505),f=fixture({summon_skeleton_cast:cast,pet_skeleton_born:birth});f.apply([state(),pet()]);
 assert.equal(f.view.audioAudit.gestureUnlocked,false);cc.input.touch();assert.equal(f.view.audioAudit.gestureUnlocked,true);
 assert(f.view.presentPetBorn(accept(f)));await flush();assert.equal(f.view.audioAudit.voices[1].cue,'pet_skeleton_born');assert.equal(f.view.audioAudit.voices[1].playCalls,1);
 f.update(1000);assert.equal(f.view.petAppearanceAudit[0].action,'standing');assert.equal(f.view.audioAudit.voices[1].playing,true);
 f.update(1505);assert.equal(f.view.audioAudit.voices[1].clipBound,false);assert.equal(f.view.audioAudit.voices[1].playing,false);
 f.view.dispose();assert.equal(cast.refs,0);assert.equal(birth.refs,0);
});
test('出生声音微任务迟到跨Life不播放；资源归还之前两AudioSource先stop与clip=null',async()=>{
 const cast=clip('cast',1.136),birth=clip('birth',1.505),f=fixture({summon_skeleton_cast:cast,pet_skeleton_born:birth});f.apply([pet()]);cc.input.touch();
 assert(f.view.presentPetBorn(accept(f)));const audio=f.view.audio;f.apply([pet({life:Long.fromInt(2,true)})],10);await flush();assert.equal(audio.audit.voices[1].playCalls,0);
 f.view.dispose();assert(audio.voices.every(voice=>!voice.source.clip&&voice.source.destroyed));assert.equal(cast.refs,0);assert.equal(birth.refs,0);
 const g=fixture({summon_skeleton_cast:clip('cast',1.136),pet_skeleton_born:clip('birth',1.505)});g.apply([pet()]);cc.input.touch();assert(g.view.presentPetBorn(accept(g)));const old=g.view.audio;g.view.dispose();await flush();assert.equal(old.audit.voices[1].playCalls,0);
});
test('17起手fresh与Recall每个CastAccepted只发声一次，1136ms原声不被600ms动画截断',async()=>{
 const f=fixture({summon_skeleton_cast:clip('cast',1.136),pet_skeleton_born:clip('birth',1.505)});f.assets.gameplay.skills.push({id:17,active:true,kind:'summon_pet',target_kind:'friendly_self',cast_ms:600});
 f.apply([state(1,1,8,8,{life:Long.UONE}),pet()]);cc.input.touch();
 const cast={sceneEpoch:Long.UONE,castId:Long.UONE,skillId:17,casterHandle:1,casterLife:Long.UONE,targetHandle:1,targetLife:Long.UONE,origin:{x:8,y:8},targetGrid:{x:8,y:8},direction:2,delayMs:500};
 f.view.acceptCast(cast);f.view.acceptCast(cast);await flush();assert.equal(f.view.audioAudit.voices[0].playCalls,1);assert.equal(f.view.audioAudit.voices[1].playCalls,0);
 f.update(600);assert.equal(f.view.audioAudit.voices[0].playing,true);f.update(1136);assert.equal(f.view.audioAudit.voices[0].clipBound,false);
 f.update(1800);f.view.acceptCast({...cast,castId:Long.fromInt(2,true)});await flush();assert.equal(f.view.audioAudit.voices[0].playCalls,2);assert.equal(f.view.audioAudit.voices[1].playCalls,0);f.view.dispose();
});
test('WorldAssets直接释放走真实失效通知，clip引用降0的瞬间已无任何声道绑定',async()=>{
 const a=clip('cast',1.136),b=clip('birth',1.505),f=fixture({summon_skeleton_cast:a,pet_skeleton_born:b});f.apply([pet()]);cc.input.touch();assert(f.view.presentPetBorn(accept(f)));await flush();const audio=f.view.audio;
 for(const value of [a,b]){const release=value.decRef;value.decRef=function(){assert(audio.voices.every(v=>v.source.clip!==this));release.call(this);};}
 f.assets.dispose();assert.equal(a.refs,0);assert.equal(b.refs,0);assert(audio.voices.every(v=>v.source.destroyed&&!v.source.clip));f.view.dispose();
});
