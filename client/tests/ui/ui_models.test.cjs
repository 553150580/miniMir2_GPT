const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const output = path.resolve(__dirname, '../../../.work/ui-tests');
const { PanelStackState } = require(path.join(output, 'ui/PanelStackState.js'));
const { visibleRange } = require(path.join(output, 'ui/VirtualListWindow.js'));
const { NavigationIntent, direction8 } = require(path.join(output, 'input/NavigationIntent.js'));

test('普通面板和覆盖弹窗分别返回，已有面板与滚动数据保持同一引用', () => {
    const ui = new PanelStackState();
    const popup = new PanelStackState();
    const inventory = { name: '背包', scrollOffset: 384 };
    const detail = { name: '详情' };
    const help = { name: '帮助' };
    ui.open(inventory);
    popup.open(detail);
    popup.open(help);
    assert.equal(popup.back(), help);
    assert.equal(popup.top, detail);
    assert.equal(ui.top, inventory);
    assert.equal(popup.back(), detail);
    assert.equal(inventory.scrollOffset, 384);
    assert.equal(ui.back(), inventory);
    assert.equal(popup.back(), null);
});

test('重复打开已存在面板只移动到栈顶，不重复保存或清理数据', () => {
    const stack = new PanelStackState();
    const first = { selectedTab: 2 };
    const second = { scrollOffset: 128 };
    stack.open(first);
    stack.open(second);
    stack.open(first);
    assert.equal(stack.size, 2);
    assert.equal(stack.back(), first);
    assert.equal(stack.top, second);
    const snapshot = stack.items;
    snapshot.length = 0;
    assert.equal(stack.size, 1);
    stack.clear();
    assert.equal(stack.top, null);
    assert.equal(second.scrollOffset, 128);
});

test('百万条数据只申请视口及缓冲范围，首尾和滚动中间索引正确', () => {
    assert.deepEqual(visibleRange(1_000_000, 0, 640, 64, 2), { start: 0, end: 12 });
    assert.deepEqual(visibleRange(1_000_000, 3200, 640, 64, 2), { start: 48, end: 62 });
    assert.deepEqual(visibleRange(1_000_000, Infinity, 640, 64, 2), { start: 0, end: 12 });
    assert.deepEqual(visibleRange(1_000_000, Number.MAX_SAFE_INTEGER, 640, 64, 2),
        { start: 999988, end: 1_000_000 });
    assert.deepEqual(visibleRange(2, 0, 640, 64, 2), { start: 0, end: 2 });
});

test('部分可见条目、空视口和异常输入具有有界结果', () => {
    assert.deepEqual(visibleRange(100, 33, 100, 64, 0), { start: 0, end: 3 });
    assert.deepEqual(visibleRange(100, -200, 100, 64, 0), { start: 0, end: 2 });
    assert.deepEqual(visibleRange(0, 500, 100, 64), { start: 0, end: 0 });
    assert.deepEqual(visibleRange(100, 500, 0, 64), { start: 0, end: 0 });
    assert.deepEqual(visibleRange(NaN, 500, 100, 64), { start: 0, end: 0 });
    assert.throws(() => visibleRange(10, 0, 100, 0), /高度/);
    assert.throws(() => visibleRange(10, 0, 100, NaN), /高度/);
    for (let sample = 0; sample < 200; ++sample) {
        const count = sample * 97 + 1;
        const height = sample * 7 + 1;
        const offset = sample * 1003;
        const range = visibleRange(count, offset, height, 64, 2);
        assert.ok(range.start >= 0 && range.end <= count && range.end >= range.start);
        assert.ok(range.end - range.start <= Math.ceil(height / 64) + 5);
    }
});

test('摇杆稳定映射八方向，死区和非法输入不会产生方向', () => {
    for (const [x, y] of [[1, 0], [1, 1], [0, 1], [-1, 1], [-1, 0], [-1, -1], [0, -1], [1, -1]]) {
        assert.deepEqual(direction8(x, y), { x, y });
    }
    assert.deepEqual(direction8(1, 0.1), { x: 1, y: 0 });
    assert.deepEqual(direction8(0.1, 0.1), { x: 0, y: 0 });
    assert.deepEqual(direction8(NaN, 1), { x: 0, y: 0 });
    assert.deepEqual(direction8(1, Infinity), { x: 0, y: 0 });
    assert.deepEqual(direction8(0.1, 0, NaN), { x: 0, y: 0 });
});

test('面板、实体操作及后台尺寸失焦释放不会清除地面目标', () => {
    const nav = new NavigationIntent();
    const ordinary = new PanelStackState();
    const popup = new PanelStackState();
    assert.equal(nav.setGroundTarget({ x: 5, y: 8 }), true);
    ordinary.open({ name: '聊天' });
    popup.open({ name: '实体详情' });
    nav.releaseJoystick();
    nav.releaseJoystick();
    popup.back();
    ordinary.back();
    nav.setJoystick({ x: 0, y: 0 });
    assert.deepEqual(nav.state, { target: { x: 5, y: 8 }, direction: { x: 0, y: 0 } });
});

test('主动推方向终止导航，松手只释放方向而不恢复已终止的目标', () => {
    const nav = new NavigationIntent();
    const reasons = [];
    nav.subscribe((_state, reason) => reasons.push(reason));
    nav.setGroundTarget({ x: 5, y: 8 });
    nav.setJoystick({ x: 1, y: 1 });
    assert.equal(nav.state.target, null);
    nav.releaseJoystick();
    assert.deepEqual(nav.state, { target: null, direction: { x: 0, y: 0 } });
    assert.deepEqual(reasons, ['ground', 'direction', 'release']);
});

test('到达回执只结束对应目标，旧路径回执不能清除新目标', () => {
    const nav = new NavigationIntent();
    nav.setGroundTarget({ x: 1, y: 2 });
    nav.bindGroundInput(1);
    nav.setGroundTarget({ x: 3, y: 4 });
    nav.bindGroundInput(2);
    assert.equal(nav.observeAuthority({ inputSequence: 1, destination: { x: 1, y: 2 }, status: 2, revision: '1' }), false);
    assert.deepEqual(nav.state.target, { x: 3, y: 4 });
    assert.equal(nav.observeAuthority({ inputSequence: 2, destination: { x: 3, y: 4 }, status: 2, revision: '2' }), true);
    assert.equal(nav.state.target, null);
    nav.setGroundTarget({ x: 9, y: 9 });
    nav.exitGame();
    assert.deepEqual(nav.state, { target: null, direction: { x: 0, y: 0 } });
});

test('意图快照、非法目标和观察者异常不污染当前导航', () => {
    const nav = new NavigationIntent();
    const input = { x: 4, y: 6 };
    const events = [];
    nav.subscribe(() => { throw new Error('界面观察者异常'); });
    const unsubscribe = nav.subscribe((state, reason) => events.push({ state, reason }));
    nav.setGroundTarget(input);
    input.x = 100;
    const snapshot = nav.state;
    snapshot.target.x = 200;
    assert.deepEqual(nav.state.target, { x: 4, y: 6 });
    assert.equal(nav.setGroundTarget({ x: 0.1, y: 8 }), false);
    assert.equal(nav.setGroundTarget({ x: Infinity, y: 8 }), false);
    nav.setJoystick({ x: NaN, y: 0 });
    assert.deepEqual(nav.state.target, { x: 4, y: 6 });
    unsubscribe();
    nav.exitGame();
    assert.equal(events.length, 1);
});

test('status5只结束匹配input/revision/格子的导航，旧失败不能清新目标且终态只通知一次',()=>{
    const nav=new NavigationIntent(),reasons=[];nav.subscribe((_value,reason)=>reasons.push(reason));
    nav.setGroundTarget({x:633,y:613});nav.bindGroundInput(8);
    nav.observeAuthority({inputSequence:8,destination:{x:633,y:613},status:1,revision:'9007199254740993'});
    assert.equal(nav.observeAuthority({inputSequence:8,destination:{x:633,y:613},status:5,revision:'9007199254740994'}),false);
    assert.equal(nav.observeAuthority({inputSequence:7,destination:{x:633,y:613},status:5,revision:'9007199254740993'}),false);
    // 更高但身份不匹配的推送仍是全局修订水位，不能接受随后的低修订。
    assert.deepEqual(nav.state.target,{x:633,y:613});
    nav.setGroundTarget({x:640,y:620});nav.bindGroundInput(9);
    assert.equal(nav.observeAuthority({inputSequence:8,destination:{x:633,y:613},status:5,revision:'9007199254740994'}),false);
    assert.equal(nav.observeAuthority({inputSequence:9,destination:{x:640,y:620},status:5,revision:'9007199254740995'}),true);
    assert.equal(nav.state.target,null);assert.equal(nav.observeAuthority({inputSequence:9,destination:{x:640,y:620},status:5,revision:'9007199254740995'}),false);
    assert.equal(reasons.filter(value=>value==='unreachable').length,1);assert.equal(reasons.includes('arrived'),false);
});
