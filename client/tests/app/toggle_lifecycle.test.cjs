const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const engine=process.env.MINIMIR_CREATOR_APP??'/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const toggleFile=path.join(require('../../../tools/client/creator_paths.cjs').engineRoot,'cocos/ui/toggle.ts');

// 加载锁定引擎的真实Toggle及AppRoot实现；仅外围Node事件容器、Button基类和场景依赖使用替身。
class NodeEvents{
    constructor(){this.rows=[];this.parent={getComponent(){return null;}};}
    on(name,fn,target){this.rows.push({name,fn,target});}
    off(name,fn,target){this.rows=this.rows.filter(row=>row.name!==name||row.fn!==fn||row.target!==target);}
    emit(name,...args){for(const row of this.rows.filter(row=>row.name===name))row.fn.apply(row.target,args);}
    getComponent(){return this.component;}
}
class Button{
    static EventType={CLICK:'click'};
    constructor(){this.node=new NodeEvents();this.node.component=this;this.enabled=true;}
    onEnable(){}onDisable(){}
}
const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
const decorators=new Proxy({},{get(){return decorator;}});
function load(file,requireModule){
    const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true}});
    const module={exports:{}};new Function('require','module','exports',output.outputText)(requireModule,module,module.exports);return module.exports;
}
const {Toggle}=load(toggleFile,id=>id==='cc.decorator'?decorators:id==='internal:constants'?{EDITOR_NOT_IN_PREVIEW:false}
    :id==='./button'?{Button,ButtonEventType:Button.EventType}:id==='../core/data/utils/extends-enum'?{extendsEnum:Object.assign}
    :id==='../core/global-exports'?{legacyCC:{Node:{isNode:()=>true}}}:id==='../scene-graph/component-event-handler'?{EventHandler:{emitEvents(){}}}:{});
const trace=[];
const session={lastError:''};
const {AppRoot}=load(path.resolve(__dirname,'../../assets/scripts/app/AppRoot.ts'),id=>id==='cc'
    ?{_decorator:decorators,Button,Toggle,EditBox:{EventType:{TEXT_CHANGED:'text-changed'}},Component:class{},isValid:()=>true,game:{off(){}},Game:{EVENT_HIDE:'hide'}}
    :id.endsWith('/NavigationIntent')?{NavigationIntent:class{}}
    :id==='./GameSession'?{session}
    :id==='./InputTrace'?{traceInput:record=>trace.push(record)}:{});
function setup(){const root=new AppRoot(),toggle=new Toggle();root.find=()=>toggle.node;root.error=error=>{throw error;};toggle.onEnable();return {root,toggle};}

test('真实3.8.8 Toggle常规及重新启用后，每次TOGGLE语义绑定只提交一次最终值',()=>{
    for(const restart of [false,true]){
        const {root,toggle}=setup(),submitted=[];
        root.button('MapViewport/Actions/Auto',()=>submitted.push(toggle.isChecked),Toggle.EventType.TOGGLE);
        if(restart){toggle.onDisable();toggle.onEnable();}
        toggle.node.emit(Button.EventType.CLICK);
        assert.deepEqual(submitted,[false]);assert.equal(toggle.isChecked,false);
        toggle.node.emit(Button.EventType.CLICK);
        assert.deepEqual(submitted,[false,true]);assert.equal(toggle.isChecked,true);
        root.onDestroy();toggle.node.emit(Button.EventType.CLICK);assert.deepEqual(submitted,[false,true]);
    }
});
test('真实Toggle忙碌时回到权威状态且不发请求，无通知写入不再触发业务事件',()=>{
    const {root,toggle}=setup(),submitted=[];let refreshed=0;
    root.refreshGame=()=>{refreshed++;toggle.setIsCheckedWithoutNotify(true);};
    root.button('MapViewport/Actions/Auto',()=>submitted.push(toggle.isChecked),Toggle.EventType.TOGGLE);
    root.uiActionBusy=true;toggle.node.emit(Button.EventType.CLICK);
    assert.equal(refreshed,1);assert.equal(toggle.isChecked,true);assert.deepEqual(submitted,[]);
    assert.equal(trace.at(-1).event_type,'toggle');assert.equal(trace.at(-1).blocked,true);
    root.uiActionBusy=false;toggle.setIsCheckedWithoutNotify(false);assert.deepEqual(submitted,[]);
    root.onDestroy();
});
test('普通Button仍默认CLICK，清理按注册事件类型移除',()=>{
    const {root,toggle}=setup();let clicks=0;root.button('BottomHUD/Skills/BluePotion',()=>clicks++);
    assert.equal(root.buttons[0][1],Button.EventType.CLICK);
    toggle.node.emit(Button.EventType.CLICK);assert.equal(clicks,1);
    root.onDestroy();toggle.node.emit(Button.EventType.CLICK);assert.equal(clicks,1);
});
test('真实引擎Toggle的自动回收设置回填不触发TOGGLE；普通用户点击仍触发一次',()=>{
    const Long=require('../../assets/scripts/app/../../../node_modules/long');
    const {GameAutoRecycle}=load(path.resolve(__dirname,'../../assets/scripts/app/GameAutoRecycle.ts'),id=>id==='cc'?{Toggle,Button,EditBox:class{},Label:class{}}:id==='long'?Long:id==='./AutoRecycleModel'?require('../../../.work/ui-tests/app/AutoRecycleModel.js'):{});
    const {defaultAutoRecycle}=require('../../../.work/ui-tests/app/AutoRecycleModel.js');const values=new Map();let changed=0;
    const names=['AutoRecycleEnabled','RecycleProtectBound','RecycleProtectUnbound',...[0,1,2,3,4].map(id=>'RecycleQuality'+id)];
    for(const name of names){const toggle=new Toggle();toggle.node.on(Toggle.EventType.TOGGLE,()=>changed++);toggle.onEnable();values.set(name,toggle);}
    const panel={getChildByPath(name){if(!values.has(name))values.set(name,{string:'',interactable:true});return{getComponent:()=>values.get(name)};}};
    const view=new GameAutoRecycle(panel),policy=defaultAutoRecycle();view.refresh({role:'r',ready:true,policy,definitions:[]},false,false);view.reset();assert.equal(changed,0);
    values.get('AutoRecycleEnabled').node.emit(Button.EventType.CLICK);assert.equal(changed,1);
});
test('真实技能模式Toggle绑定只发送一次最终勾选，发送与拒绝前保持权威值，状态回推不反向发送',()=>{
    for(const restart of [false,true]){
        const {root,toggle}=setup(),submitted=[],selected={definition:{id:12}},register=root.button.bind(root);let authoritative=true;
        root.skills={selected};root.castSkill=(skill,enabled)=>submitted.push({skill,enabled});
        root.refreshGame=()=>toggle.setIsCheckedWithoutNotify(authoritative);
        root.button=(path,action,eventType)=>{if(path==='UIMgr/Panels/Skills/ModeToggle')register(path,action,eventType);};
        root.bindGameButtons();if(restart){toggle.onDisable();toggle.onEnable();}
        toggle.node.emit(Button.EventType.CLICK);assert.deepEqual(submitted,[{skill:selected,enabled:false}]);assert.equal(toggle.isChecked,true);
        root.refreshGame();assert.equal(submitted.length,1);assert.equal(toggle.isChecked,true);
        authoritative=false;root.refreshGame();assert.equal(toggle.isChecked,false);assert.equal(submitted.length,1);
        root.uiActionBusy=true;toggle.node.emit(Button.EventType.CLICK);assert.equal(submitted.length,1);assert.equal(toggle.isChecked,false);
        root.uiActionBusy=false;toggle.node.emit(Button.EventType.CLICK);assert.equal(submitted.length,2);assert.equal(submitted[1].enabled,true);assert.equal(toggle.isChecked,false);
        root.onDestroy();
    }
});

test('真实退出完成后Creator清空旧组件对象字段，finally不再解锁已销毁导航',async()=>{
    const root=new AppRoot(),blocked=[];let exited=0,closed=0;
    session.ready=false;session.connection={close(){closed++;}};
    root.releaseInput=()=>{};
    root.nav={setInputBlocked:value=>blocked.push(value),exitGame(){exited++;}};
    root.changeScene=async name=>{
        assert.equal(name,'RoleSelect');
        root.disposed=true;root.nav=null;root.node=null;
        session.lastError='新场景状态';
    };
    await root.leave(false);
    assert.deepEqual(blocked,[true]);assert.equal(exited,1);assert.equal(closed,1);
    root.error(new Error('旧场景迟到错误'));root.status('旧场景迟到文字');
    assert.equal(session.lastError,'新场景状态');
});

test('退出未切换场景时仍恢复原输入，迟到按钮拒绝不会触碰已销毁节点',async()=>{
    const root=new AppRoot(),blocked=[];
    session.ready=false;session.connection={close(){}};root.releaseInput=()=>{};
    root.nav={setInputBlocked:value=>blocked.push(value),exitGame(){}};
    root.changeScene=async()=>{throw new Error('切换失败');};
    await assert.rejects(root.leave(false),/切换失败/);
    assert.deepEqual(blocked,[true,false]);assert.equal(root.leaving,false);
    const button=new NodeEvents();root.find=()=>button;
    let reject;root.button('Enter',()=>new Promise((_resolve,fail)=>{reject=fail;}));
    button.emit(Button.EventType.CLICK);
    root.disposed=true;root.node=null;session.lastError='新场景状态';
    reject(new Error('旧操作拒绝'));await new Promise(resolve=>setImmediate(resolve));
    assert.equal(session.lastError,'新场景状态');
});
