const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { createHash } = require('node:crypto');
const { EventEmitter } = require('node:events');
const path = require('node:path');
const { typescript } = require('../../../tools/client/creator_paths.cjs');
const { Long, load } = require('./helpers/session_harness.cjs');
const rulesModel = require('../../../.work/ui-tests/app/EquipmentRefineRules.js');
const model = require('../../../.work/ui-tests/app/EquipmentRefineModel.js');
const receiptModel = require('../../../.work/ui-tests/app/EquipmentRefineReceipt.js');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', key => key === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const transport = load('client/assets/scripts/app/GameConnection.ts', key => ({ long: Long, '../generated/protobuf/client_pb.js': pb })[key] ?? {});

const id = n => new Uint8Array(16).fill(n), u = n => Long.fromString(String(n), true), s = n => Long.fromString(String(n));
const version = u('9007199254741092'), revision = u('9007199254741101'), scene = u('9007199254741123');
const ui = id(30), quoteId = id(8), pendingId = id(9);
const stoneId = 900051;
const definition = { item_id: 220, kind: 'equipment', name: '乌木剑', required_level: 1, equipment_stage: 0 };
const necklace = { item_id: 910022, kind: 'equipment', name: '稀有项链', required_level: 20, equipment_stage: 1 };
const raw = {
    stone_item_id: stoneId, targets: [{ item_id: 220, quality: 1 }, { item_id: 910022, quality: 2 }],
    qualities: [{ quality: 1, slot_count: 1, stone_cost: 2, allow_duplicate_attr: false }, { quality: 2, slot_count: 2, stone_cost: 4, allow_duplicate_attr: false }],
    affixes: [{ id: 1, name: '攻击', min_quality: 2 }, { id: 2, name: '防御', min_quality: 1 }, { id: 3, name: '最大生命', min_quality: 1 }],
    gold_bands: [{ min_level: 1, max_level: 19, gold: 1000 }, { min_level: 20, max_level: 0, gold: 3000 }],
    lock_costs: [{ locked_slots: 0, ingot_u: 0 }, { locked_slots: 1, ingot_u: 2000 }],
    tiers: [450000, 300000, 150000, 70000, 30000].map((weight_ppm, index) => ({ tier: index + 1, weight_ppm })),
    recycle: { item_id: 220, source_kind: 'monster_drop', stage: 0, quality: 1, probability_ppm: 50000, quantity: 1 },
};
const rules = rulesModel.equipmentRefineRules(raw);
const weapon = { instanceId: id(1), itemId: definition.item_id, version, quantity: s(1), bound: false, quality: 1,
    sourceKind: 'monster_drop', sourceId: '42', durabilityU: u(6191), maxDurabilityU: u(7000), originalDurabilityU: u(7000) };
const free = { instanceId: id(2), itemId: stoneId, version, quantity: s(2), bound: false, sourceKind: 'equipment_recycle_refine', sourceId: assets.assetId(id(22)) };
const bound = { ...free, instanceId: id(3), bound: true, quantity: s(1), sourceId: assets.assetId(id(23)) };
const state = { role: 'role-a', scope: 'role-a:scene', revision, sceneEpoch: scene, rules, items: [weapon, free, bound],
    equipped: [], catalog: [definition, necklace], blocked: false };
const newAffix = { slot: 0, attributeId: 3, tier: 5, valueUnits: s(96) };
const firstAffix = { slot: 0, attributeId: 2, tier: 1, valueUnits: s(10) };
const roundTrip = (code, object) => transport.decodePacket(transport.encodePacket(code, 11, Long.UONE, object)).value;

function quoteFor(request, snapshot = state, extra = {}) {
    return { quoteId, uiSessionId: request.uiSessionId, sceneEpoch: request.sceneEpoch, roleRevision: snapshot.revision,
        expiresAtUnixMs: u(2000000000000), target: model.findRefineItem(snapshot, request.target.instanceId, request.target.equipped),
        equipped: request.target.equipped, lockedSlots: request.lockedSlots, allowBound: request.allowBound, outputBound: false,
        stones: request.stones.map(row => ({ item: snapshot.items.find(item => model.sameRefineId(item.instanceId, row.instanceId)), quantity: row.quantity })),
        gold: s(1000), boundGold: s(0), yuanbaoUnits: s(0), boundYuanbaoUnits: s(0), ...extra };
}

function selectFree(snapshot = state) {
    const materials = new model.RefineStoneSelection();
    materials.pick(snapshot, free, 2n);
    return materials.request(snapshot, weapon, false, [], ui);
}

test('洗炼入口只接受完整公开装配，旧规则不出现，分解概率按原普通掉落保护条件展示', () => {
    assert.equal(rulesModel.equipmentRefineRules(undefined), undefined);
    assert.ok(rules);
    assert.equal(rulesModel.equipmentRefineRules({ ...raw, lock_costs: [{ locked_slots: 0, ingot_u: 2000 }] }), undefined);
    assert.equal(rulesModel.equipmentRefineQuality(rules, { ...weapon, quality: 0 }), undefined);
    assert.equal(rulesModel.equipmentRefineGold(rules, 25), 3000n);
    assert.match(rulesModel.refineRecycleHint(rules, weapon, definition), /5%.*1枚非绑石/);
    assert.match(rulesModel.refineRecycleHint(rules, { ...weapon, bound: true }, definition), /1枚绑定石/);
    for (const changed of [{ sourceKind: 'npc_shop' }, { sourceId: '未知' }, { currentLuck: 1 }, { durabilityU: u(0) }, { durabilityU: u(7331) }]) {
        assert.equal(rulesModel.refineRecycleHint(rules, { ...weapon, ...changed }, definition), '');
    }
});

test('双列词条保留真实十分之一，锁槽只能使用已采纳的左边，候选从库存而非本地状态恢复', () => {
    const paid = { ...weapon, version: version.add(1), refineAffixes: [firstAffix], recycleBlockFlags: 2,
        refinePending: { pendingId, affixes: [newAffix], lockedSlots: [] } };
    const stateAfter = model.refineTargetState(state, paid);
    assert.equal(stateAfter.blocked, false);
    assert.deepEqual(stateAfter.current, [firstAffix]);
    assert.deepEqual(stateAfter.pending.affixes, [newAffix]);
    assert.equal(model.refineAffixText(rules, newAffix), '最大生命 +9.6\n5档');
    assert.equal(model.refineTargetState(state, { ...paid, locked: true }).blocked, true);
    assert.equal(model.refineTargetState(state, { ...paid, refinePending: { ...paid.refinePending, lockedSlots: [0] } }).blocked, true);
    const piece = { ...paid, itemId: necklace.item_id, quality: 2, refinePending: null, recycleBlockFlags: 0 };
    const worn = { ...state, items: [{ ...free, quantity: s(4) }], equipped: [{ slot: 3, item: piece }] };
    const materials = new model.RefineStoneSelection();
    materials.pick(worn, worn.items[0], 4n);
    assert.ok(materials.request(worn, piece, true, [0], ui));
    assert.equal(materials.request(worn, piece, true, [1], ui), undefined);
    assert.equal(materials.request(worn, piece, true, [0, 1], ui), undefined);
    assert.equal(model.findRefineItem(worn, piece.instanceId, false), undefined);
});

test('原石分配、双钱袋分账及精确 64 位版本由真实编解码保留', () => {
    const selection = new model.RefineStoneSelection();
    selection.pick(state, bound, 2n);
    assert.equal(selection.total(state), 0n);
    selection.allowBound = true;
    selection.pick(state, bound, 2n);
    selection.pick(state, free, 2n);
    const request = roundTrip(40058, selection.request(state, weapon, false, [], ui));
    const quote = roundTrip(40059, quoteFor(request, state, { gold: s(500), boundGold: s(500), outputBound: true }));
    assert.ok(request.target.version.equals(version));
    assert.ok(quote.roleRevision.equals(revision));
    assert.deepEqual(request.stones.map(row => row.quantity.toString()), ['1', '1']);
    assert.doesNotThrow(() => model.validateRefineQuote(quote, request, { ...state, revision: revision.add(4) }));
    assert.throws(() => model.validateRefineQuote({ ...quote, outputBound: false }, request, state), /费用或绑定/);
    assert.throws(() => model.validateRefineQuote({ ...quote, boundGold: s(499) }, request, state), /费用或绑定/);
    assert.throws(() => model.validateRefineQuote({ ...quote, roleRevision: revision.add(1) }, request, state), /重新查看/);
    const changed = { ...state, items: [weapon, { ...free, version: version.add(2) }, bound] };
    assert.equal(selection.focused(changed), undefined);
    assert.equal(selection.total(changed), 1n);
    assert.equal(selection.request(changed, weapon, false, [], ui), undefined);
    assert.throws(() => model.validateRefineQuote(quote, request, changed), /洗炼石已变化/);
});

test('永久回执保留原报价和候选号，重抽须新候选，采纳或放弃不会把费用再次扣除', () => {
    const request = selectFree(), quote = quoteFor(request);
    const action = { action: 1, target: request.target, quoteId, uiSessionId: ui, allowBound: false };
    const paid = { ...weapon, version: version.add(1), recycleBlockFlags: 2,
        refinePending: { pendingId, affixes: [newAffix], lockedSlots: [] } };
    const receipt = roundTrip(40005, { equipmentRefine: { ...quote, action: 1, target: paid, pendingId } }).equipmentRefine;
    assert.ok(receiptModel.validateRefineReceipt(action, receipt, quote).target.version.equals(version.add(1)));
    assert.equal(receiptModel.validateRefineReceipt(action, receipt).target.refinePending.affixes[0].valueUnits.toString(), '96');
    assert.throws(() => receiptModel.validateRefineReceipt(action, { ...receipt, pendingId: id(25) }), /原报价/);
    assert.throws(() => receiptModel.validateRefineReceipt(action, receipt, { ...quote, target: paid }), /偏离原报价/);
    for (const n of [2, 3]) {
        const decide = { action: n, target: model.freezeRefineTarget(paid, false), pendingId, uiSessionId: ui };
        const after = { ...paid, version: paid.version.add(1), refinePending: null, recycleBlockFlags: 0, refineAffixes: n === 2 ? [newAffix] : [] };
        const confirmed = roundTrip(40005, { equipmentRefine: { action: n, target: after, pendingId } }).equipmentRefine;
        assert.equal(receiptModel.validateRefineReceipt(decide, confirmed).action, n);
        assert.throws(() => receiptModel.validateRefineReceipt(decide, { ...confirmed, gold: s(1) }), /决定回执/);
    }
});

test('实发 Go 绑定报价和回执经过幂运算降级后仍可对账', () => {
    // 从隔离触屏取出的三个原始协议体包含 Go 省略的零字段、两枚绑定石和绑定金币。
    const captured = (code, sequence, body) => transport.decodePacket(Buffer.concat([
        Buffer.from([1, 0, code >> 8, code & 255, sequence, 1]), Buffer.from(body, 'base64'),
    ])).value;
    const request = captured(40060, 9,
        'CAESGAoQAbKp1BVIqmusCvlosHPV5xABGAIgHhoUChBkcgj/Vdkar05OscuTvpDEEAIiEKIpiy+Al/Orm9sOgzOqVmsqEJI6gYTewZpGI8W3ohX6qeg4AQ==');
    const quote = captured(40059, 8,
        'ChCiKYsvgJfzq5vbDoMzqlZrELHLw73Lhez7zwEaNQoQZHII/1XZGq9OTrHLk76QxBDcARgBKAIyDG1vbnN0ZXJfZHJvcDoBMUABWNg2YNg2aNg2' +
        'MlwKWAoQyhLrBkP7Sa9+F4/z02xInBDT9zYYASABKAEyGGVxdWlwbWVudF9yZWN5Y2xlX3JlZmluZTogYmJkNGE0YjQ0YWE1YjdkMTQxNjU2MjBkMDJiNmQ5NzUQATJc' +
        'ClgKEJ/resSpp9Zzc1XSVvjTe/8Q0/c2GAEgASgBMhhlcXVpcG1lbnRfcmVjeWNsZV9yZWZpbmU6IDRiNTliZGIzNzFiZWNmNDQ5NTI0NDVlYTAwNmRiMTZiEAE4AUjoB2ABaJCWw8ChNHACehCSOoGE3sGaRiPFt6IV+qno');
    const { equipmentRefine } = captured(40005, 9,
        'ChABsqnUFUiqa6wK+Wiwc9XnEAEYA4oBvQIIARJWChBkcgj/Vdkar05OscuTvpDEENwBGAEgASgDMgxtb25zdGVyX2Ryb3A6ATFAAVACWNg2YNg2aNg2kgEa' +
        'ChCudAMO5mmRqUQXyWuFIUJpEgYQAxgCIDAiEKIpiy+Al/Orm9sOgzOqVmsqEK50Aw7maZGpRBfJa4UhQmkyXApYChDKEusGQ/tJr34Xj/PTbEicENP3NhgBIAEoATIYZXF1aXBtZW50X3JlY3ljbGVfcmVmaW5lOiBiYmQ0YTRiNDRhYTViN2QxNDE2NTYyMGQwMmI2ZDk3NRABMlwKWAoQn+t6xKmn1nNzVdJW+NN7/xDT9zYYASABKAEyGGVxdWlwbWVudF9yZWN5Y2xlX3JlZmluZTogNGI1OWJkYjM3MWJlY2Y0NDk1MjQ0NWVhMDA2ZGIxNmIQAUDoBw==');
    assert.equal(request.allowBound, true);
    assert.equal(quote.target.bound, false);
    assert.equal(equipmentRefine.target.bound, true);

    // 与 Creator 实包相同，使用会把幂运算转为 Math.pow 的目标执行同一个生产校验器。
    const source = readFileSync(path.join(__dirname, '../../assets/scripts/app/EquipmentRefineReceipt.ts'), 'utf8');
    const { outputText } = typescript.transpileModule(source, { compilerOptions: {
        target: typescript.ScriptTarget.ES2015, module: typescript.ModuleKind.CommonJS,
    } });
    const compiled = {};
    new Function('require', 'exports', outputText)(name => ({ './SkillAssetsModel': assets, './EquipmentRefineModel': model })[name], compiled);
    const checked = compiled.validateRefineReceipt(request, equipmentRefine, quote);
    assert.equal(checked.boundGold.toString(), '1000');
    assert.deepEqual(checked.stones.map(row => [row.item.bound, row.quantity.toString()]), [[true, '1'], [true, '1']]);
    assert.equal(checked.target.refinePending.affixes[0].slot, 0);
});

test('分解石回执命中与未命中都钉原装备，拒绝重复发放和错误来源', () => {
    const award = { ...free, version: u(1), quantity: s(1), sourceId: assets.assetId(weapon.instanceId) };
    const rolls = roundTrip(40005, { recycleRefine: { rolls: [
        { equipmentInstanceId: weapon.instanceId, stone: award }, { equipmentInstanceId: id(27) },
    ] } }).recycleRefine.rolls;
    assert.deepEqual(receiptModel.recycleRefineResultLines(receiptModel.validateRecycleRefineReceipt(
        [weapon.instanceId, id(27)], { rolls }, stoneId)), ['获得非绑定洗炼石 1枚 · 装备分解', '本次分解未获得洗炼石']);
    assert.throws(() => receiptModel.validateRecycleRefineReceipt([weapon.instanceId], { rolls }), /数量无效/);
    assert.throws(() => receiptModel.validateRecycleRefineReceipt([weapon.instanceId, id(26)], { rolls }), /原装备/);
    assert.throws(() => receiptModel.validateRecycleRefineReceipt([weapon.instanceId], { rolls: [{ ...rolls[0], stone: { ...award, version } }] }), /来源无效/);
});

test('本地原操作按三动作保存严格字段，重开按同一协议原样重放', () => {
    const content = new Map();
    const storage = { getItem: key => content.get(key) ?? null, setItem: (key, value) => content.set(key, value),
        removeItem: key => content.delete(key) };
    const operation = { opId: id(7), commandSeq: u(100), expectedRevision: revision, expectedRuleVersion: u(30) };
    const ledger = new PendingOperations(storage);
    const target = model.freezeRefineTarget(weapon, true);
    for (const n of [1, 2, 3]) {
        const value = { action: n, target, operation, uiSessionId: ui, ...(n === 1 ? { quoteId, allowBound: true } : { pendingId }) };
        const role = 'role-' + n;
        ledger.set(role, { id: 40060, value });
        assert.deepEqual(new PendingOperations(storage).get(role), { id: 40060, value });
        const request = roundTrip(40060, value);
        assert.ok(request.target.version.equals(version));
        assert.equal(request.target.equipped, true);
        assert.equal(request.pendingId.length, n === 1 ? 0 : 16);
        assert.equal(request.quoteId.length, n === 1 ? 16 : 0);
        assert.throws(() => ledger.set('role-extra', { id: 40060, value: { ...value, neverSubmitted: 2 } }), /未允许/);
    }
});

const saved = JSON.parse(readFileSync(path.join(__dirname, '../../assets/scenes/Game.scene'), 'utf8'));
const savedNode = (name, list = saved) => list.find(row => row.__type__ === 'cc.Node' && row._name === name);
const savedChild = (parent, name) => savedNode(name, parent._children.map(ref => saved[ref.__id__]));
const savedComponent = (node, type) => node._components.map(ref => saved[ref.__id__]).find(ref => ref.__type__ === type);

test('左右列、原材料滚动、锁定和绑定开关全部保存在可编辑 Creator 布局', () => {
    const prior = require('./helpers/equipment_refine_layout.cjs')(structuredClone(saved)).slice(0, 2944);
    assert.equal(createHash('sha256').update(JSON.stringify(prior)).digest('hex'),
        '711749623846ea762509658df0e5b5abac5e757dc8a45d4a55341ac07632af58');
    const popup = savedChild(savedNode('PopupView'), 'Panels');
    const board = savedChild(popup, 'EquipmentRefine'), materials = savedChild(popup, 'EquipmentRefineMaterials');
    assert.ok(savedChild(popup, 'ItemActions'));
    assert.equal(board._active, false);
    assert.equal(materials._active, false);
    const left = savedChild(board, 'Current'), right = savedChild(board, 'Candidate');
    assert.ok(left._lpos.x < 0 && right._lpos.x > 0);
    for (let n = 1; n <= 4; ++n) {
        const leftRow = savedChild(left, 'Row' + n), rightRow = savedChild(right, 'Row' + n);
        assert.equal(leftRow._lpos.y, rightRow._lpos.y);
        assert.equal(savedComponent(savedChild(leftRow, 'Value'), 'cc.Label')._fontSize, 24);
        assert.equal(savedComponent(savedChild(rightRow, 'Value'), 'cc.Label')._fontSize, 24);
        assert.ok(savedComponent(savedChild(board, 'Lock' + n), 'cc.Toggle'));
    }
    assert.equal(savedComponent(savedChild(board, 'BoundToggle'), 'cc.Toggle')._isChecked, false);
    assert.ok(savedComponent(savedChild(materials, 'List'), 'cc.ScrollView'));
    assert.ok(savedComponent(savedChild(savedChild(materials, 'List'), 'View'), 'cc.Mask'));
    assert.equal(savedChild(board, 'CloseButton'), undefined);
});

function pageHarness(initial) {
    class Node extends EventEmitter {
        constructor(data) {
            super(); this.name = data._name; this.components = new Map(); this.children = new Map();
            for (const entry of data._children) { const node = new Node(saved[entry.__id__]); this.children.set(node.name, node); }
            for (const entry of data._components) {
                const raw = saved[entry.__id__];
                if (raw.__type__ === 'cc.Label') this.components.set(Label, new Label(raw._string));
                if (raw.__type__ === 'cc.Button') this.components.set(Button, new Button());
                if (raw.__type__ === 'cc.Toggle') this.components.set(Toggle, new Toggle());
            }
        }
        getChildByName(name) { return this.children.get(name); }
        getChildByPath(name) { return name.split('/').reduce((node, key) => node?.getChildByName(key), this); }
        getComponent(component) { return this.components.get(component); }
    }
    class Button { static EventType = { CLICK: 'click' }; }
    class Toggle { static EventType = { TOGGLE: 'toggle' }; setIsCheckedWithoutNotify(value) { this.checked = value; } }
    class Label { constructor(text) { this.string = text; } }
    class Color { constructor(...values) { this.values = values; } }
    class PopupView {
        static EVENT_CHANGED = 'change';
        constructor(root) { this.node = root; this.stack = []; this.answers = []; this.confirmed = []; }
        get top() { return this.stack.at(-1); }
        show(name) {
            const panel = this.node.getChildByPath('Panels/' + name);
            this.stack = this.stack.filter(item => item !== panel).concat(panel);
            this.node.emit(PopupView.EVENT_CHANGED, panel);
        }
        back() { this.stack.pop(); this.node.emit(PopupView.EVENT_CHANGED, this.top); }
        async confirm(_name, message, title) { this.confirmed.push({ message, title }); return this.answers.shift() ?? false; }
    }
    let materialView;
    class GameItemActions {
        constructor(_host, popup, _error, name) { this.popup = popup; this.name = name; }
        open(read) { materialView = read; this.popup.show(this.name); }
        refresh() {}
        dispose() {}
    }
    const { GameEquipmentRefine } = load('client/assets/scripts/app/GameEquipmentRefine.ts', key => ({
        cc: { Node, Label, Toggle, Button, Color, isValid: Boolean }, '../ui/PopupView': { PopupView }, './GameItemActions': { GameItemActions },
        './SkillAssetsModel': assets, './EquipmentRefineModel': model, './EquipmentRefineReceipt': receiptModel,
    })[key]);
    const safe = new Node(saved[savedNode('PopupView')._parent.__id__]), popup = new PopupView(safe.getChildByName('PopupView'));
    const board = popup.node.getChildByPath('Panels/EquipmentRefine');
    const submissions = [], errors = [];
    let snapshot = initial;
    const game = new GameEquipmentRefine(safe, popup, { snapshot: () => snapshot, newSession: () => ui,
        quote: async request => quoteFor(request, snapshot), submit: async (request, expectedRevision) => { submissions.push({ request, expectedRevision }); },
        tap() {}, error: error => errors.push(error),
    });
    return {
        game, popup, submissions, errors, material: () => materialView(), button: name => board.getChildByName(name).getComponent(Button),
        toggle: name => board.getChildByName(name).getComponent(Toggle), label: name => board.getChildByPath(name).getComponent(Label).string,
        press: name => board.getChildByName(name).emit('click'), change: name => board.getChildByName(name).emit('toggle'),
        update(next) { snapshot = next; game.refresh(); },
    };
}
const flush = () => new Promise(resolve => setImmediate(resolve));

test('比较页两次绑材确认，先到的回执不抢写槽位，候选等真实库存才可接受或放弃', async () => {
    const view = pageHarness(state);
    view.game.open(weapon.instanceId, false);
    assert.equal(view.label('Current/Row1/Value'), '空词条');
    assert.equal(view.label('Candidate/Row1/Value'), '待生成');
    view.popup.answers.push(true, false);
    view.change('BoundToggle');
    await flush();
    assert.equal(view.toggle('BoundToggle').checked, false);
    view.popup.answers.push(true, true);
    view.change('BoundToggle');
    await flush();
    assert.equal(view.toggle('BoundToggle').checked, true);
    view.change('BoundToggle');
    view.press('MaterialButton');
    assert.deepEqual(view.material().actions.map(action => action.label), ['查看报价', '清空选择']);
    assert.equal(view.material().options.length, 1);
    view.material().options[0].choose();
    await view.material().actions[0].run();
    assert.equal(view.material().title, '确认洗炼报价');
    await view.material().actions[0].run();
    const sent = view.submissions[0];
    assert.ok(sent.expectedRevision.equals(revision));
    const paid = { ...weapon, version: version.add(1), refinePending: { pendingId, affixes: [newAffix], lockedSlots: [] } };
    const receipt = { ...quoteFor(selectFree()), action: 1, pendingId, target: paid };
    view.game.accept(state.role, sent.request, view.game.reconcile(sent.request, receipt));
    assert.equal(view.button('ApplyButton').interactable, false);
    assert.equal(view.label('Candidate/Row1/Value'), '待生成');
    view.update({ ...state, revision: revision.add(2), items: [paid, bound] });
    assert.equal(view.button('ApplyButton').interactable, true);
    assert.match(view.label('Candidate/Row1/Value'), /最大生命 \+9\.6/);
    view.popup.answers.push(true);
    view.press('DiscardButton');
    await flush();
    assert.equal(view.submissions[1].request.action, 3);
    assert.ok(model.sameRefineId(view.submissions[1].request.pendingId, pendingId));
    assert.equal(view.submissions[1].expectedRevision, undefined);
    assert.deepEqual(view.errors, []);
});
