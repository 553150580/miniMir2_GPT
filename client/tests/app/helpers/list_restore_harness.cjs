// 真实List/仓库控制器；只有节点、绘制和物理内容位移使用轻量替身，不替换可见池算法。
const fs=require('node:fs'),path=require('node:path');
const {load,Long,root}=require('./session_harness.cjs');
const ts=require(path.join(root,'tools/client/creator_paths.cjs')).typescript;
class Vec2{constructor(x=0,y=0){this.x=x;this.y=y;}}
class Label{} class EditBox{} class Mask{} class Button{static EventType={CLICK:'click'};}
class UITransform{constructor(width=290,height=90){this.width=width;this.height=height;this.anchorX=.5;this.anchorY=1;}get contentSize(){return{width:this.width,height:this.height};}setContentSize(width,height){this.width=width;this.height=height;}}
class SavedNode{
 static EventType={NODE_DESTROYED:'destroyed',SIZE_CHANGED:'size'};
 constructor(name=''){this.name=name;this.active=true;this.isValid=true;this.controls=new Map();this.components=new Map();this.events=new Map();this.position={x:0,y:0,z:0};}
 on(name,fn,context){const rows=this.events.get(name)||[];rows.push({fn,context});this.events.set(name,rows);}
 off(name,fn,context){this.events.set(name,(this.events.get(name)||[]).filter(x=>x.fn!==fn||x.context!==context));}
 emit(name,...args){for(const row of this.events.get(name)||[])row.fn.apply(row.context,args);}
 getComponent(type){return this.components.get(type)||null;}
 getChildByPath(name){return this.controls.get(name)||null;}
 getChildByName(name){return this.getChildByPath(name);}
 _getUITransformComp(){return this.getComponent(UITransform);}
 setParent(parent){this.parent=parent;}
 setPosition(x,y,z=0){this.position={x,y,z};}
 destroy(){this.isValid=false;this.active=false;this.events.clear();}
}
const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
const decorators=new Proxy({},{get(){return decorator;}});
const enginePath=path.join(require(path.join(root,'tools/client/creator_paths.cjs')).engineRoot,'cocos/ui/scroll-view.ts');
const engineSource=fs.readFileSync(enginePath,'utf8'),ast=ts.createSourceFile(enginePath,engineSource,ts.ScriptTarget.Latest,true);
const klass=ast.statements.find(n=>ts.isClassDeclaration(n)&&n.name.text==='ScrollView');
const method=name=>klass.members.find(n=>n.name?.getText(ast)===name).getText(ast);
const names=['scrollToTop','scrollToOffset','scrollTo','_doScroll','getScrollOffset','getMaxScrollOffset','_getContentTopBoundary','_getContentBottomBoundary'];
const compiled=ts.transpileModule('class EngineScroll{'+names.map(method).join('\n')+'};exports.EngineScroll=EngineScroll;',{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}).outputText;
const module_={exports:{}},options={};
new Function('exports','Vec2','v2','assignMoveDeltaOption','_moveDeltaOptions',compiled)(module_.exports,Vec2,()=>new Vec2(),(x,y)=>Object.assign(options,{x,y}),options);
const EngineScroll=module_.exports.EngineScroll;EngineScroll.EventType={SCROLLING:'scrolling',SCROLL_ENDED:'ended'};
const cc={_decorator:decorators,Component:class{},Button,EditBox,Mask,Label,UITransform,Vec2,Node:SavedNode,ScrollView:EngineScroll,
 isValid:value=>Boolean(value&&value.isValid!==false),warn:()=>{},instantiate:()=>{const n=new SavedNode('Row'),text=new SavedNode('Text');n.components.set(Button,{});n.components.set(UITransform,new UITransform());text.components.set(Label,{string:''});n.controls.set('Text',text);return n;}};
const {List}=load('client/assets/scripts/ui/List.ts',id=>id==='cc'?cc:id==='./VirtualListWindow'?require(path.join(root,'.work/ui-tests/ui/VirtualListWindow.js')):{});
function actualList(name='List'){
 const node=new SavedNode(name),viewport=new SavedNode('View'),content=new SavedNode('Content'),template=new SavedNode('ItemTemplate');
 const view=new UITransform(290,280),size=new UITransform(290,280);viewport.components.set(UITransform,view);viewport.components.set(Mask,{});content.components.set(UITransform,size);content.setParent(viewport);node.controls.set('ItemTemplate',template);
 const scroll=new EngineScroll();Object.assign(scroll,{node,content,_content:content,view,_topBoundary:140,_leftBoundary:0,writes:[],stops:0,emitted:0,
  _getContentPosition:()=>content.position,_getContentLeftBoundary:()=>0,stopAutoScroll(){this.stops++;},
  _calculateMovePercentDelta(q){return{y:(1-q.y)*this.getMaxScrollOffset().y-this.getScrollOffset().y};},
  // 引擎0时长经_doScroll直接_moveContent；此处只替换物理坐标写入，不主动发scroll事件。
  _moveContent(delta){content.position.y+=delta.y;this.writes.push(this.getScrollOffset().y);}});content.position.y=140;
 const list=new List();Object.assign(list,{node,scrollView:scroll,viewport,templateNode:template,rowHeight:90,overscan:2});node.components.set(List,list);list.onLoad();list.onEnable();
 return{list,node,scroll,content,view,visible(){return Array.from(list.rows).filter(([,row])=>{const top=content.position.y+row.position.y,bottom=top-90;return row.active&&top>-132&&bottom<132;}).map(([index])=>index);},userScroll(offset){scroll.scrollToOffset(new Vec2(0,offset),0);scroll.emitted++;node.emit(EngineScroll.EventType.SCROLLING);}};
}
function warehouse(current,query){
 const safe=new SavedNode('Safe'),panel=new SavedNode('Warehouse');safe.controls.set('UIMgr/Panels/Warehouse',panel);
 for(const name of['Personal','Shared','Refresh','Previous','Next','Gold','Yuanbao','DepositItem','WithdrawItem','DepositMoney','WithdrawMoney','Expand','Details']){const n=new SavedNode(name);n.components.set(Button,{interactable:true});panel.controls.set(name,n);}
 for(const name of['Personal/Label','Shared/Label','Gold/Label','Yuanbao/Label','Message','Page']){const n=new SavedNode(name);n.components.set(Label,{string:''});panel.controls.set(name,n);}
 for(const name of['Quantity','Amount']){const n=new SavedNode(name);n.components.set(EditBox,{string:name==='Quantity'?'1':'',enabled:true});panel.controls.set(name,n);}
 for(const name of['AmountLabel','PersonalHint'])panel.controls.set(name,new SavedNode(name));
 const bag=actualList('BagList'),store=actualList('WarehouseList');panel.controls.set('BagList',bag.node);panel.controls.set('WarehouseList',store.node);
 const {GameWarehouse}=load('client/assets/scripts/app/GameWarehouse.ts',id=>id==='cc'?cc:id==='long'?Long:id==='../ui/List'?{List}:id==='../generated/protocol_ids'?require(path.join(root,'.work/ui-tests/generated/protocol_ids.js')):id==='./SkillAssetsModel'?require(path.join(root,'.work/ui-tests/app/SkillAssetsModel.js')):{});
 const component=new GameWarehouse(safe,current,query);return{component,bag,store,panel};
}
module.exports={actualList,warehouse,method,enginePath,Vec2,Button,Long,List,cc};
