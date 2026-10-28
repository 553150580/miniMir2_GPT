const { test } = require('node:test');
const assert = require('node:assert/strict');
const { SystemMessageLog } = require('../../../.work/ui-tests/app/SystemMessageLog.js');
const { protocolErrorMessage } = require('../../../.work/ui-tests/app/GameErrors.js');

function state(overrides = {}) {
    return {
        roleIdentity: '1:role-a', role: { name: '道士甲', level: 1 }, ready: true, lastError: '',
        entry: { playerHandle: 1, sceneEpoch: '1', baselineId: '1' }, map: { name: '初行之地' },
        inventory: { level: 1, experience: '0', items: [{ itemId: 35, quantity: '5', bound: true }], equipped: [] },
        wallet: { gold: '0', boundGold: '10', yuanbaoUnits: '0', boundYuanbaoUnits: '0' },
        baseline: { entities: [{ entityHandle: 1, entityKind: 1, hp: '20' }] },
        hosting: { remainHostingSec: 21600, autoBattleEnabled: true },
        gameplay: { items: [{ item_id: 35, name: '小红药' }, { item_id: 218, name: '木剑' }] },
        ...overrides,
    };
}

test('首次快照只记入场，重复快照不重新产生系统消息', () => {
    const log = new SystemMessageLog();
    const snapshot = state();
    assert.equal(log.observe(snapshot), true);
    assert.deepEqual(log.messages.map(item => item.title), ['进入地图']);
    const version = log.version;
    for (let i = 0; i < 30; i++) assert.equal(log.observe(state()), false);
    assert.equal(log.version, version);
    assert.match(log.messages[0].message, /道士甲.*初行之地/);
});

test('物品和四钱包保持64位精度，元宝按十分单位显示', () => {
    const log = new SystemMessageLog();
    const initial = state({ wallet: { gold: '9007199254740993', boundGold: '10', yuanbaoUnits: '11', boundYuanbaoUnits: '0' } });
    log.observe(initial);
    log.observe({ ...initial, wallet: { gold: '9007199254740994', boundGold: '60', yuanbaoUnits: '9', boundYuanbaoUnits: '100' },
        inventory: { ...initial.inventory, items: [{ itemId: 35, quantity: '9007199254740998', bound: true }] } });
    const text = log.messages.map(item => item.message).join('\n');
    assert.match(text, /小红药 ×9007199254740993/);
    assert.match(text, /金币 \+1（当前 9007199254740994）/);
    assert.match(text, /绑定金币 \+50（当前 60）/);
    assert.match(text, /元宝 -0\.2（当前 0\.9）/);
    assert.match(text, /绑定元宝 \+10\.0（当前 10\.0）/);
});

test('换装容器移动不误报物品减少，真实减少和经验进度有据可查', () => {
    const log = new SystemMessageLog();
    const sword = { itemId: 218, quantity: '1', bound: true };
    const initial = state({ inventory: { level: 1, experience: '15', items: [sword], equipped: [] } });
    log.observe(initial);
    log.observe({ ...initial, inventory: { level: 1, experience: '25', items: [], equipped: [{ item: sword }] } });
    assert.deepEqual(log.messages.map(item => item.title), ['进入地图', '经验获得']);
    assert.equal(log.messages[1].message, '经验 +10');
    log.observe({ ...initial, inventory: { level: 2, experience: '5', items: [], equipped: [] } });
    assert.deepEqual(log.messages.slice(-3).map(item => item.title), ['角色升级', '经验进度', '物品减少']);
    assert.equal(log.messages.at(-2).message, '2 级，当前经验 5');
});

test('断线和入场身份换代重建观察基线，不推算离线资产收益', () => {
    const log = new SystemMessageLog();
    log.observe(state());
    log.observe(state({ ready: false, lastError: '连接已关闭' }));
    log.observe(state({ ready: false, lastError: '连接已关闭' }));
    log.observe(state({ wallet: { boundGold: '10000' }, inventory: { level: 9, experience: '9900', items: [] } }));
    assert.deepEqual(log.messages.map(item => item.title), ['进入地图', '提示', '连接状态', '连接已恢复']);
    log.observe(state({ entry: { playerHandle: 1, sceneEpoch: '2', baselineId: '3' }, wallet: { boundGold: '15000' } }));
    assert.equal(log.messages.at(-1).title, '连接已恢复');
    assert.equal(log.messages.some(item => item.title === '余额更新' || item.title === '角色升级'), false);
});

test('未就绪快照不产生收益，迟到的首次余额仍作为观察基线', () => {
    const log = new SystemMessageLog();
    log.observe(state({ ready: false }));
    log.observe(state({ ready: false, wallet: { boundGold: '1000' } }));
    assert.equal(log.messages.length, 0);
    log.observe(state({ inventory: null, wallet: null, baseline: null, hosting: null }));
    log.observe(state({ wallet: { boundGold: '5000' } }));
    assert.equal(log.messages.length, 1);
});

test('死亡恢复、自动状态及托管时长只记录真实边沿变化', () => {
    const log = new SystemMessageLog();
    log.observe(state());
    const dead = state({ baseline: { entities: [{ entityHandle: 1, entityKind: 1, hp: '0' }] }, hosting: { remainHostingSec: 43200, autoBattleEnabled: false } });
    log.observe(dead); log.observe(dead);
    assert.deepEqual(log.messages.slice(1).map(item => item.title), ['角色倒下', '托管时长更新', '战斗设置']);
    log.observe({ ...dead, baseline: { entities: [{ entityHandle: 1, entityKind: 1, hp: '16' }] } });
    assert.equal(log.messages.at(-1).title, '角色恢复');
});
test('系统死亡消息使用入场owner句柄，其它kind1倒下不产生本人的死亡提示',()=>{
    const log=new SystemMessageLog(),base=state(),entities=[{entityHandle:2,entityKind:1,hp:'20'},base.baseline.entities[0]];
    log.observe({...base,baseline:{entities}});log.observe({...base,baseline:{entities:[{...entities[0],hp:'0'},entities[1]]}});
    assert(!log.messages.some(row=>row.title==='角色倒下'));
    log.observe({...base,baseline:{entities:[{...entities[0],hp:'0'},{...entities[1],hp:'0'}]}});assert.equal(log.messages.at(-1).title,'角色倒下');
});

test('战报按真实结算身份去重，重连同回执不重复，切角清除旧消息', () => {
    const log = new SystemMessageLog();
    const report = { hostingSessionId: new Uint8Array([1, 2]), settlementSequence: '5', usedSeconds: '31', kills: '2', experience: '30' };
    log.observe(state({ report }));
    log.observe(state({ report: { ...report } }));
    log.observe(state({ ready: false }));
    log.observe(state({ report }));
    assert.equal(log.messages.filter(item => item.title === '托管战报').length, 1);
    assert.match(log.messages.find(item => item.title === '托管战报').message, /31 秒，击杀 2，经验 30/);
    log.observe(state({ roleIdentity: '1:role-b', role: { name: '战士乙' } }));
    assert.equal(log.messages.length, 1);
    assert.match(log.messages[0].message, /战士乙/);
});

test('日志有界且可按中文检索，不改变已存消息或产生伪时间', () => {
    const log = new SystemMessageLog(3);
    log.observe(state());
    for (let i = 1; i <= 5; i++) log.observe(state({ lastError: `操作提示 ${i}` }));
    assert.equal(log.messages.length, 3);
    assert.deepEqual(log.messages.map(item => item.message), ['操作提示 3', '操作提示 4', '操作提示 5']);
    assert.equal(log.filter('  提示 4 ')[0].message, '操作提示 4');
    assert.equal(log.filter('世界聊天').length, 0);
    assert.equal(log.messages.length, 3);
    assert.deepEqual(Object.keys(log.messages[0]), ['id', 'title', 'message']);
});

test('自动战斗满包拒绝显示解决办法，其他协议上下文不误报满包', () => {
    assert.equal(protocolErrorMessage(30004, 2103), '背包已满，整理后再开启自动战斗');
    assert.equal(protocolErrorMessage(30004, 1001), '操作失败，错误码1001');
    assert.equal(protocolErrorMessage(10001, 2103), '操作失败，错误码2103');
});

test('一次满包错误的订阅与请求回执不重复刷屏，旧两种文案也归并', () => {
    const log = new SystemMessageLog();
    log.observe(state());
    for (let index = 0; index < 30; index++) {
        log.observe(state({ lastError: '服务端拒绝：2103' }));
        log.observe(state({ lastError: '操作失败，错误码2103' }));
        log.observe(state({ lastError: protocolErrorMessage(30004, 2103) }));
    }
    assert.deepEqual(log.messages.filter(item => item.title === '提示').map(item => item.message),
        ['背包已满，整理后再开启自动战斗']);
    log.observe(state({ lastError: '连接已关闭' }));
    assert.equal(log.messages.at(-1).message, '连接已关闭');
});

test('无法到达消息按原导航身份去重，切角清理去重状态，不冒充已到达',()=>{
    const log=new SystemMessageLog();log.observe(state());assert.equal(log.navigationUnreachable('role:binding:scene:revision:input'),true);
    assert.equal(log.navigationUnreachable('role:binding:scene:revision:input'),false);assert.equal(log.navigationUnreachable(''),false);
    assert.equal(log.messages.at(-1).message,'当前无法到达该位置');assert.equal(log.messages.filter(value=>value.title==='导航').length,1);
    log.observe(state({roleIdentity:'1:role-b'}));assert.equal(log.navigationUnreachable('role:binding:scene:revision:input'),true);
});
