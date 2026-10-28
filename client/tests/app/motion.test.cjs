const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;
const file = path.resolve(__dirname, '../../assets/scripts/app/EntityMotion.ts');
const result = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
});
const compiled = { exports: {} };
new Function('module', 'exports', result.outputText)(compiled, compiled.exports);
const { EntityMotion } = compiled.exports;
const target = (x, y = 0, durationMs = 600, direction = 2, extra = {}) => ({ x, y, durationMs, direction, ...extra });
const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 1e-8, `${actual} != ${expected}`);

test('首次出现立即就位，邻格中点与终点按输入时长插值', () => {
    for (const duration of [600, 1000, 1500]) {
        const motion = new EntityMotion();
        const first = motion.push(target(5, 6, duration), 0);
        assert.deepEqual([first.x, first.y, first.moving, first.stepProgress], [5, 6, false, 0]);
        const start = motion.push(target(6, 6, duration), 200);
        assert.equal(start.moving, true);
        const middle = motion.sample(200 + duration / 2);
        near(middle.x, 5.5); near(middle.y, 6); near(middle.stepProgress, .5);
        assert.equal(middle.stepSequence, start.stepSequence);
        const end = motion.sample(200 + duration);
        assert.deepEqual([end.x, end.y, end.moving, end.stepProgress], [6, 6, false, 1]);
    }
});

test('200ms重复standing坐标和方向更新不重启当前步', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0);
    const start = motion.push(target(1), 200);
    const before = motion.sample(400);
    const repeat = motion.push(target(1, 0, 600, 4), 400);
    near(repeat.x, before.x); near(repeat.stepProgress, before.stepProgress);
    assert.equal(repeat.stepSequence, start.stepSequence);
    assert.equal(repeat.direction, 2);
    const zeroDurationRepeat = motion.push(target(1, 0, 0, 6), 600);
    assert.equal(zeroDurationRepeat.stepSequence, start.stepSequence);
    near(zeroDurationRepeat.stepProgress, 2 / 3);
    const end = motion.sample(800);
    assert.equal(end.moving, false); assert.equal(end.direction, 6);
    assert.equal(end.x, 1); assert.equal(end.stepSequence, start.stepSequence);
});

test('连续转弯保留权威拐点，不斜切拐角，当前段朝向和步进度稳定', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0);
    const first = motion.push(target(1, 0, 600), 200);
    const before = motion.sample(400);
    const turn = motion.push(target(1, 1, 600, 4), 400);
    near(turn.x, before.x); near(turn.stepProgress, before.stepProgress);
    assert.equal(turn.stepSequence, first.stepSequence);
    assert.equal(turn.direction, 2);
    // 剩余东向步长2/3与下一整步按2:3分配200ms，480ms恰好到已知拐角。
    const corner = motion.sample(480);
    near(corner.x, 1); near(corner.y, 0);
    assert.equal(corner.direction, 4); assert.ok(corner.stepSequence > first.stepSequence);
    const after = motion.sample(520);
    near(after.x, 1); near(after.y, 1 / 3);
    assert.equal(after.direction, 4);
    const end = motion.sample(600);
    assert.deepEqual([end.x, end.y, end.moving], [1, 1, false]);
});

test('多次提前快照追赶时保持位置连续并在最后目标的有界窗口完成', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0);
    motion.push(target(1), 200);
    for (let step = 2; step <= 8; ++step) {
        const now = step * 200;
        const before = motion.sample(now);
        const pushed = motion.push(target(step), now);
        near(pushed.x, before.x);
        assert.ok(pushed.x <= step && pushed.x >= 0);
    }
    const end = motion.sample(1800);
    assert.equal(end.x, 8); assert.equal(end.moving, false);
    assert.equal(motion.sample(5000).x, 8);
});

test('快照延迟时停止在已知目标，迟到邻格从停点开始而不外推', () => {
    const motion = new EntityMotion({ resumeGapMs: 5000 });
    motion.push(target(0), 0); motion.push(target(1), 200);
    assert.equal(motion.sample(1500).x, 1);
    const next = motion.push(target(2), 1600);
    assert.equal(next.x, 1); assert.equal(next.moving, true);
    near(motion.sample(1900).x, 1.5);
    assert.equal(motion.sample(10000).x, 2);
});

test('传送、复活和长后台空窗立即重置，snap不显示半截步', () => {
    for (const mode of ['teleport', 'revive', 'background']) {
        const motion = new EntityMotion();
        motion.push(target(0), 0); motion.push(target(1), 200); motion.sample(400);
        const snap = mode === 'teleport' ? motion.push(target(9, 9), 400)
            : mode === 'revive' ? motion.push(target(1, 0, 600, 4, { snap: true }), 400)
                : motion.push(target(2), 2500);
        assert.equal(snap.moving, false); assert.equal(snap.stepProgress, 0);
        assert.equal(snap.x, mode === 'teleport' ? 9 : mode === 'revive' ? 1 : 2);
    }
    const missingPath = new EntityMotion();
    missingPath.push(target(0), 0);
    // 只知道两个非邻格端点时不能推断绕墙路径。
    const acrossUnknownCells = missingPath.push(target(2, 1), 200);
    assert.equal(acrossUnknownCells.moving, false);
    assert.deepEqual([acrossUnknownCells.x, acrossUnknownCells.y], [2, 1]);
});

test('30fps和60fps按同一绝对时刻得到同样位置、方向及步动画进度', () => {
    function run(fps) {
        const motion = new EntityMotion();
        motion.push(target(0), 0); motion.push(target(1), 200);
        for (let frame = 1; frame / fps * 1000 < 500; ++frame) motion.sample(200 + frame / fps * 1000);
        const middle = motion.sample(700);
        motion.push(target(1, 1, 600, 4), 800);
        for (let frame = 1; frame / fps * 1000 < 300; ++frame) motion.sample(800 + frame / fps * 1000);
        return [middle, motion.sample(1100), motion.sample(1400)];
    }
    assert.deepEqual(run(30), run(60));
});

test('八方向显示使用x右y下的0..7约定，走完后服从权威朝向', () => {
    const directions = [[0,-1],[1,-1],[1,0],[1,1],[0,1],[-1,1],[-1,0],[-1,-1]];
    directions.forEach(([x,y], index) => {
        const motion = new EntityMotion(); motion.push(target(0), 0);
        const frame = motion.push(target(x, y, 600, 4), 200);
        assert.equal(frame.direction, index);
        assert.equal(motion.sample(800).direction, 4);
    });
});

test('密集积压有队列上限，reset和非法输入不产生NaN或残留运动', () => {
    const motion = new EntityMotion({ maxQueuedWaypoints: 2 });
    motion.push(target(0), 0);
    motion.push(target(1, 0), 200); motion.push(target(1, 1), 200);
    const snapped = motion.push(target(2, 1), 200);
    assert.equal(snapped.moving, false); assert.equal(snapped.x, 2);
    motion.reset();
    assert.deepEqual(motion.sample(0), { x: 0, y: 0, moving: false, direction: 0, stepSequence: 0, stepProgress: 0, movementKind: 0, dashPhase: 0 });
    assert.throws(() => motion.push(target(NaN), 0));
    assert.throws(() => motion.push(target(0, 0, -1), 0));
    assert.throws(() => motion.push(target(0, 0, 600, 8), 0));
    assert.throws(() => motion.sample(Infinity));
});

test('攻击起步门槛700ms之前不前移，截止后按600ms完整走一步', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0); motion.sample(100); motion.holdUntil(700);
    const queued = motion.push(target(1), 200);
    for (const now of [200, 400, 699]) {
        const frame = motion.sample(now);
        assert.equal(frame.x, 0); assert.equal(frame.moving, false); assert.equal(frame.stepProgress, 0);
        assert.equal(frame.stepSequence, queued.stepSequence);
    }
    const start = motion.sample(700);
    assert.equal(start.moving, true); assert.equal(start.x, 0);
    near(motion.sample(1000).x, .5);
    assert.deepEqual([motion.sample(1300).x, motion.sample(1300).moving], [1, false]);
});

test('每帧重复hold同一700ms不延长，门槛期间多拐点保留独立步时长', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0); motion.sample(100); motion.holdUntil(700);
    motion.push(target(1, 0, 600), 200);
    motion.push(target(1, 1, 1000, 4), 400);
    for (const now of [400, 500, 600, 699]) { motion.sample(now); motion.holdUntil(700); }
    near(motion.sample(1000).x, .5);
    const corner = motion.sample(1300);
    near(corner.x, 1); near(corner.y, 0); assert.equal(corner.direction, 4);
    near(motion.sample(1800).y, .5);
    const end = motion.sample(2300);
    assert.deepEqual([end.x, end.y, end.moving], [1, 1, false]);
});

test('复活snap清除起步门槛，新步不继续等待旧攻击', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0); motion.sample(100); motion.holdUntil(700);
    motion.push(target(1), 200);
    const revived = motion.push(target(5, 0, 600, 4, { snap: true }), 300);
    assert.equal(revived.moving, false); assert.equal(revived.stepProgress, 0);
    assert.equal(motion.push(target(6), 400).moving, true);
    near(motion.sample(700).x, 5.5);
});

test('releaseHold只去掉剩余等待，400起步、700中点并保留各拐点', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0); motion.sample(100); motion.holdUntil(700);
    motion.push(target(1), 200); motion.push(target(1, 1, 600, 4), 300);
    const released = motion.releaseHold(400);
    assert.equal(released.x, 0); assert.equal(released.moving, true);
    near(motion.sample(700).x, .5);
    const corner = motion.sample(1000);
    near(corner.x, 1); near(corner.y, 0); assert.equal(corner.direction, 4);
    near(motion.sample(1300).y, .5);
    assert.equal(motion.sample(1600).moving, false);
});

test('hold和release不冻结已开始的步，也不会把后续段提前到前段之前', () => {
    const motion = new EntityMotion();
    motion.push(target(0), 0); motion.push(target(1), 200);
    near(motion.sample(400).x, 1 / 3);
    motion.holdUntil(1000);
    motion.push(target(1, 1, 600, 4), 500);
    near(motion.sample(500).x, .5);
    near(motion.releaseHold(600).x, 2 / 3);
    const corner = motion.sample(800);
    near(corner.x, 1); near(corner.y, 0);
    near(motion.sample(1100).y, .5);
    assert.equal(motion.sample(1400).moving, false);
    for (const bad of [NaN, Infinity, -1]) {
        assert.throws(() => motion.holdUntil(bad));
        assert.throws(() => motion.releaseHold(bad));
    }
});
