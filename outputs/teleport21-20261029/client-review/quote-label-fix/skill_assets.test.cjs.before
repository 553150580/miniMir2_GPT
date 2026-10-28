const {test}=require('node:test');
const assert=require('node:assert/strict');
const {SkillMaterialSelection,adjustPurchaseQuantity,assetId,canRecycleBook,upgradeRequirements,materialSelectionMatches,captureSkillAssetContext,validateSkillAssetContext,
    validateQuotedMaterials,skillQuoteLines,yuanbaoText}=require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const id=value=>new Uint8Array(16).fill(value);
function snapshot(){return {ready:true,roleIdentity:'1:roleA',sceneEpoch:'9007199254740993',ruleVersion:'4',inventory:{revision:'9007199254740995',level:7,
    learnedSkills:[{skillId:1,level:0}],items:[
        {instanceId:id(1),itemId:7,quantity:'3',version:'2',bound:false,sourceKind:'monster_drop',sourceId:'drop1',quality:1},
        {instanceId:id(2),itemId:7,quantity:'2',version:'4',bound:true,sourceKind:'npc_shop',sourceId:'buy2',quality:0},
        {instanceId:id(3),itemId:900001,quantity:'9007199254740993',version:'6',bound:true,sourceKind:'npc_shop',sourceId:'buy3',quality:0},
    ]},wallet:{revision:'9007199254740995',gold:'1000',boundGold:'300',yuanbaoUnits:'100',boundYuanbaoUnits:'200'},
    definitions:[{item_id:7,name:'《火球术》',kind:'book'},{item_id:900001,name:'书页',kind:'skill_page'}],skills:[{id:1,name:'火球术',job_id:1,max_level:3}],offers:[],upgrades:[],recycle:[]};}
const rule=(level=1)=>({skill_id:1,level,required_role_level:7,book_costs:[{item_id:7,quantity:level+1}],page_item_id:900001,page_cost:[0,5,10,20][level]});
const recycle=[{item_id:7,book_quality:1,page_item_id:900001,pages_per_book:5,allowed_source_kinds:['monster_drop']}];
const choices=input=>[{instanceId:id(1),version:'2',quantity:2n},{instanceId:id(3),version:'6',quantity:5n}];
function quote(input){return {previewId:id(9),kind:2,ruleVersion:'4',configHash:'a'.repeat(64),roleRevision:'9007199254740995',expiresAtUnixMs:'20000',
    skillId:1,fromLevel:0,toLevel:1,offerId:0,quantity:'0',spend:{revision:'9007199254740995',gold:'0',boundGold:'0',yuanbaoUnits:'0',boundYuanbaoUnits:'0'},
    materials:[{item:{...input.inventory.items[0]},consumeQuantity:'2'},{item:{...input.inventory.items[2]},consumeQuantity:'5'}],outputs:[],
    stats:[{key:'base_power',beforeMin:'2',beforeMax:'3',afterMin:'3',afterMax:'5'},{key:'accuracy_bonus',beforeMin:'0',beforeMax:'0',afterMin:'3',afterMax:'3'}]};}

test('购买数量显式增减并受当前SKU上限约束，不切换支付商品或币种',()=>{
    let amount=1n;for(let i=0;i<4;i++)amount=adjustPurchaseQuantity(amount,1,32);assert.equal(amount,5n);
    amount=adjustPurchaseQuantity(amount,-1,32);assert.equal(amount,4n);
    assert.throws(()=>adjustPurchaseQuantity(1n,-1,32),/1～32/);assert.throws(()=>adjustPurchaseQuantity(32n,1,32),/1～32/);
    assert.throws(()=>adjustPurchaseQuantity(1n,1,0),/配置无效/);
});

test('材料从0开始显式选择，绑定与非绑定实例独立，数量与版本不经Number截断',()=>{
    const input=snapshot(),selected=new SkillMaterialSelection();for(const item of input.inventory.items)assert.equal(selected.quantity(item),0n);
    selected.adjust(input.inventory.items[0],2);selected.adjust(input.inventory.items[2],5);
    assert.equal(selected.quantity(input.inventory.items[1]),0n);assert.deepEqual(selected.selected(input.inventory.items).map(x=>x.quantity),[2n,5n]);
    assert(materialSelectionMatches(rule(),selected.selected(input.inventory.items),input.inventory.items));
    assert.throws(()=>selected.adjust(input.inventory.items[0],2),/超出/);assert.throws(()=>selected.adjust(input.inventory.items[1],-1),/超出/);
    selected.adjust(input.inventory.items[0],-2);assert.equal(selected.selected(input.inventory.items).length,1);
    assert.equal(input.inventory.items[2].quantity,'9007199254740993');
});
test('原实例消失或版本变化不得替换到另一堆，刷新仅清理失效选择',()=>{
    const input=snapshot(),selected=new SkillMaterialSelection();selected.adjust(input.inventory.items[0],2);selected.adjust(input.inventory.items[2],5);
    input.inventory.items.splice(0,1);assert.throws(()=>selected.selected(input.inventory.items),/离开背包/);
    assert.equal(selected.reconcile(input.inventory.items),true);assert.equal(selected.quantity(input.inventory.items[0]),0n);
    assert.deepEqual(selected.selected(input.inventory.items).map(x=>x.quantity),[5n]);
    input.inventory.items[1].version='7';assert.throws(()=>selected.adjust(input.inventory.items[1],1),/版本/);
    selected.reconcile(input.inventory.items);assert.equal(selected.selected(input.inventory.items).length,0);
});
test('逐阶独立书本与书页需求精确匹配，超额、欠额或错书不进入报价',()=>{
    const input=snapshot();assert.deepEqual(Array.from(upgradeRequirements(rule())),[[7,2n],[900001,5n]]);
    assert.deepEqual(Array.from(upgradeRequirements(rule(2))),[[7,3n],[900001,10n]]);
    assert.deepEqual(Array.from(upgradeRequirements(rule(3))),[[7,4n],[900001,20n]]);
    assert(materialSelectionMatches(rule(),choices(input),input.inventory.items));
    assert.equal(materialSelectionMatches(rule(2),choices(input),input.inventory.items),false);
    assert.equal(materialSelectionMatches(rule(),[{instanceId:id(1),version:'2',quantity:3n}],input.inventory.items),false);
});
test('只有明确掉落来源与白名单品质可拆，NPC、初始、legacy、无来源ID均拒绝',()=>{
    const item=snapshot().inventory.items[0];assert(canRecycleBook(item,recycle));
    assert(canRecycleBook({...item,bound:true},recycle));
    for(const changed of [{sourceKind:'npc_shop'},{sourceKind:'initial_grant'},{sourceKind:''},{sourceKind:undefined},{sourceId:''},{quality:0},{quality:2},{quantity:'0'}])assert.equal(canRecycleBook({...item,...changed},recycle),false);
});
test('报价捕获要求钱包与库存同revision，角色场景和每个原材料信息均须保持',()=>{
    for(const mutate of [v=>{v.ready=false;},v=>{v.wallet.revision='1';},v=>{v.wallet=null;},v=>{v.roleIdentity='';}]){const input=snapshot();mutate(input);assert.throws(()=>captureSkillAssetContext(input,choices(input)),/尚未同步/);}
    for(const mutate of [v=>{v.roleIdentity='1:roleB';},v=>{v.sceneEpoch='2';},v=>{v.ruleVersion='5';},v=>{v.inventory.revision='3';},v=>{v.wallet.revision='3';},v=>{v.wallet.gold='999';},
        v=>{v.inventory.items[0].version='3';},v=>{v.inventory.items[0].quantity='2';},v=>{v.inventory.items[0].bound=true;},v=>{v.inventory.items[0].sourceKind='npc_shop';},v=>{v.inventory.items[0].sourceId='different';},v=>{v.inventory.items[0].quality=0;}]){
        const input=snapshot(),context=captureSkillAssetContext(input,choices(input)),preview=quote(input);mutate(input);assert.throws(()=>validateSkillAssetContext(context,preview,input,10000),/重新预览/);
    }
});
test('报价到期、规则或角色revision不一致须重新预览，不自动重报价或换料',()=>{
    const input=snapshot(),context=captureSkillAssetContext(input,choices(input)),preview=quote(input);validateSkillAssetContext(context,preview,input,19999);
    assert.throws(()=>validateSkillAssetContext(context,preview,input,20000),/重新预览/);
    for(const changed of [{roleRevision:'1'},{ruleVersion:'5'},{previewId:new Uint8Array(15)}])assert.throws(()=>validateSkillAssetContext(context,{...preview,...changed},input,10000));
});
test('服务器报价必须完整对应所选实例、消耗量和来源，不接受替换或重复实例',()=>{
    const input=snapshot();validateQuotedMaterials(quote(input),choices(input),input);
    for(const mutate of [q=>{q.materials.pop();},q=>{q.materials[0].consumeQuantity='1';},q=>{q.materials[0].item.version='3';},q=>{q.materials[0].item.sourceKind='npc_shop';},q=>{q.materials[1]={...q.materials[0]};},q=>{q.spend.gold='1001';}]){
        const preview=quote(input);mutate(preview);assert.throws(()=>validateQuotedMaterials(preview,choices(input),input));
    }
});
test('滚动报价列出四钱包、逐技能数值、原材料绑定来源和产物，基础威力不冒充总伤害',()=>{
    const input=snapshot(),preview=quote(input);preview.spend={...preview.spend,gold:'200',boundGold:'300',yuanbaoUnits:'50',boundYuanbaoUnits:'100'};
    preview.outputs=[{...input.inventory.items[2],quantity:'5',bound:false,sourceKind:'skill_recycle'}];
    preview.stats.push({key:'max_attack_bonus',beforeMin:'5',beforeMax:'5',afterMin:'6',afterMax:'6'});
    const text=skillQuoteLines(preview,input.definitions).join('\n');
    for(const expression of [/金币支出 200/,/绑定金币支出 300/,/元宝支出 5\.0/,/绑定元宝支出 10\.0/,/技能等级 0 → 1/,/技能基础威力\n当前下限 2\n当前上限 3\n下一阶下限 3\n下一阶上限 5/,/准确加成\n当前值 0\n下一阶 3/,/非绑定 · 怪物掉落/,/绑定 · NPC书店/,/非绑定 · 拆书所得/])assert.match(text,expression);
    assert.doesNotMatch(text,/预计伤害|总伤害|总治疗/);assert.equal(yuanbaoText('9007199254740993'),'900719925474099.3');
    assert.match(text,/攻击上限加成\n当前值 5\n下一阶 6/);
    assert.equal(assetId(input.inventory.items[0].instanceId),'01'.repeat(16));
});
