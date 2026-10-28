const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;
function load(name) {
    const source = fs.readFileSync(path.resolve(__dirname, `../../assets/scripts/app/${name}.ts`), 'utf8');
    const result = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } });
    const module = { exports: {} }; new Function('module', 'exports', result.outputText)(module, module.exports);
    return module.exports;
}
const { EntityMotion } = load('EntityMotion');
const { EntityAnimation, movementDisplayDurationMs } = load('EntityAnimation');
const manifest = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../assets/resources/development/manifest.json'), 'utf8'));
const profiles = [[2, manifest.profiles.pet_skeleton], [3, manifest.profiles.monster_scarecrow]];
const observation = { direction: 2, action: 0, attackSequence: 0, dead: false, injured: false };
const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 1e-8, `${actual} != ${expected}`);
function target(kind, actions, cadence, x, y = 0, extra = {}) {
    return { x, y, direction: 2, durationMs: movementDisplayDurationMs(kind, actions.walking, cadence), ...extra };
}

test('实际怪物和宝宝六帧100ms独立于1000/2000ms发步节奏，显示时长来自各自资源', () => {
    for (const [kind, profile] of profiles) {
        const walking = profile.layers[0].actions.walking;
        assert.deepEqual([walking.count, walking.duration_ms], [6, 100]);
        for (const cadence of [1000, 2000]) assert.equal(movementDisplayDurationMs(kind, walking, cadence), 600);
        // 资源时长改变时跟随元数据，不能只把当前600写死在渲染器。
        assert.equal(movementDisplayDurationMs(kind, { count: 8, duration_ms: 90 }, 2000), 720);
    }
    assert.equal(movementDisplayDurationMs(1, { count: 6, duration_ms: 100 }, 400), 400);
    assert.equal(movementDisplayDurationMs(4, undefined, 0), 0);
    assert.equal(movementDisplayDurationMs(3, undefined, 1000), 1000);
});

test('慢速权威发步时600ms完成六帧后站立，下一步从原落脚点开始且不重播', () => {
    for (const [kind, profile] of profiles) for (const cadence of [1000, 2000]) {
        const actions = profile.layers[0].actions;
        const motion = new EntityMotion(); const animation = new EntityAnimation(actions);
        motion.push(target(kind, actions, cadence, 0), 0); animation.observe(observation, 0);
        for (let step = 1; step <= 3; step++) {
            const start = 200 + (step - 1) * cadence;
            const first = motion.push(target(kind, actions, cadence, step), start);
            near(first.x, step - 1); assert.equal(first.moving, true);
            for (let elapsed = 0; elapsed < cadence; elapsed += 100) {
                const now = start + elapsed;
                // 真实全量快照在AI等下一步时仍继续到达，重复坐标不重启步帧。
                if (elapsed % 200 === 0) motion.push(target(kind, actions, cadence, step), now);
                animation.observe(observation, now);
                const frame = motion.sample(now + 1); const pose = animation.sample(now + 1, frame);
                if (elapsed < 600) {
                    near(frame.x, step - 1 + (elapsed + 1) / 600);
                    assert.equal(frame.moving, true);
                    assert.equal(pose.action, 'walking'); assert.equal(pose.frame, elapsed / 100);
                    assert.equal(frame.stepSequence, first.stepSequence);
                } else {
                    assert.equal(frame.x, step); assert.equal(frame.moving, false);
                    assert.equal(pose.action, 'standing');
                }
            }
        }
    }
});

test('提前收到下一格仍连续经过已知拐点，600ms显示窗口内有界追赶且不斜穿', () => {
    for (const [kind, profile] of profiles) {
        const actions = profile.layers[0].actions; const motion = new EntityMotion();
        motion.push(target(kind, actions, 2000, 0), 0);
        motion.push(target(kind, actions, 2000, 1), 200);
        const before = motion.sample(400);
        const added = motion.push(target(kind, actions, 2000, 1, 1, { direction: 4 }), 400);
        near(added.x, before.x); near(added.y, before.y); near(added.stepProgress, before.stepProgress);
        const corner = motion.sample(480); near(corner.x, 1); near(corner.y, 0);
        const after = motion.sample(520); near(after.x, 1); near(after.y, 1 / 3);
        const end = motion.sample(600); assert.deepEqual([end.x, end.y, end.moving], [1, 1, false]);
    }
});

test('慢发步显示修复保留丢路径、复活和后台空窗的显式恢复边界', () => {
    const [kind, profile] = profiles[0]; const actions = profile.layers[0].actions;
    for (const mode of ['missing_path', 'revive', 'background']) {
        const motion = new EntityMotion();
        motion.push(target(kind, actions, 2000, 0), 0);
        motion.push(target(kind, actions, 2000, 1), 200);
        near(motion.sample(400).x, 1 / 3);
        const now = mode === 'background' ? 2500 : 400;
        const finalX = mode === 'missing_path' ? 9 : 2;
        const restored = motion.push(target(kind, actions, 2000, finalX, 0, { snap: mode === 'revive' }), now);
        assert.deepEqual([restored.x, restored.moving, restored.stepProgress], [finalX, false, 0]);
        const next = motion.push(target(kind, actions, 2000, finalX + 1), now + 200);
        assert.equal(next.x, finalX); assert.equal(next.moving, true);
        near(motion.sample(now + 500).x, finalX + .5);
    }
});
