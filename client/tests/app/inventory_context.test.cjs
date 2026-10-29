const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load, harness, ready, entry, Long } = require('./helpers/session_harness.cjs');
const compiled = name => require('../../../.work/ui-tests/app/' + name + '.js');
const bytes = n => new Uint8Array(16).fill(n);
function fixture() {
    const h = harness(), s = h.session; ready(h, entry('1'));
    const page = n => ({ instanceId: bytes(n), itemId: 900001, version: Long.UONE, quantity: Long.fromInt(5), bound: true, quality: 0, sourceKind: 'skill_recycle', sourceId: 'same-source', locked: false, recycleBlockFlags: 0 });
    s.inventory = { revision: Long.UONE, level: 10, items: [page(1), page(2), { ...page(3), itemId: 218 }], equipped: [] };
    s.baseline = { stateSequence: Long.UONE, entities: [], navigationState: { status: 1 } };
    const decorator = (...args) => args.length > 1 ? undefined : (...values) => values.length === 1 ? values[0] : undefined;
    class Button {} class Toggle {} class Label {} class List {}
    const cc = { _decorator: new Proxy({}, { get: () => decorator }), Component: class {}, Button, Toggle, Label };
    const { AppRoot } = load(process.env.MINIMIR_INVENTORY_CONTEXT_SOURCE || 'client/assets/scripts/app/AppRoot.ts', id => id === 'cc' ? cc : id === 'long' ? Long : id === './GameSession' ? h
        : id === '../input/NavigationIntent' ? require('../../../.work/ui-tests/input/NavigationIntent.js') : id === '../ui/List' ? { List }
        : ['./SkillAssetsModel', './PetBornModel', './RoleBindingModel', './MapTravelModel'].includes(id) ? compiled(id.slice(2)) : {});
    const nodes = new Map(), node = path => {
        if (!nodes.has(path)) {
            const listeners = new Map(), saved = { active: true, on(event, listener) { listeners.set(event, listener); }, emit(event, ...args) { listeners.get(event)?.(...args); }, getComponent() { return this.component; } };
            saved.component = { node: saved, string: '', interactable: false, offset: 517, setItemsCount: 0, items: [], setItems(items) { this.items = items.slice(); this.setItemsCount++; } };
            nodes.set(path, saved);
        }
        return nodes.get(path);
    };
    const app = new AppRoot(), shown = []; let consumed = 0;
    app.world = { isMapReady: true, apply() {}, map: { name: '开发图' }, manifest: { profiles: { pet_skeleton: { layers: [{ actions: { appear: { count: 10, duration_ms: 100 } } }] } } },
        gameplay: { rule_version: 21, skills: [], items: [{ item_id: 900001, kind: 'skill_page', name: '书页' }, { item_id: 218, kind: 'equipment', name: '木剑', equip_slot: 0, allowed_jobs: [0], required_level: 1 }], map_catalog: [], town_travel: { enabled: true } } };
    app.find = node; app.isWorldCurrent = () => true; app.quickSkill = () => null; app.status = () => {}; app.popup = { show(kind, value) { shown.push({ kind, ...value }); } }; app.reconciliationStarted = true;
    app.refreshGame(); const inventory = node('UIMgr/Panels/Inventory/List').component;
    app.nav.setGroundTarget({ x: 9, y: 9 }); s.petBorn.take = () => { consumed++; return null; };
    let pendingReads = 0; app.reconciliationStarted = false; s.pending = { get() { pendingReads++; return null; } };
    const count = inventory.setItemsCount;
    return { app, s, shown, node, inventory, count, consumed: () => consumed, pendingReads: () => pendingReads,
        select(item) { inventory.node.emit('list-item-click', 0, item); },
        selectEquipped(item) { node('UIMgr/Panels/Role/List').component.node.emit('list-item-click', 0, item); },
        label: () => node('UIMgr/Panels/Inventory/ItemLock/Label').component.string,
        enabled: () => node('UIMgr/Panels/Inventory/ItemLock').component.interactable };
}

test('同一保存List选择回调立即更新合并上下文，不等notify、不重绑或移动滚动、不消费场景事件和未决操作', () => {
    const f = fixture(); assert.equal(f.label(), '锁定装备');
    f.select(f.s.inventory.items[0]);
    assert.equal(f.label(), '合并书页'); assert.equal(f.enabled(), true); assert.equal(f.shown.at(-1).kind, 'Details');
    assert.equal(f.inventory.setItemsCount, f.count); assert.equal(f.inventory.offset, 517); assert.equal(f.consumed(), 0);
    assert.equal(f.pendingReads(), 0); assert.equal(f.app.reconciliationStarted, false);
    assert.deepEqual(f.app.nav.state.target, { x: 9, y: 9 }); assert.equal(f.s.connection.sent.length, 0);
});

test('当次选择区分普通装备、已锁装备与受保护或缺同源的书页', () => {
    const f = fixture(), equipment = f.s.inventory.items[2];
    f.select(equipment); assert.equal(f.label(), '锁定装备'); assert.equal(f.enabled(), true);
    equipment.locked = true; equipment.recycleBlockFlags = 1; f.select(equipment); assert.equal(f.label(), '解锁装备'); assert.equal(f.enabled(), true);
    equipment.recycleBlockFlags = 4; f.select(equipment); assert.equal(f.enabled(), false);
    const page = f.s.inventory.items[0]; page.recycleBlockFlags = 4; f.select(page); assert.equal(f.label(), '合并书页'); assert.equal(f.enabled(), false);
    page.recycleBlockFlags = 0; f.s.inventory.items[1].sourceId = 'different-source'; f.select(page); assert.equal(f.enabled(), false);
    assert.equal(f.s.connection.sent.length, 0); assert.equal(f.inventory.offset, 517); assert.equal(f.inventory.setItemsCount, f.count);
});

test('虚拟旧行只按同原ID读取当前物品；已消失条目清上下文且不偷偷选择其它实例', () => {
    const f = fixture(), old = f.s.inventory.items[0];
    const current = { ...old, quantity: Long.fromInt(7), version: Long.fromInt(2, true) }; f.s.inventory.items[0] = current;
    f.select(old); assert.equal(f.app.selectedItem, current); assert(f.shown.at(-1).message.includes('数量 7'));
    f.s.inventory.items = f.s.inventory.items.filter(x => x !== current); f.select(old);
    assert.equal(f.app.selectedItem, null); assert.equal(f.enabled(), false); assert.equal(f.shown.at(-1).title, '物品已变化');
    assert.equal(f.s.connection.sent.length, 0); assert.deepEqual(f.app.nav.state.target, { x: 9, y: 9 });
});

test('Role已穿戴选择与背包上下文隔离，当次显示卸下而不重绑列表或丢任一offset',()=>{
    const f=fixture(),bag=f.s.inventory.items[0],worn={...f.s.inventory.items[2],instanceId:bytes(8),quantity:Long.ONE,recycleBlockFlags:8};f.s.inventory.equipped=[{slot:0,item:worn}];f.select(bag);
    const role=f.node('UIMgr/Panels/Role/List').component,count=role.setItemsCount;f.selectEquipped(worn);
    assert.equal(f.app.selectedItem,bag);assert.equal(f.app.selectedEquipped.item,worn);assert.equal(f.app.selectedEquipped.slot,0);
    assert.equal(f.label(),'合并书页');assert.equal(f.node('UIMgr/Panels/Role/Equip/Label').component.string,'卸下装备');assert.equal(f.node('UIMgr/Panels/Role/Equip').component.interactable,true);
    assert.equal(f.inventory.offset,517);assert.equal(role.offset,517);assert.equal(role.setItemsCount,count);assert.equal(f.inventory.setItemsCount,f.count);assert.equal(f.pendingReads(),0);assert.equal(f.consumed(),0);
});

test('Inventory刷新保留仍在原slot的Role选择，新版本实时重取；原已穿戴消失不改成同类背包实例',()=>{
    const f=fixture(),worn={...f.s.inventory.items[2],instanceId:bytes(8),quantity:Long.ONE,recycleBlockFlags:8};f.s.inventory.equipped=[{slot:0,item:worn}];f.selectEquipped(worn);
    const updated={...worn,version:Long.fromInt(2,true)};f.s.inventory={...f.s.inventory,revision:Long.fromInt(2,true),equipped:[{slot:0,item:updated}]};f.app.refreshInventoryContext();assert.equal(f.app.selectedEquipped.item,updated);assert.equal(f.app.selectedItem,null);
    f.s.inventory={...f.s.inventory,equipped:[]};f.app.refreshInventoryContext();assert.equal(f.app.selectedEquipped,null);assert.equal(f.node('UIMgr/Panels/Role/Equip').component.interactable,false);assert.equal(f.app.selectedItem,null);
});

test('实际装备条目打开保存滚动详情provider保留单件属性/来源；当前容器实时重取且旧Entry失效',()=>{
    const f=fixture(),definition=f.app.world.gameplay.items.find(row=>row.item_id===218);definition.equipment_stage=0;definition.base_attributes={physical_attack_min:1,physical_attack_max:3,magic_attack_min:0,magic_attack_max:1,spirit_attack_min:0,spirit_attack_max:2,physical_defence_min:0,physical_defence_max:0,magic_defence_min:0,magic_defence_max:0,attack_speed:0};
    const worn={...f.s.inventory.items[2],instanceId:bytes(8),quantity:Long.ONE,recycleBlockFlags:8};f.s.inventory.equipped=[{slot:0,item:worn}];let read;f.app.combatStatus={showLiveDetails(title,provider){assert.equal(title,'木剑');read=provider;}};f.selectEquipped(worn);
    assert(read().includes('已穿戴装备'));assert(read().includes('魔法攻击 0–1'));assert(read().includes('道术攻击 0–2'));assert(read().some(line=>line.startsWith('来源 ')));f.s.inventory.equipped=[];assert.deepEqual(read(),['原装备已离开此列表，请重新选择']);f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};assert.equal(read(),null);assert.equal(f.inventory.offset,517);
});
