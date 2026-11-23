const assert = require('node:assert/strict');
const test = require('node:test');
const { Long, harness, entry, ready } = require('./helpers/session_harness.cjs');

const occurrenceId = '0123456789abcdef0123456789abcdef';
const homeEpoch = '9007199254740993';
const bossEpoch = '9007199254740994';
const returnEpoch = '9007199254740995';

test('个人和世界首领均需要匹配的入场状态和终态才能切换地图', () => {
    for (const [messageId, key, kind, map] of [[20014, 'challengeId', 3, '80'], [20020, 'occurrenceId', 4, '81']]) {
        const game = harness(), socket = game.session.connection;
        const home = ready(game, entry(homeEpoch));
        const inside = { ...entry(bossEpoch, map, kind), serverTransitionId: occurrenceId };
        socket.deliver(20002, inside);
        assert.equal(game.session.entry, home);
        socket.deliver(messageId, { [key]: occurrenceId, sceneEpoch: inside.sceneEpoch, state: 2 }, { binding: Long.UZERO });
        socket.deliver(20002, inside);
        assert.equal(game.session.entry, home);
        socket.deliver(messageId, { [key]: occurrenceId, sceneEpoch: inside.sceneEpoch, state: 2 });
        socket.deliver(20002, inside);
        assert.equal(game.session.entry, inside);
        assert.equal(game.session.entryBossTransition, true);
        assert.equal(game.session.ready, false);
        socket.deliver(6, inside);
        assert.equal(game.session.ready, true);
        const restored = { ...entry(returnEpoch), serverTransitionId: occurrenceId };
        socket.deliver(20002, restored);
        assert.equal(game.session.entry, inside);
        socket.deliver(messageId, { [key]: occurrenceId, sceneEpoch: inside.sceneEpoch, state: 4 });
        socket.deliver(20002, { ...restored, roomId: Long.fromInt(20, true) });
        assert.equal(game.session.entry, inside);
        socket.deliver(20002, restored);
        assert.equal(game.session.entry, restored);
    }
});
