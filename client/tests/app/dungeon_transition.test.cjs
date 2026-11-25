const assert = require('node:assert/strict');
const test = require('node:test');
const { Long, harness, entry, ready } = require('./helpers/session_harness.cjs');

const challengeId = new Uint8Array(16).fill(17);
const enter = (scene, map, kind) => ({ ...entry(scene, map, kind), serverTransitionId: '11'.repeat(16) });
const grant = (from, to) => ({
    challengeId, sourceSceneEpoch: from.sceneEpoch, targetSceneEpoch: to.sceneEpoch,
    targetBaselineId: to.baselineId, targetMapId: to.roomId, targetSceneKind: to.sceneKind,
});

test('讨伐首帧与原地图回场分别需要同连接的精确授权', () => {
    for (const [map, kind] of [['1', 1], ['5', 2]]) {
        const h = harness(), socket = h.session.connection;
        const home = ready(h, entry('9007199254740993', map, kind));
        const battle = enter('9007199254740995', '85', 5);
        socket.deliver(20002, battle);
        assert.equal(h.session.entry, home);
        socket.deliver(20024, grant(home, battle), { binding: Long.UZERO });
        socket.deliver(20002, battle);
        assert.equal(h.session.entry, home);
        socket.deliver(20024, grant(home, battle));
        socket.deliver(20002, { ...battle, baselineId: battle.baselineId.add(1) });
        assert.equal(h.session.entry, home);
        socket.deliver(20002, battle);
        socket.deliver(6, battle);
        assert.equal(h.session.entry, battle);
        assert.equal(h.session.entryDungeonTransition, true);
        assert.equal(h.session.ready, true);
        const back = enter('9007199254740996', map, kind);
        socket.deliver(20024, grant(battle, { ...back, roomId: Long.fromInt(6, true) }));
        socket.deliver(20002, back);
        assert.equal(h.session.entry, battle);
        socket.deliver(20024, grant(battle, back));
        socket.deliver(20002, back);
        socket.deliver(6, back);
        assert.equal(h.session.entry, back);
        assert.equal(h.session.ready, true);
    }
});

test('新票须重新接收讨伐授权，迟到段号和终态不能倒拨本场进度', () => {
    const h = harness(), socket = h.session.connection;
    const home = ready(h, entry('11'));
    const battle = enter('13', '85', 5);
    socket.deliver(20024, grant(home, battle));
    socket.status('模拟掉线');
    socket.binding = Long.fromInt(2, true);
    ready(h, home);
    socket.deliver(20002, battle);
    assert.equal(h.session.entry, home);
    socket.deliver(20024, grant(home, battle));
    socket.deliver(20002, battle);
    socket.deliver(6, battle);
    const progress = (stage, state, sceneEpoch = battle.sceneEpoch) =>
        socket.deliver(20023, { challengeId, sceneEpoch, stage, stageCount: 4, state });
    progress(2, 2);
    progress(1, 2);
    progress(4, 4, home.sceneEpoch);
    assert.equal(h.session.dungeonTransition.objective, '讨伐 · 第 2/4 段');
    progress(4, 3);
    assert.equal(h.session.dungeonTransition.objective, '正在结算');
    progress(4, 4);
    progress(4, 6);
    assert.equal(h.session.dungeonTransition.objective, '已通关');
});
