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
