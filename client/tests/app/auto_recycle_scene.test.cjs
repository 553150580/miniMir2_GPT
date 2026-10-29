const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const scene=JSON.parse(fs.readFileSync(path.resolve(__dirname,'../../assets/scenes/Game.scene'),'utf8'));
const deref=ref=>scene[ref.__id__];const nodeName=(parent,name)=>parent._children.map(deref).find(node=>node._name===name);
function byPath(value){let node=scene.find(item=>item?.__type__==='cc.Node'&&item._name==='Safe');for(const part of value.split('/')){node=nodeName(node,part);assert(node,part);}return node;}
const component=(node,type)=>node._components.map(deref).find(value=>value.__type__===type);
test('自动回收固定面板与默认关闭Toggle保存在Game.scene，沿用内置EditBox默认标签树',()=>{
 const settings=byPath('UIMgr/Panels/Settings');assert.equal(component(nodeName(settings,'AutoRecycleEnabled'),'cc.Toggle')._isChecked,false);
 for(const name of ['RecycleQuality0','RecycleQuality1','RecycleQuality2','RecycleQuality3','RecycleQuality4','RecycleProtectBound','RecycleProtectUnbound'])assert(component(nodeName(settings,name),'cc.Toggle'));
 for(const name of ['RecycleMaxLevel','RecycleProtectedTypes']){const input=nodeName(settings,name),edit=component(input,'cc.EditBox');assert(edit);for(const ref of [edit._textLabel,edit._placeholderLabel]){const label=deref(ref);assert.equal(label.__type__,'cc.Label');const textNode=deref(label.node);assert.equal(textNode._parent.__id__,scene.indexOf(input));assert(!component(textNode,'cc.Widget'));assert(label._fontSize>=24);}}
});
test('保存应用/仅未来/恢复与锁定入口分别存在，名称保护不暴露配置ID输入',()=>{
 for(const name of ['RecycleSave','RecycleApply','RecycleReset'])assert(component(byPath('UIMgr/Panels/Settings/'+name),'cc.Button'));
 assert(component(byPath('UIMgr/Panels/Inventory/ItemLock'),'cc.Button'));
 const edit=component(byPath('UIMgr/Panels/Settings/RecycleProtectedTypes'),'cc.EditBox');assert(edit._placeholder.includes('木剑'));assert(!edit._placeholder.includes('218'));assert(!edit._placeholder.includes('ID'));
});
