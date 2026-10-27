const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const root=path.resolve(__dirname,'../../..');
const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
const decorators=new Proxy({},{get(){return decorator;}});
class Vec2{constructor(x=0,y=0){this.x=x;this.y=y;}}
class UITransform{}
class Label{}
class Button{static EventType={CLICK:'click'};}
class Events{on(){}off(){}}
function load(file,requireModule){
    const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true}});
    const module={exports:{}};new Function('require','module','exports',compiled.outputText)(requireModule,module,module.exports);return module.exports;
}

// 采用真实List及锁定3.8.8的偏移读取方法；滚动写入替身只记录公开scrollToOffset目标并更新边界。
const engine=process.env.MINIMIR_CREATOR_APP??'/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const scrollSource=ts.createSourceFile('scroll-view.ts',fs.readFileSync(path.join(engine,'Contents/Resources/resources/3d/engine/cocos/ui/scroll-view.ts'),'utf8'),ts.ScriptTarget.Latest,true);
const scrollClass=scrollSource.statements.find(node=>ts.isClassDeclaration(node)&&node.name.text==='ScrollView');
const methods=['getScrollOffset','_getContentTopBoundary','_getContentBottomBoundary'].map(name=>scrollClass.members.find(node=>node.name?.getText(scrollSource)===name).getText(scrollSource));
const scrollCompiled=ts.transpileModule(`class ReadOffsets{${methods.join('\n')}};exports.ReadOffsets=ReadOffsets;`,{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}});
const scrollModule={exports:{}};new Function('exports','Vec2',scrollCompiled.outputText)(scrollModule.exports,Vec2);
const {List}=load(path.join(root,'client/assets/scripts/ui/List.ts'),id=>id==='cc'?{_decorator:decorators,Component:class{},Button,UITransform,Vec2,
    isValid:value=>value!=null,Node:{EventType:{SIZE_CHANGED:'size'}},ScrollView:{EventType:{SCROLLING:'scrolling',SCROLL_ENDED:'ended'}}}
    :id==='./VirtualListWindow'?require('../../../.work/ui-tests/ui/VirtualListWindow.js'):{});

function actualList(){
    const view={width:600,height:400},content={width:600,height:400,anchorY:1,setContentSize(width,height){this.width=width;this.height=height;}};
    const contentNode={position:{x:0,y:200},getComponent:()=>content,_getUITransformComp:()=>content};
    const scroll=new scrollModule.exports.ReadOffsets();
    Object.assign(scroll,{_content:contentNode,content:contentNode,node:new Events(),_topBoundary:0,_leftBoundary:0,stops:0,writes:[],
        _getContentPosition:()=>contentNode.position,_getContentLeftBoundary:()=>0,
        stopAutoScroll(){this.stops++;},scrollToOffset(offset){this.writes.push(offset.y);this._topBoundary=view.height/2;contentNode.position.y=this._topBoundary+offset.y;}});
    const list=new List();list.node=new Events();list.scrollView=scroll;list.viewport=Object.assign(new Events(),{getComponent:()=>view});list.rowHeight=104;
    list.refreshVisible=()=>{};
    return {list,scroll,content};
}
test('隐藏首次绑定5×104行时抛弃未初始化边界产生的200偏移，首行从顶端开始',()=>{
    const {list,scroll,content}=actualList();assert.equal(scroll.getScrollOffset().y,200);
    list.setItems([1,2,3,4,5]);assert.equal(content.height,520);assert.deepEqual(scroll.writes,[0]);assert.equal(scroll.getScrollOffset().y,0);
    list.onEnable();assert.equal(scroll.getScrollOffset().y,0);
});
test('列表初始化后同尺寸刷新不打断滚动，数据扩展保留真实偏移，显式新内容回顶',()=>{
    const {list,scroll}=actualList();list.setItems([1,2,3,4,5]);scroll.scrollToOffset(new Vec2(0,75));const stopped=scroll.stops;
    list.setItems([1,2,3,4,5],undefined,true);assert.equal(scroll.stops,stopped);assert.equal(scroll.getScrollOffset().y,75);
    list.setItems([1,2,3,4,5,6],undefined,true);assert.equal(scroll.getScrollOffset().y,75);
    list.setItems([1,2,3,4,5],undefined,false);assert.equal(scroll.getScrollOffset().y,0);
});

class SavedList{
    static EVENT_ITEM_CLICK='list-item-click';
    constructor(){this.node=new Events();this.offset=0;this.calls=[];}
    setItems(items,renderer,preserve){this.calls.push({items,preserve});if(!preserve)this.offset=0;}
}
class SavedNode{
    constructor(list){this.list=list;this.label={string:''};this.button={interactable:true};this.active=true;}
    getComponent(type){return type===SavedList?this.list:type===Label?this.label:this.button;}
}
function controller(){
    const offers=new SavedList(),materials=new SavedList(),quote=new SavedList(),nodes=new Map();
    const panel={getChildByPath(name){if(!nodes.has(name))nodes.set(name,new SavedNode(name==='OffersList'?offers:name==='MaterialsList'?materials:null));return nodes.get(name);}};
    const popup={confirm(){return new Promise(resolve=>{this.resolve=resolve;});}};
    const safe={getChildByPath(name){return name==='UIMgr/Panels/SkillAssets'?panel:name==='PopupView/Panels/SkillQuote/List'?new SavedNode(quote):null;}};
    const {GameSkillAssets}=load(path.join(root,'client/assets/scripts/app/GameSkillAssets.ts'),id=>id==='cc'?{Button,Label,isValid:value=>value!=null}
        :id==='../ui/List'?{List:SavedList}:id==='./SkillAssetsModel'?require('../../../.work/ui-tests/app/SkillAssetsModel.js'):id==='./InputTrace'?{traceInput(){}}:{});
    const assets=new GameSkillAssets(safe,popup,error=>{throw error;});
    const input={ready:true,roleIdentity:'1:role',sceneEpoch:'1',ruleVersion:'4',inventory:{revision:'1',level:7,items:[],learnedSkills:[]},
        wallet:{revision:'1',gold:'1000',boundGold:'1000',yuanbaoUnits:'1000',boundYuanbaoUnits:'1000'},definitions:[],skills:[],upgrades:[],recycle:[],
        offers:Array.from({length:5},(_,i)=>({offer_id:i+1,item_id:i+1,name:`商品${i+1}`,quantity:1,currency_kind:'gold',price_units:500,required_role_level:1,output_bound:true,max_purchase_quantity:32}))};
    assets.refresh(input);return {assets,input,offers,materials,quote,popup};
}
test('真实资产控制器新模式回顶，同模式报价取消保留滚动、原SKU及数量',async()=>{
    const {assets,input,offers,materials,popup}=controller();
    assets.open(1);assert.equal(offers.calls.at(-1).preserve,false);assert.equal(offers.offset,0);
    assets.selectOffer(0,input.offers[0]);for(let i=0;i<4;i++)assets.adjustPurchase(1);offers.offset=120;
    assets.lock(true);
    const confirming=assets.showQuote({kind:1,fromLevel:0,toLevel:0,spend:input.wallet,materials:[],outputs:[],stats:[]});popup.resolve(false);assert.equal(await confirming,false);
    assets.lock(false);assert.equal(offers.calls.at(-1).preserve,true);assert.equal(offers.offset,120);
    assert.equal(assets.prepare().request.offerId,1);assert.equal(assets.prepare().request.quantity,5n);
    materials.offset=150;assets.open(3);assert.equal(materials.calls.at(-1).preserve,false);assert.equal(materials.offset,0);
    offers.offset=120;assets.open(1);assert.equal(offers.calls.at(-1).preserve,false);assert.equal(offers.offset,0);
});
