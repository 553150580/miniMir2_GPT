const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const model=require('../../../.work/ui-tests/app/SkillModel.js');
const {CastPlayback,MeleePlayback,effectPoses}=require('../../../.work/ui-tests/app/CastPlayback.js');
const state=changes=>({sceneEpoch:'9007199254740993',revision:'9007199254740993',playerLife:'0',serverTimeUnixMs:'1900000000000',
    states:[{skillId:12,enabled:true,ready:false,remainingMs:0,cooldownRemainingMs:0},{skillId:26,enabled:false,ready:false,remainingMs:0,cooldownRemainingMs:10000}],...changes});
const definition=changes=>({id:12,name:'刺杀剑术',job_id:0,kind:'thrusting',active:true,activation_kind:'toggle',target_kind:'attack_mode',range:2,
    base_cost:0,level_cost:0,cast_ms:600,impact_base_ms:500,accuracy_per_level:0,...changes});
test('模式全量状态接收保持Long修订精度，初始life0合法，拒绝旧场景/旧修订/重复条目',()=>{
    const first=state();assert(model.acceptsSkillModeState(null,first,first.sceneEpoch));
    assert(!model.acceptsSkillModeState(first,state(),first.sceneEpoch));
    assert(model.acceptsSkillModeState(first,state({revision:'9007199254740994',playerLife:'1'}),first.sceneEpoch));
    assert(!model.acceptsSkillModeState(null,state({sceneEpoch:'2'}),first.sceneEpoch));
    assert(!model.acceptsSkillModeState(null,state({states:[first.states[0],first.states[0]]}),first.sceneEpoch));
    assert(!model.acceptsSkillModeState(null,state({states:[{...first.states[0],remainingMs:-1}]}),first.sceneEpoch));
});
test('倒计时归零只更新文字，不授予烈火ready；开关文字反映服务端状态，快捷短名不超过三字',()=>{
    const modes=state(),snapshot={sceneEpoch:modes.sceneEpoch,modes,modesReceivedAt:1000};
    const toggle={definition:definition(),learned:true,level:0,cost:0};
    assert.equal(model.skillActionLabel(snapshot,toggle),'关闭');assert.equal(model.skillQuickStateLabel(snapshot,toggle),'刺杀开');
    assert.equal(model.skillQuickStateLabel({...snapshot,modes:null},toggle),'刺杀');
    const charge=definition({id:26,name:'烈火剑法',kind:'flaming_sword',activation_kind:'charge',target_kind:'attack_charge'});
    assert.equal(model.skillModeStatus(snapshot,charge,2000),'冷却 9秒');assert.equal(model.skillModeStatus(snapshot,charge,12000),'可充能');
    assert.equal(modes.states[1].ready,false);assert.equal(model.skillActionLabel(snapshot,{...toggle,definition:charge}),'充能');
    modes.states[1]={...modes.states[1],ready:true,remainingMs:10000};assert.match(model.skillModeStatus(snapshot,charge,12000),/已就绪/);
    assert.equal(model.skillQuickStateLabel(snapshot,{...toggle,definition:charge}),'烈火备');
    for(const [kind,name]of [['thrusting','刺杀剑术'],['half_moon','半月弯刀'],['flaming_sword','烈火剑法'],['hell_fire','地狱火'],['lightning','疾光电影'],['thunder_storm','地狱雷光'],['ice_storm','冰咆哮']])assert(model.skillQuickLabel(definition({kind,name})).length<=3);
});
test('真实保存技能面板保留三行条目，模式用开启关闭、烈火用充能就绪；未同步禁止误提交',()=>{
    const file=path.resolve(__dirname,'../../assets/scripts/app/GameSkills.ts'),compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}});
    class List{}List.EVENT_ITEM_CLICK='item';class Label{}class Button{}class Toggle{}
    const module={exports:{}};new Function('require','module','exports',compiled.outputText)(id=>id==='cc'?{Label,Button,Toggle,isValid:()=>true}:id==='../ui/List'?{List}:id==='./SkillModel'?model:{},module,module.exports);
    const labels=new Map(),buttons=new Map(),nodes=new Map(),toggle={isChecked:false,setIsCheckedWithoutNotify(value){this.isChecked=value;}},texts=[],list={node:{on(){},off(){}},setItems(rows,render){
        texts.length=0;for(const row of rows){const label={string:''};render({getChildByName:name=>name==='Text'?{getComponent:()=>label}:{active:false}},row);texts.push(label.string);}
    }};
    const panel={getChildByName:()=>({getComponent:()=>list}),getChildByPath(name){if(!nodes.has(name))nodes.set(name,{active:true,getComponent(type){if(type===Toggle)return toggle;const values=type===Label?labels:buttons;if(!values.has(name))values.set(name,{});return values.get(name);}});return nodes.get(name);}};
    const controller=new module.exports.GameSkills({getChildByPath:()=>panel},{show(){}}),modes=state();
    const input={ready:true,job:0,sceneEpoch:modes.sceneEpoch,modes,modesReceivedAt:Date.now(),definitions:[definition({id:7,name:'攻杀剑术',kind:'slaying',active:false,activation_kind:'passive_proc',accuracy_by_level:[0,1,2,3],max_attack_bonus_by_level:[5,6,7,8]}),definition(),definition({id:26,name:'烈火剑法',kind:'flaming_sword',activation_kind:'charge',target_kind:'attack_charge'})],inventory:{level:40,items:[],learnedSkills:[{skillId:7,level:0},{skillId:12,level:0},{skillId:26,level:0}]}};
    controller.refresh(input);assert(texts.every(value=>value.split('\n').length<=3));assert.match(labels.get('Message').string,/攻击上限 \+5/);
    controller.selectedId=12;controller.refresh(input);assert.equal(labels.get('Use/Label').string,'关闭');assert.equal(buttons.get('Use').interactable,true);
    assert.equal(nodes.get('Use').active,false);assert.equal(nodes.get('ModeToggle').active,true);assert.equal(toggle.isChecked,true);assert.equal(toggle.interactable,true);assert.equal(labels.get('ModeToggle/Label').string,'已开启');
    toggle.isChecked=false;controller.refresh(input);assert.equal(toggle.isChecked,true);
    controller.refresh({...input,modes:null});assert.equal(buttons.get('Use').interactable,false);assert.equal(toggle.interactable,false);
    controller.selectedId=26;controller.refresh(input);assert.equal(labels.get('Use/Label').string,'充能');
    assert.equal(nodes.get('Use').active,true);assert.equal(nodes.get('ModeToggle').active,false);
    modes.states[1].ready=true;controller.refresh(input);assert.equal(labels.get('Use/Label').string,'已就绪');assert.equal(buttons.get('Use').interactable,false);
    const line={definition:definition({id:10,kind:'lightning',activation_kind:'cast',target_kind:'enemy_direction',line_cells:6,range:0}),learned:true,level:0,cost:38};
    assert.match(controller.description(line,input),/前方6格/);assert.doesNotMatch(controller.description(line,input),/射程 0格/);
    assert.match(controller.description({...line,definition:definition({target_kind:'enemy_self',activation_kind:'cast',area_radius:2})},input),/自身周围2格/);
});
test('攻杀初学只显示源常驻准确0与攻击上限5，不伪造主动或高阶收益',()=>{
    const row={definition:definition({id:7,kind:'slaying',active:false,activation_kind:'passive_proc',accuracy_by_level:[0,1,2,3],max_attack_bonus_by_level:[5,6,7,8]}),learned:true,level:0};
    assert.equal(model.skillPassiveText(row),'当前准确 +0 · 攻击上限 +5');row.level=3;assert.equal(model.skillPassiveText(row),'当前准确 +3 · 攻击上限 +8');
});
test('疾光方向梁从源六帧身体结束启动，前格命中不控制梁长；雷光不按多目标Impact重播',()=>{
    const event={sceneEpoch:'1',castId:'9',skillId:10,casterHandle:1,targetHandle:0,origin:{x:2,y:2},targetGrid:{x:3,y:2},direction:2,delayMs:500};
    const playback=new CastPlayback();playback.bind('1');
    playback.accept(event,definition({id:10,kind:'lightning',activation_kind:'cast',target_kind:'enemy_direction'}),0);
    assert.deepEqual(effectPoses(playback.casts[0],599,0,600),[]);
    assert.deepEqual(effectPoses(playback.casts[0],600,0,600).map(value=>[value.kind,value.direction,value.elapsed]),[['beam',2,0]]);
    assert.equal(playback.impact({sceneEpoch:'1',eventId:'10',castId:'9',skillId:10,targetHandle:3,targetLife:'0',outcome:1},650),false);
    playback.expire(1199,()=>0,()=>0,()=>600);assert.equal(playback.casts.length,1);
    playback.expire(1200,()=>0,()=>0,()=>600);assert.equal(playback.casts.length,0);
    playback.accept({...event,castId:'11',skillId:24},definition({id:24,kind:'thunder_storm',activation_kind:'cast',target_kind:'enemy_self'}),0);
    for(let i=0;i<3;i++)assert.equal(playback.impact({sceneEpoch:'1',eventId:String(12+i),castId:'11',skillId:24,targetHandle:3+i,targetLife:'0',outcome:1},500),false);
    assert.deepEqual(effectPoses(playback.casts[0],500,600).map(value=>value.kind),['cast']);
    playback.expire(600,()=>0,()=>600);assert.equal(playback.casts.length,0);
});
test('刀光严格匹配受理事件与实际身体攻击序号/帧，等待走步或后续攻击不抢播，重连不补旧光',()=>{
    const file=path.resolve(__dirname,'../../assets/scripts/app/EntityAnimation.ts'),compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}});
    const module={exports:{}};new Function('module','exports',compiled.outputText)(module,module.exports);
    const {EntityAnimation}=module.exports,animation=new EntityAnimation({standing:{count:4,duration_ms:500},walking:{count:6,duration_ms:100},attack:{count:6,duration_ms:100},struck:{count:3,duration_ms:100},die:{count:4,duration_ms:100}});
    animation.observe({direction:4,action:0,attackSequence:0,dead:false,injured:false},0);
    const playback=new MeleePlayback();playback.bind('1');
    const event={sceneEpoch:'1',eventId:'9007199254740993',casterHandle:1,skillId:12,skillLevel:3,casterLife:'0',attackSequence:1,direction:2};
    assert(playback.accept(event,0));assert(!playback.accept(event,1));assert(!playback.accept({...event,eventId:'9007199254740994',sceneEpoch:'2'},1));
    animation.acceptAttack(1,2);
    const sample=now=>{const pose=animation.sample(now,{moving:now<100,direction:4,stepProgress:.5});return pose.action==='attack'?{...pose,attackSequence:animation.currentAttackSequence}:null;};
    assert.deepEqual(playback.poses(0,()=>sample(0)),[]);
    assert.equal(playback.poses(100,()=>sample(100))[0].frame,0);
    animation.acceptAttack(2,4);assert.equal(playback.poses(400,()=>sample(400))[0].frame,3);
    assert.deepEqual(playback.poses(700,()=>sample(700)),[]);
    playback.bind('1');assert.deepEqual(playback.poses(701,()=>({attackSequence:1,frame:1,direction:2})),[]);
});
