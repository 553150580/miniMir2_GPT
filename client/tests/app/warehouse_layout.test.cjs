const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),crypto=require('node:crypto');
const scene=JSON.parse(fs.readFileSync('client/assets/scenes/Game.scene','utf8')),nodes=new Map();
function walk(index,prefix=''){const n=scene[index],key=prefix?prefix+'/'+n._name:n._name;assert(!nodes.has(key));nodes.set(key,index);for(const c of n._children||[])walk(c.__id__,key);}
walk(scene[0].scene.__id__);const safe=Array.from(nodes.keys()).find(k=>k.endsWith('/Canvas/Safe'));
const index=name=>{const key=safe+'/'+name;assert(nodes.has(key),key);return nodes.get(key);},node=name=>scene[index(name)];
const component=(n,type)=>{const c=n._components.map(r=>scene[r.__id__]).find(c=>c.__type__===type);assert(c,type+' '+n._name);return c;},size=n=>component(n,'cc.UITransform')._contentSize;
const base='UIMgr/Panels/Warehouse/',bounds=n=>({l:n._lpos.x-size(n).width/2,r:n._lpos.x+size(n).width/2,b:n._lpos.y-size(n).height/2,t:n._lpos.y+size(n).height/2});
const overlap=(a,b)=>a.l<b.r&&a.r>b.l&&a.b<b.t&&a.t>b.b;
test('仓库只追加保存子树与Functions入口；原1191对象只父children各追加一次，原地图/HUD/ItemLock全部字节语义保持',()=>{
 assert.equal(scene.length,1373);const projected=structuredClone(scene.slice(0,1191));
 for(const parent of [253,395]){assert.equal(scene[parent]._children.filter(x=>x.__id__>=1191).length,1);projected[parent]._children=projected[parent]._children.filter(x=>x.__id__<1191);}
 assert.equal(crypto.createHash('sha256').update(JSON.stringify(projected)).digest('hex'),'373823584fe100fa7d5e53150f5660cb242e87ba7055d7a75e54ca34e6669675');
 assert.equal(index('UIMgr/Panels/Warehouse'),1191);assert(index('UIMgr/Panels/Functions/Pages/Page0/Warehouse')>=1191);assert.equal(component(node('UIMgr/Panels/Inventory/List/View'),'cc.UITransform')._contentSize.height,414);
 const ids=scene.filter(x=>x.__type__==='cc.Node').map(x=>x._id);assert.equal(new Set(ids).size,ids.length);
});
test('两列仓库列表使用保存ScrollView/Mask/动态模板，真View避开数量和固定按钮，不生成运行时布局',()=>{
 const views=[];
 for(const name of ['BagList','WarehouseList']){const list=node(base+name),scroll=component(list,'cc.ScrollView'),custom=list._components.map(x=>scene[x.__id__]).find(x=>'rowHeight'in x);assert.equal(custom.rowHeight,90);assert.equal(custom.overscan,2);const view=node(base+name+'/View');assert.equal(custom.viewport.__id__,index(base+name+'/View'));assert.equal(component(view,'cc.Mask')._type,0);assert.deepEqual(size(view),{__type__:'cc.Size',width:290,height:280});assert.equal(scroll._content.__id__,index(base+name+'/View/Content'));const template=node(base+name+'/View/Content/ItemTemplate');assert.equal(template._active,false);assert(component(template,'cc.Button'));assert.equal(component(node(base+name+'/View/Content/ItemTemplate/Text'),'cc.Label')._fontSize,24);views.push(bounds(list));}
 assert(!overlap(views[0],views[1]));for(const name of ['Quantity','Details','DepositItem','WithdrawItem','Previous','Next','Refresh','Expand','Gold','Yuanbao','Amount','DepositMoney','WithdrawMoney'])for(const view of views)assert(!overlap(view,bounds(node(base+name))),'View覆盖固定控件 '+name);
 for(const x of scene.slice(1191))if(x.__type__==='cc.Label')assert(x._fontSize>=24);
});
test('数量/金额沿内置EditBox默认子树，个人与共享两种显示组合没有固定按钮互相遮挡',()=>{
 for(const name of ['Quantity','Amount']){const n=node(base+name),edit=component(n,'cc.EditBox');assert.deepEqual(edit.textChanged,[]);assert.equal(edit._maxLength,20);for(const child of ['TEXT_LABEL','PLACEHOLDER_LABEL']){const t=node(base+name+'/'+child);assert.equal(t._components.filter(x=>scene[x.__id__].__type__==='cc.Widget').length,0);assert.equal(component(t,'cc.Label')._fontSize,26);}}
 const shared=['Personal','Shared','Quantity','Details','DepositItem','WithdrawItem','Previous','Next','Gold','Yuanbao','Amount','Refresh','DepositMoney','WithdrawMoney'];const personal=shared.filter(n=>!['Gold','Yuanbao','Amount','DepositMoney','WithdrawMoney'].includes(n)).concat('Expand');
 for(const names of [shared,personal])for(let i=0;i<names.length;i++)for(let j=i+1;j<names.length;j++)assert(!overlap(bounds(node(base+names[i])),bounds(node(base+names[j]))),names[i]+'遮挡'+names[j]);
 for(const n of scene.slice(1191))if(n.__type__==='cc.Button')assert.deepEqual(n.clickEvents,[]);
});
