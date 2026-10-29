const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const path=require('node:path');
const creator=require('../../../tools/client/creator_paths.cjs');
const {makeWorld,cc,state,Long}=require('../support/world_runtime.cjs');
// 执行本机锁定Creator的真实NodeEventProcessor传播代码；仅节点/回调表/池和AudioSource为轻量替身，不是浏览器或硬件声音验收。
const engineFile=path.join(creator.engineRoot,'cocos/scene-graph/node-event-processor.ts');
class CallbacksInvoker{
 constructor(){this.entries=[];}on(type,callback,target){this.entries.push({type,callback,target});}off(type,callback,target){this.entries=this.entries.filter(e=>e.type!==type||e.callback!==callback||e.target!==target);this.offCallback?.();}
 hasEventListener(type,callback,target){return this.entries.some(e=>e.type===type&&(!callback||e.callback===callback)&&(!target||e.target===target));}
 emit(type,...args){for(const e of this.entries.slice())if(e.type===type)e.callback.apply(e.target,args);}
 _registerOffCallback(callback){this.offCallback=callback;}
}
const types={TOUCH_START:'touch-start',TOUCH_MOVE:'touch-move',TOUCH_END:cc.Node.EventType.TOUCH_END,TOUCH_CANCEL:'touch-cancel',MOUSE_DOWN:'mouse-down',MOUSE_UP:'mouse-up',MOUSE_MOVE:'mouse-move'};
const compiled=creator.typescript.transpileModule(fs.readFileSync(engineFile,'utf8'),{compilerOptions:{target:creator.typescript.ScriptTarget.ES2020,module:creator.typescript.ModuleKind.CommonJS}}),moduleValue={exports:{}};
new Function('require','module','exports',compiled.outputText)(id=>id.includes('callbacks-invoker')?{CallbacksInvoker}:id.endsWith('/vec2')?{Vec2:class{}}
 :id.endsWith('/global-exports')?{cclegacy:{}}:id.endsWith('/node-event')?{NodeEventType:types}:id.endsWith('/event-enum')?{InputEventType:types}
 :id==='../core'?{Pool:class{constructor(make){this.make=make;}alloc(){return this.make();}free(){}}}: {},moduleValue,moduleValue.exports);
const {NodeEventProcessor}=moduleValue.exports;
Object.defineProperty(cc.Node.prototype,'eventProcessor',{get(){return this._events??=new NodeEventProcessor(this);}});
cc.Node.prototype.on=function(...args){this.eventProcessor.on(...args);};cc.Node.prototype.off=function(...args){this.eventProcessor.off(...args);};
function clip(name,seconds){return {name,refs:0,addRef(){this.refs++;return this;},decRef(){assert(this.refs>0);this.refs--;},getDuration(){return seconds;}};}
const scope={connection:1,binding:'1',role:'1:'+'09'.repeat(16),scene:'1',baseline:'1',map:'1',travelId:''};
function fixture(){const a=clip('cast',1.136),b=clip('born',1.505),f=makeWorld(1,{audio:{summon_skeleton_cast:a,pet_skeleton_born:b}});let current=true;f.view.bindPresentation(scope,()=>current);return Object.assign(f,{clips:[a,b],setCurrent(v){current=v;}});}
function buttonTouch(root,observed){
 const button=new cc.Node('保存Button');button.parent=root;
 button.on(cc.Node.EventType.TOUCH_END,event=>{observed?.();event.propagationStopped=true;});
 // 实际UI PointerEventDispatcher认领touch后返回false，全局input不会再被调用。
 const event={type:cc.Node.EventType.TOUCH_END,bubbles:true,propagationStopped:false,propagationImmediateStopped:false};button.eventProcessor.dispatchEvent(event);assert.equal(event.propagationStopped,true);return button;
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
test('真实Creator capture先于Button停冒泡，UI认领不发全局input也能正常解锁，且不吞按钮',t=>{
 const f=fixture();t.after(()=>f.view.dispose());let clicked=0;assert.equal(f.view.audioAudit.gestureUnlocked,false);buttonTouch(f.inputRoot,()=>{clicked++;assert.equal(f.view.audioAudit.gestureUnlocked,true,'capture必须在目标按钮之前执行');});
 assert.equal(clicked,1);assert.equal(f.view.audioAudit.gestureUnlocked,true);assert.equal(f.view.audioAudit.gestureSource,'saved_ui_capture');const source=f.view.audio.voices[0].source;assert.equal(source.calls.filter(c=>c.kind==='play').length,1);assert.equal(source.calls.find(c=>c.kind==='play').volume,0);f.view.dispose();
});
test('捕获解锁与未认领地图input fallback去重，Born仍播完整1505ms原声而非1000ms图像',async t=>{
 const f=fixture();t.after(()=>f.view.dispose());buttonTouch(f.inputRoot);cc.input.touch();assert.equal(f.view.audio.voices[0].source.calls.filter(c=>c.kind==='play').length,1);
 const pet=state(3,2,8,8,{definitionId:1,life:Long.UONE,direction:5});f.apply([pet]);assert(f.view.presentPetBorn({event:{sceneEpoch:'1',mapId:'1',entityHandle:3,entityLife:'1',definitionId:1,movementRevision:'0',direction:5,sourceCastId:'1'},elapsedMs:0}));await flush();
 f.update(1000);assert.equal(f.view.audioAudit.voices[1].playing,true);f.update(1505);assert.equal(f.view.audioAudit.voices[1].playing,false);f.view.dispose();
 const fallback=fixture();cc.input.touch();assert.equal(fallback.view.audioAudit.gestureUnlocked,true);assert.equal(fallback.view.audioAudit.gestureSource,'global_input');fallback.view.dispose();
});
test('旧scope的Safe捕获不解锁；dispose用原root/callback/context/capture退订且先解绑clip',t=>{
 const f=fixture(),audio=f.view.audio;t.after(()=>f.view.dispose());f.setCurrent(false);buttonTouch(f.inputRoot);cc.input.touch();assert.equal(audio.audit.gestureUnlocked,false);assert.equal(audio.voices[0].source.calls.filter(c=>c.kind==='play').length,0);
 assert(f.inputRoot.eventProcessor.capturingTarget?.hasEventListener(cc.Node.EventType.TOUCH_END));f.view.dispose();assert.equal(f.inputRoot.eventProcessor.capturingTarget?.hasEventListener(cc.Node.EventType.TOUCH_END),false);
 buttonTouch(f.inputRoot);cc.input.touch();assert.equal(audio.audit.gestureUnlocked,false);assert(audio.voices.every(v=>v.source.destroyed&&!v.source.clip));assert(f.clips.every(c=>c.refs===0));
});
