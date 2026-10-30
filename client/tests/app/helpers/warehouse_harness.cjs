const {Long,load,root}=require('./session_harness.cjs');
const path=require('node:path');
class WidgetNode{
 static EventType={NODE_DESTROYED:'destroyed'};
 constructor(name=''){this.name=name;this.active=true;this.isValid=true;this.controls=new Map();this.events=new Map();this.components=new Map();}
 on(name,fn,context){const rows=this.events.get(name)||[];rows.push({fn,context});this.events.set(name,rows);}
 off(name,fn,context){if(!this.isValid)throw Error('已销毁节点不能off');this.events.set(name,(this.events.get(name)||[]).filter(x=>x.fn!==fn||x.context!==context));}
 emit(name,...args){for(const x of this.events.get(name)||[])x.fn.apply(x.context,args);}
 getChildByPath(name){return this.controls.get(name)||null;}
 getChildByName(name){return this.getChildByPath(name);}
 getComponent(kind){return this.components.get(kind)||null;}
}
class List{
 static EVENT_ITEM_CLICK='list-item-click';
 constructor(node){this.node=node;this.items=[];this.binds=0;this.offset=0;this.scrollView={getScrollOffset:()=>({y:this.offset}),scrollToOffset:value=>{this.offset=value.y;}};}
 setItems(items,renderer,preserve){this.items=items;this.renderer=renderer;this.binds++;if(!preserve)this.offset=0;}
 setRenderer(renderer){this.renderer=renderer;}
 restoreScrollOffset(offset){this.offset=offset;}
}
const cc={Node:WidgetNode,List,Button:'button',EditBox:'edit',Label:'label',Vec2:class{constructor(x,y){this.x=x;this.y=y;}},isValid:value=>Boolean(value&&value.isValid!==false)};
function view(current,query){
 const safe=new WidgetNode('Safe'),panel=new WidgetNode('Warehouse');safe.controls.set('UIMgr/Panels/Warehouse',panel);
 for(const name of ['Personal','Shared','Refresh','Previous','Next','Gold','Yuanbao','DepositItem','WithdrawItem','DepositMoney','WithdrawMoney','Expand','Details']){const n=new WidgetNode(name);n.components.set(cc.Button,{interactable:true});panel.controls.set(name,n);}
 for(const name of ['Personal/Label','Shared/Label','Gold/Label','Yuanbao/Label','Message','Page']){const n=new WidgetNode(name);n.components.set(cc.Label,{string:''});panel.controls.set(name,n);}
 for(const name of ['Quantity','Amount']){const n=new WidgetNode(name);n.components.set(cc.EditBox,{string:name==='Quantity'?'1':'',enabled:true});panel.controls.set(name,n);}
 for(const name of ['AmountLabel','PersonalHint'])panel.controls.set(name,new WidgetNode(name));
 for(const name of ['BagList','WarehouseList']){const n=new WidgetNode(name),list=new List(n);n.components.set(List,list);panel.controls.set(name,n);}
 const {GameWarehouse}=load('client/assets/scripts/app/GameWarehouse.ts',id=>id==='cc'?cc:id==='long'?Long:id==='../ui/List'?{List}:id==='../generated/protocol_ids'?require(path.join(root,'.work/ui-tests/generated/protocol_ids.js')):id==='./SkillAssetsModel'?require(path.join(root,'.work/ui-tests/app/SkillAssetsModel.js')):{});
 const component=new GameWarehouse(safe,current,query);
 return{component,safe,panel,bag:panel.controls.get('BagList').getComponent(List),store:panel.controls.get('WarehouseList').getComponent(List),label:name=>panel.controls.get(name).getComponent(cc.Label),edit:name=>panel.controls.get(name).getComponent(cc.EditBox),button:name=>panel.controls.get(name).getComponent(cc.Button)};
}
module.exports={cc,view,WidgetNode,List};
