const {test}=require('node:test'),assert=require('node:assert/strict');
const {actualList,cc,List}=require('./helpers/list_restore_harness.cjs');
const {load}=require('./helpers/session_harness.cjs');
const {GameMessages}=load('client/assets/scripts/app/GameMessages.ts',id=>id==='cc'?cc:id==='../ui/List'?{List}:id==='./SystemMessageLog'?require('../../../.work/ui-tests/app/SystemMessageLog.js'):{});

function fixture(){
 const v=actualList(),component=new GameMessages(),rows=Array.from({length:30},(_,id)=>({id,title:'消息',message:String(id)}));
 v.list.setItems(rows);v.scroll.isScrolling=()=>false;v.scroll.isAutoScrolling=()=>false;
 Object.assign(component,{list:v.list,scroll:v.scroll,panel:{activeInHierarchy:true},log:{filter:()=>rows.slice(0,25),messages:rows,capacity:200},search:{string:'消息'},pending:true,rendered:rows,renderedOnce:true});
 return{v,component,rows};
}

test('真实消息搜索从底部回顶部后立即显示匹配项，无需等待额外滚动事件',()=>{
 const {v,component}=fixture();v.userScroll(2200);assert(v.visible().includes(24));
 component.queryChanged=true;const emitted=v.scroll.emitted;component.flush();
 assert.equal(v.scroll.getScrollOffset().y,0);assert(v.visible().includes(0));assert.equal(v.scroll.emitted,emitted);assert.equal(v.list.itemCount,25);
 const first=v.list.rows.get(0);assert.equal(first.getChildByName('Text').getComponent(cc.Label).string,'消息\n0');
 assert(v.list.visibleNodeCount<=10);
});

test('消息淘汰后按原消息身份恢复锚点，同步重绑当前位置且不改选择数据',()=>{
 const {v,component,rows}=fixture();v.userScroll(1820);const filtered=rows.slice(18);
 component.log.filter=()=>filtered;component.log.messages=filtered;component.flush();
 assert(Math.abs(v.scroll.getScrollOffset().y-200)<1e-8);assert(v.visible().includes(2));
 const node=v.list.rows.get(2);assert.equal(node.getChildByName('Text').getComponent(cc.Label).string,'消息\n20');
 assert.equal(component.rendered[2].id,20);assert(v.list.visibleNodeCount<=10);
});
