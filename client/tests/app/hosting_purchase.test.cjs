const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { root, Long, load } = require('./helpers/session_harness.cjs');

const assets = load('client/assets/scripts/app/SkillAssetsModel.ts', () => ({}));
const model = load('client/assets/scripts/app/HostingPurchaseModel.ts', () => assets);
const code = { CodeHostingPurchaseUnavailable: 3801, CodeHostingPurchaseBalance: 3803, CodeHostingPurchaseFunds: 3804 };
const cc = { Label: class {}, Button: class {} };
const { GameHosting } = load('client/assets/scripts/app/GameHosting.ts', name => name === 'cc' ? cc : name.endsWith('protocol_ids') ? code : model);

const id = new Uint8Array(16).fill(35);
const epoch = Long.fromString('9007199254740993', true);
const fixtureContext = () => ({ scope: 'original', role: 'roleA', level: 11, sceneEpoch: epoch, ruleVersion: Long.fromInt(30, true),
    revision: Long.fromInt(4, true), balance: 86399, boundGold: Long.fromInt(10), gold: Long.fromInt(10_000), pending: false });

function quote(context, seconds = 30) {
    const step = 15, reference = 1801n, rate = reference * 2n;
    const total = (rate * BigInt(seconds) + 3599n) / 3600n;
    const bound = BigInt(context.boundGold.toString()), spend = bound < total ? bound : total;
    return { quoteId: seconds ? new Uint8Array(id) : new Uint8Array(), sceneEpoch: context.sceneEpoch, roleRevision: context.revision,
        ruleVersion: context.ruleVersion, configHash: 'current', quoteVersion: Long.fromInt(71, true), bandMinLevel: 1, bandMaxLevel: 19,
        granularitySeconds: step, requestedSeconds: seconds, referenceGoldPerHour: reference, goldPerHour: rate, totalGold: total,
        beforeSeconds: context.balance, boundGoldCost: spend, goldCost: total - spend, expiresAtUnixMs: 1795610853163 };
}

test('用精确秒数校验2倍毛金币、整笔向上取整及绑金优先，累计可超过24小时', () => {
    const context = fixtureContext(), snapshot = quote(context), request = model.validateHostingQuote(snapshot, context, 30);
    assert.deepEqual(request, { requestedSeconds: 30, quoteId: id });
    assert.notEqual(request.quoteId, snapshot.quoteId);
    assert.equal(model.hostingDuration(context.balance), '23小时59分59秒');
    assert.match(model.hostingQuoteMessage(snapshot), /1–19级 · 报价版本 71\n标准毛金币 G_ref：1801金币\/小时\n每小时售价（2倍）：3602金币\n购买 30秒 · 合计 31金币\n先扣绑定金币 10，再扣金币 21/);
    const receipt = { quote: snapshot, balanceSeconds: 86429 };
    assert.match(model.hostingReceiptMessage(receipt, request), /已购 30秒 · 余额 24小时29秒\n扣绑定金币 10、金币 21 · 报价版本 71/);
    assert.throws(() => model.hostingReceiptMessage({ ...receipt, balanceSeconds: 86430 }, request), /到账回执不完整/);
    assert.throws(() => model.hostingReceiptMessage(receipt, { ...request, quoteId: new Uint8Array(16) }), /与原报价不一致/);
    assert.throws(() => model.validateHostingQuote({ ...snapshot, beforeSeconds: 86400 }, { ...context, balance: 86400 }, 30), /报价或到账金额无效/);
    assert.throws(() => model.validateHostingQuote({ ...snapshot, goldCost: 22n, boundGoldCost: 9n }, context, 30), /钱包已变化/);
    assert.throws(() => model.validateHostingQuote(snapshot, { ...context, ruleVersion: Long.fromInt(31, true) }, 30), /重新报价/);
});

test('报价与回执完整保留超过Number安全范围的金币和版本', () => {
    const context = fixtureContext(), reference = 9007199254740993n;
    context.boundGold = Long.fromString('9007199254740993');
    context.gold = Long.fromString('9007199254740995');
    const version = Long.fromString('9007199254748027', true);
    const snapshot = { ...quote(context, 3600), referenceGoldPerHour: reference, goldPerHour: reference * 2n,
        totalGold: reference * 2n, boundGoldCost: reference, goldCost: reference, quoteVersion: version };
    const request = model.validateHostingQuote(snapshot, context, 3600);
    assert.match(model.hostingQuoteMessage(snapshot), /9007199254748027.*\n.*9007199254740993.*\n.*18014398509481986/s);
    assert.match(model.hostingReceiptMessage({ quote: snapshot, balanceSeconds: context.balance + 3600 }, request), /扣绑定金币 9007199254740993、金币 9007199254740993/);
});

function viewFixture(discoveryCode = 0) {
    const elements = new Map(), requests = [], buys = [], context = fixtureContext();
    const getChildByPath = key => {
        if (!elements.has(key)) {
            const component = { interactable: false, string: '' };
            elements.set(key, { active: false, getComponent: () => component });
        }
        return elements.get(key);
    };
    const panel = { activeInHierarchy: true, getChildByPath };
    let confirm = () => false;
    const view = new GameHosting(panel, { confirm: (...args) => Promise.resolve(confirm(...args)) }, () => context, {
        quote: (_, seconds) => { requests.push(seconds); return Promise.resolve({ ...quote(context, seconds), errorCode: seconds ? 0 : discoveryCode }); },
        buy: async request => { buys.push(request); }, reason: value => value === 3801 ? '当前规则未开放金币补时' : `错误 ${value}`,
    });
    return { view, context, panel, requests, buys, getChildByPath, setConfirm: value => { confirm = value; } };
}

test('缺少生效价格时买时控件保持隐藏，重复刷新不发重复发现请求', async () => {
    const f = viewFixture(code.CodeHostingPurchaseUnavailable);
    f.view.open(); f.view.refresh(120); f.view.refresh(120);
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(f.requests, [0]);
    assert.equal(f.getChildByPath('BuyQuote').active, false);
    assert.match(f.getChildByPath('Message').getComponent().string, /剩余托管 2分.*\n.*\n当前规则未开放金币补时/);
    await f.view.confirmPurchase();
    assert.deepEqual(f.buys, []);
});

test('真实控制器只在确认后提交冻结报价，取消或确认期间原资产变化不购买', async () => {
    const f = viewFixture();
    f.view.open(); f.view.refresh(86399);
    await new Promise(resolve => setImmediate(resolve));
    f.view.adjust(1);
    assert.equal(f.getChildByPath('BuyQuantity').getComponent().string, '购买 30秒');
    await f.view.confirmPurchase();
    assert.deepEqual(f.requests, [0, 30]);
    assert.equal(f.buys.length, 0);
    f.setConfirm(() => { f.context.revision = f.context.revision.add(1); return true; });
    await assert.rejects(f.view.confirmPurchase(), /原角色、档位或钱包已变化/);
    assert.equal(f.buys.length, 0);
    f.setConfirm((_panel, message, title) => {
        assert.match(message, /金币\/小时/);
        assert.equal(title, '金币补时确认');
        return true;
    });
    await f.view.confirmPurchase();
    assert.deepEqual(f.buys, [{ requestedSeconds: 30, quoteId: id }]);
    f.context.balance = 86429;
    f.view.acceptReceipt(f.context.role, '原补时回执'); f.view.refresh(f.context.balance);
    assert.equal(f.getChildByPath('BuyQuote').getComponent().interactable, false);
    assert.equal(f.getChildByPath('BuyReceipt').getComponent().string, '原补时回执');
    f.context.role = 'anotherRole'; f.view.refresh(0);
    assert.equal(f.getChildByPath('BuyReceipt').active, false);
});

test('托管加减、确认入口和回执都保存在可编辑场景且默认隐藏', () => {
    const scene = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scenes/Game.scene'), 'utf8'));
    const panel = scene.find(node => node._name === 'Hosting' && node._children?.map(ref => scene[ref.__id__]._name).includes('Claim'));
    assert(panel);
    const child = name => panel._children.map(ref => scene[ref.__id__]).find(node => node._name === name);
    for (const name of ['BuyMinus', 'BuyPlus', 'BuyQuote']) {
        const node = child(name), components = node._components.map(ref => scene[ref.__id__]);
        assert.equal(node._active, false);
        assert(components.some(component => component.__type__ === 'cc.Button' && !component._interactable));
    }
    for (const name of ['BuyQuantity', 'BuyReceipt']) {
        const node = child(name), label = node._components.map(ref => scene[ref.__id__]).find(component => component.__type__ === 'cc.Label');
        assert.equal(node._active, false);
        assert(label._fontSize >= 24);
    }
    assert(child('Claim')._active && child('Leave')._active);
});
