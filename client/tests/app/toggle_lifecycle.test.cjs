const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const engine=process.env.MINIMIR_CREATOR_APP??'/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const toggleFile=path.join(engine,'Contents/Resources/resources/3d/engine/cocos/ui/toggle.ts');

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
const {AppRoot}=load(path.resolve(__dirname,'../../assets/scripts/app/AppRoot.ts'),id=>id==='cc'
    ?{_decorator:decorators,Button,Toggle,Component:class{},isValid:()=>true,game:{off(){}},Game:{EVENT_HIDE:'hide'}}
    :id.endsWith('/NavigationIntent')?{NavigationIntent:class{}}
    :id==='./GameSession'?{session:{lastError:''}}
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
