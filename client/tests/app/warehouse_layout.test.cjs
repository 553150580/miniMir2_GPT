const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),crypto=require('node:crypto');
const scene=JSON.parse(fs.readFileSync('client/assets/scenes/Game.scene','utf8')),nodes=new Map();
function walk(index,prefix=''){const n=scene[index],key=prefix?prefix+'/'+n._name:n._name;assert(!nodes.has(key));nodes.set(key,index);for(const c of n._children||[])walk(c.__id__,key);}
walk(scene[0].scene.__id__);const safe=Array.from(nodes.keys()).find(k=>k.endsWith('/Canvas/Safe'));
const index=name=>{const key=safe+'/'+name;assert(nodes.has(key),key);return nodes.get(key);},node=name=>scene[index(name)];
const component=(n,type)=>{const c=n._components.map(r=>scene[r.__id__]).find(c=>c.__type__===type);assert(c,type+' '+n._name);return c;},size=n=>component(n,'cc.UITransform')._contentSize;
const base='UIMgr/Panels/Warehouse/',bounds=n=>({l:n._lpos.x-size(n).width/2,r:n._lpos.x+size(n).width/2,b:n._lpos.y-size(n).height/2,t:n._lpos.y+size(n).height/2});
const overlap=(a,b)=>a.l<b.r&&a.r>b.l&&a.b<b.t&&a.t>b.b;
test('仓库只追加保存子树与Functions入口；原1191对象只父children各追加一次，原地图/HUD/ItemLock全部字节语义保持',()=>{
 const market=require('./helpers/item_market_layout.cjs')(structuredClone(scene));
 const projection=require('./helpers/hosting_purchase_layout.cjs')(require('./helpers/role_card_layout.cjs')(market));
 const beforeDirectory=require('./helpers/inventory_grid_layout.cjs')(require('./helpers/skill_layout.cjs')(projection)).slice(0,1373);
 beforeDirectory[253]._children=beforeDirectory[253]._children.filter(x=>x.__id__<1373);
 const ground=beforeDirectory[26]._children.filter(ref=>ref.__id__>=1373);
 assert.deepEqual(ground.map(ref=>scene[ref.__id__]._name),['GroundEffects']);
 beforeDirectory[26]._children=beforeDirectory[26]._children.filter(ref=>ref.__id__<1373);
 const reconnect=beforeDirectory[2]._children.filter(ref=>ref.__id__>=1373);
 assert.deepEqual(reconnect.map(ref=>scene[ref.__id__]._name),['ReconnectDialog']);
 beforeDirectory[2]._children=beforeDirectory[2]._children.filter(ref=>ref.__id__<1373);
 const overlays=beforeDirectory[450]._children.filter(ref=>scene[ref.__id__]._name==='ItemActions');
 assert.equal(overlays.length,1);
 beforeDirectory[450]._children=beforeDirectory[450]._children.filter(ref=>scene[ref.__id__]._name!=='ItemActions');
 // 镜像竞技只在 Safe 和原功能页追加入口引用，剥离后继续校验仓库阶段的历史对象。
 for(const [parent,name] of [[17,'MirrorArenaBattle'],[395,'MirrorArena']]) {
  beforeDirectory[parent]._children=beforeDirectory[parent]._children.filter(ref=>scene[ref.__id__]._name!==name);
 }
 assert.equal(crypto.createHash('sha256').update(JSON.stringify(beforeDirectory)).digest('hex'),'4914af7ce27923adca7039cdb36663ab4ffef85e7fd2915c009b9927a772f462');
 const projected=beforeDirectory.slice(0,1191);
 require('./helpers/auto_recycle_layout.cjs').normalizeConfirmTextWrap(projected);
 for(const parent of [253,395]){assert.equal(beforeDirectory[parent]._children.filter(x=>x.__id__>=1191).length,1);projected[parent]._children=projected[parent]._children.filter(x=>x.__id__<1191);}
 assert.equal(crypto.createHash('sha256').update(JSON.stringify(projected)).digest('hex'),'373823584fe100fa7d5e53150f5660cb242e87ba7055d7a75e54ca34e6669675');
 assert.equal(index('UIMgr/Panels/Warehouse'),1191);assert(index('UIMgr/Panels/Functions/Pages/Page0/Warehouse')>=1191);assert.equal(component(node('UIMgr/Panels/Inventory/List/View'),'cc.UITransform')._contentSize.height,414);
 const ids=scene.filter(x=>x.__type__==='cc.Node').map(x=>x._id);assert.equal(new Set(ids).size,ids.length);
});
test('地图目录仅追加保存面板，左右可滚动区域及按钮命中框互不重叠',()=>{
 assert.equal(index('UIMgr/Panels/TownDirectory'),1373);
 const prefix='UIMgr/Panels/TownDirectory/';
 assert.equal(component(node(prefix+'Title'),'cc.Label')._string,'地图');
 const columns=['RegionList','MapList'];
 for(const name of columns){
  const base=prefix+name,list=node(base),scroll=component(list,'cc.ScrollView'),view=node(base+'/View');
  assert.equal(component(view,'cc.Mask')._type,0);assert.equal(component(view,'cc.Widget')._bottom,0);
  assert.equal(scroll._content.__id__,index(base+'/View/Content'));
  const template=node(base+'/View/Content/ItemTemplate');assert.equal(template._active,false);
  assert(component(template,'cc.Button'));assert.equal(component(node(base+'/View/Content/ItemTemplate/Text'),'cc.Label')._fontSize,24);
  assert.equal(size(view).width,size(list).width);assert.equal(size(template).width,size(list).width);
 }
 assert(!overlap(bounds(node(prefix+columns[0])),bounds(node(prefix+columns[1]))));
 assert(size(node(prefix+columns[1])).width>=2*size(node(prefix+columns[0])).width);
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
