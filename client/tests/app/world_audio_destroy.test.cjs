const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const path=require('node:path');
const creator=require('../../../tools/client/creator_paths.cjs');const {load}=require('./helpers/session_harness.cjs');
const ts=creator.typescript,engine=path.join(creator.engineRoot,'cocos');
// 执行锁定引擎的销毁方法和事件处理器；仅节点数据、声卡与回调表替换，不代表浏览器或硬件声音验收。
function compile(text,env={}){const result=ts.transpileModule(text,{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),module={exports:{}};new Function('module','exports',...Object.keys(env),result.outputText)(module,module.exports,...Object.values(env));return module.exports;}
function source(file){return ts.createSourceFile(file,fs.readFileSync(path.join(engine,file),'utf8'),ts.ScriptTarget.Latest,true);}
function declaration(file,name){return source(file).statements.find(node=>node.name?.text===name);}
function method(file,klass,name,env){const node=declaration(file,klass).members.find(member=>member.name?.text===name);assert(node,`${file}:${klass}.${name}`);return compile(`class Extracted {${node.getText()}}\nexports.value=Extracted.prototype.${name};`,env).value;}
const flags=compile(declaration('core/data/object.ts','CCObjectFlags').getText()).CCObjectFlags;
const isValid=compile(declaration('core/data/object.ts','isValid').getText(),{CCObjectFlags:flags}).isValid;
const types={NODE_DESTROYED:'node-destroyed',TOUCH_END:'touch-end',PARENT_CHANGED:'parent',CHILD_REMOVED:'child-removed',COMPONENT_REMOVED:'component-removed'};
class CallbacksInvoker{
 constructor(){this.entries=[];}on(type,callback,target){this.entries.push({type,callback,target});}off(type,callback,target){this.entries=this.entries.filter(e=>e.type!==type||e.callback!==callback||e.target!==target);this.offCallback?.();}
 hasEventListener(type,callback,target){return this.entries.some(e=>e.type===type&&(!callback||e.callback===callback)&&(!target||e.target===target));}
 emit(type,...args){for(const e of this.entries.slice())if(e.type===type&&this.entries.includes(e))e.callback.apply(e.target,args);}clear(){this.entries.length=0;}_registerOffCallback(callback){this.offCallback=callback;}
}
const processorCode=ts.transpileModule(fs.readFileSync(path.join(engine,'scene-graph/node-event-processor.ts'),'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),processorModule={exports:{}};
new Function('require','module','exports',processorCode.outputText)(id=>id.includes('callbacks-invoker')?{CallbacksInvoker}:id.endsWith('/vec2')?{Vec2:class{}}
 :id.endsWith('/global-exports')?{cclegacy:{}}:id.endsWith('/node-event')?{NodeEventType:types}:id.endsWith('/event-enum')?{InputEventType:types}
 :id==='../core'?{Pool:class{constructor(make){this.make=make;}alloc(){return this.make();}free(){}}}: {},processorModule,processorModule.exports);
const {NodeEventProcessor}=processorModule.exports;
function fixture(){
 const queued=[],history=[],listeners=new Set(),allSources=[];let now=0,current=true;
 const fail=id=>{throw new Error(`引擎销毁警告/错误${id}`);};
 const immediate=method('core/data/object.ts','CCObject','_destroyImmediate',{CCObjectFlags:flags,errorID:fail,EDITOR_NOT_IN_PREVIEW:false,JSB:false});
 const destroy=method('core/data/object.ts','CCObject','destroy',{CCObjectFlags:flags,warnID:fail,objectsToDestroy:queued,EDITOR_NOT_IN_PREVIEW:false,JSB:false});
 const preNode=method('scene-graph/node.ts','Node','_onPreDestroyBase',{Destroying:flags.Destroying,EDITOR:false,cclegacy:{},NodeEventType:types});
 const offNode=method('scene-graph/node.ts','Node','off',{TRANSFORM_CHANGED:'transform',ACTIVE_CHANGED:'active',TRANSFORM_ON:0,ACTIVE_ON:0});
 const emitNode=method('scene-graph/node.ts','Node','emit',{});
 const removeComponent=method('scene-graph/node.ts','Node','_removeComponent',{Destroying:flags.Destroying,EDITOR:false,NodeEventType:types,errorID:fail});
 const preComponent=method('scene-graph/component.ts','Component','_onPreDestroy',{legacyCC:{director:{_nodeActivator:{destroyComp(comp){comp.onDestroy?.();}}}}});
 const audioOnDestroy=method('audio/audio-source.ts','AudioSource','onDestroy',{});
 const legacyCC={};const compileDestruct=compile(`${declaration('core/data/object.ts','compileDestruct').getText()}\nexports.value=compileDestruct;`,{legacyCC,CCClass:{_isCCClass:()=>false},SUPPORT_JIT:false}).value;
 const destruct=method('core/data/object.ts','CCObject','_destruct',{compileDestruct,js:{value(ctor,key,value){Object.defineProperty(ctor,key,{value,configurable:true});}}});
 class ObjectBase{constructor(){this._objFlags=0;}destroy(){return destroy.call(this);}_destroyImmediate(){return immediate.call(this);}_destruct(){history.push(`destruct:${this.name??'component'}`);destruct.call(this);}}
 class Node extends ObjectBase{
  static EventType=types;constructor(name){super();this.name=name;this._children=[];this._components=[];this._eventProcessor=new NodeEventProcessor(this);}
  get parent(){return this._parent;}set parent(value){this._parent=value;value?._children.push(this);}get children(){return this._children;}
  on(...args){this._eventProcessor.on(...args);}off(...args){history.push(`off:${this.name}`);return offNode.apply(this,args);}emit(...args){return emitNode.apply(this,args);}
  addComponent(Type){const value=new Type();value.node=this;this._components.push(value);return value;}_updateSiblingIndex(){}
  _onPreDestroy(){return preNode.call(this);}_removeComponent(comp){return removeComponent.call(this,comp);}
 }
 class AudioSource extends ObjectBase{
  constructor(){super();this._clip=null;this.playing=false;this.calls=[];allSources.push(this);}
  get clip(){assert(isValid(this),'已毁AudioSource不能读clip');return this._clip;}set clip(value){assert(isValid(this),'已毁AudioSource不能写clip');this._clip=value;}
  play(){assert(isValid(this),'已毁AudioSource不能play');this.calls.push('play');this.playing=true;}
  stop(){assert(isValid(this),'已毁AudioSource不能stop');this.calls.push('stop');this.playing=false;}
  unscheduleAllCallbacks(){}_onPreDestroy(){preComponent.call(this);}onDestroy(){history.push('audio:onDestroy');audioOnDestroy.call(this);}
 }
 Object.assign(legacyCC,{Node,Component:AudioSource});
 const input={on(type,callback,owner){listeners.add({type,callback,owner});},off(type,callback,owner){for(const value of listeners)if(value.type===type&&value.callback===callback&&value.owner===owner)listeners.delete(value);},touch(){for(const value of Array.from(listeners))value.callback.call(value.owner);}};
 const cc={Node,AudioSource,isValid,input,Input:{EventType:{TOUCH_END:types.TOUCH_END}}};
 const {WorldAssets}=load('client/assets/scripts/app/WorldAssets.ts',id=>id==='cc'?{}:{});
 const assets=new WorldAssets();const clips=[1.136,1.505].map((seconds,index)=>({name:`clip${index}`,refs:0,addRef(){this.refs++;return this;},decRef(){assert(!sources().some(source=>isValid(source)&&source.clip===this),'归还AudioClip前必须解绑所有活声道');this.refs--;},getDuration(){return seconds;}}));
 assets.audioClips.set('cast',assets.own(clips[0]));assets.audioClips.set('born',assets.own(clips[1]));
 const canvas=new Node('Canvas'),safe=new Node('Safe'),world=new Node('World'),effects=new Node('Effects');safe.parent=canvas;world.parent=safe;effects.parent=world;
 const {WorldAudio}=load('client/assets/scripts/app/WorldAudio.ts',id=>{assert.equal(id,'cc');return cc;});
 const audio=new WorldAudio(assets,effects,safe,()=>now,()=>current);const sources=()=>allSources.slice();
 const drain=()=>{for(const value of queued.splice(0))if(!(value._objFlags&flags.Destroyed))value._destroyImmediate();};
 return{audio,assets,clips,canvas,safe,effects,Node,AudioSource,history,listeners,sources,drain,input,now(value){now=value;},current(value){current=value;},
  destroyScene(){const app=canvas.addComponent(class extends ObjectBase{unscheduleAllCallbacks(){}_onPreDestroy(){preComponent.call(this);}onDestroy(){history.push('app:onDestroy');audio.dispose();assets.dispose();}});canvas._destroyImmediate();drain();return app;}};
}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
test('真实引擎子树先于AppRoot组件销毁：Safe自清监听，global主动退订，AudioSource与clip只归还一次',async()=>{
 const f=fixture();f.input.touch();f.audio.play(1,'born',()=>true,1000);await flush();const oldProcessor=f.safe._eventProcessor;assert.equal(f.audio.audit.voices[1].playing,true);
 f.destroyScene();assert.equal(f.safe._eventProcessor,null);assert.equal(oldProcessor.capturingTarget.hasEventListener(types.TOUCH_END),false);assert.equal(oldProcessor.bubblingTarget.hasEventListener(types.NODE_DESTROYED),false);
 assert.equal(f.listeners.size,0);assert(f.clips.every(clip=>clip.refs===0));assert(f.sources().every(source=>!isValid(source)&&source._clip===null));
 assert(f.history.indexOf('audio:onDestroy')<f.history.indexOf('app:onDestroy'));assert(!f.history.includes('off:Safe'),'NODE_DESTROYED后由引擎清监听，不再调用root.off');
 f.audio.dispose();assert.equal(f.audio.audit.voices[1].playing,false);
});
test('strict isValid不包含Destroying；公开销毁信号在处理器与子树之前完成音频退休',()=>{
 const f=fixture();let observed=false;f.safe.on(types.NODE_DESTROYED,()=>{observed=true;assert.equal(isValid(f.safe,true),true);assert(f.safe._objFlags&flags.Destroying);assert.equal(f.listeners.size,0);assert(f.sources().every(source=>isValid(source)&&source.clip===null));});
 f.safe._destroyImmediate();assert(observed);f.audio.dispose();f.assets.dispose();f.drain();assert.equal(f.listeners.size,0);
});
test('排队ToDestroy声道仍须清clip，已毁声道不再stop或重复destroy，异步旧播放不复活',async()=>{
 const f=fixture();f.input.touch();const source=f.sources()[1];f.audio.play(1,'born',()=>true,1000);source.destroy();assert.equal(isValid(source,true),false);assert.equal(isValid(source),true);
 await flush();assert.equal(source.calls.filter(value=>value==='play').length,0);f.audio.dispose();assert(f.sources().every(source=>source.clip===null));f.drain();f.audio.dispose();f.assets.dispose();assert.equal(f.listeners.size,0);
 const g=fixture();g.effects._destroyImmediate();g.audio.dispose();g.assets.dispose();assert.equal(g.listeners.size,0);g.drain();
});
test('同保存Safe的新声道不被旧实例重复dispose退订，完整原WAV与资源owner仍独立',async()=>{
 const f=fixture();f.input.touch();f.audio.play(1,'born',()=>true,1000);f.audio.dispose();f.drain();const oldSources=f.sources();const {WorldAudio}=load('client/assets/scripts/app/WorldAudio.ts',id=>({Node:f.Node,AudioSource:f.AudioSource,isValid,input:f.input,Input:{EventType:{TOUCH_END:types.TOUCH_END}}}));
 const next=new WorldAudio(f.assets,f.effects,f.safe,()=>0,()=>true);f.audio.dispose();f.input.touch();assert.equal(next.audit.gestureUnlocked,true);next.play(1,'born',()=>true,1000);await flush();assert.equal(next.audit.voices[1].playing,true);
 next.update();assert.equal(next.audit.voices[1].playing,true);assert(oldSources.every(source=>!isValid(source)));next.dispose();f.drain();f.assets.dispose();assert.equal(f.listeners.size,0);
});
test('Safe排队销毁时仍同步解绑clip与global，剩余root监听由真实事件处理器自行清空',()=>{
 const f=fixture(),processor=f.safe._eventProcessor;f.safe.destroy();assert.equal(isValid(f.safe,true),false);f.audio.dispose();assert.equal(f.listeners.size,0);assert(f.sources().every(source=>source.clip===null));
 assert(processor.capturingTarget.hasEventListener(types.TOUCH_END));f.assets.dispose();f.drain();assert.equal(processor.capturingTarget.hasEventListener(types.TOUCH_END),false);assert.equal(processor.bubblingTarget.hasEventListener(types.NODE_DESTROYED),false);
});
