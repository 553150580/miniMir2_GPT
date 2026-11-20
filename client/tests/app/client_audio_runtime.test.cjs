const assert = require('node:assert/strict');
const test = require('node:test');
const { load } = require('./helpers/session_harness.cjs');
const { makeWorld, state, Long, clientAudio } = require('../support/world_runtime.cjs');

const actorSounds = load('client/assets/scripts/app/OriginalActorAudio.ts', () => ({}));
const skillSounds = load('client/assets/scripts/app/OriginalSkillAudio.ts', () => ({}));

function fixture(missing = []) {
    const events = () => {
        const listeners = new Map();
        return {
            on(event, handler, context) { listeners.set(event, { handler, context }); },
            off(event, handler) { if (listeners.get(event)?.handler === handler) listeners.delete(event); },
            emit(event, ...args) { const listener = listeners.get(event); listener?.handler.apply(listener.context, args); },
        };
    };
    const started = [], callbacks = new Map(), traces = [], requested = new Set(), game = { totalTime: 0, ...events() };
    const scene = { nodes: [], addChild(node) { this.nodes.push(node); } };
    const director = { getScene: () => scene, addPersistRootNode(node) { node.persistent = true; } };
    const input = events();
    class AudioSource {
        static EventType = { STARTED: 'started' };
        play() { this.playing = true; started.push(this.clip.name); this.node.emit('started', this); }
        pause() { this.playing = false; }
        stop() { this.pause(); this.currentTime = 0; }
    }
    class Node {
        static EventType = { TOUCH_END: 'touch', NODE_DESTROYED: 'destroy' };
        constructor() { Object.assign(this, events()); this.channels = []; }
        addComponent(Type) { const source = new Type(); source.node = this; this.channels.push(source); return source; }
    }
    class AudioClip {}
    const available = new Set(['103', 'Log-in-long2', 'main_theme', 'Field2', ...actorSounds.originalActorAudioClips, ...skillSounds.originalSkillAudioClips]);
    for (const name of missing) available.delete(name);
    const cc = { Node, AudioSource, AudioClip, director, game, input, isValid: node => !node.destroyed,
        Game: { EVENT_HIDE: 'hide', EVENT_SHOW: 'show' }, Input: { EventType: { TOUCH_END: 'touch' } },
        resources: {
            getInfoWithPath(key, type) {
                assert.equal(type, AudioClip);
                return key.startsWith('audio/') && available.has(key.slice(6)) ? { path: key } : null;
            },
            load(key, type, done) {
                assert.ok(this.getInfoWithPath(key, type), `不得加载未导入的原资源 ${key}`);
                assert.ok(!callbacks.has(key), `同一资源只能有一次在途加载 ${key}`);
                requested.add(key.slice(6)); callbacks.set(key, done);
            },
        } };
    const audio = load('client/assets/scripts/app/ClientAudio.ts', id => ({
        cc, './InputTrace': { traceInput: trace => traces.push(trace) },
        './OriginalActorAudio': actorSounds, './OriginalSkillAudio': skillSounds,
    })[id]);
    const ready = (name, duration = 0.3) => {
        const key = 'audio/' + name, done = callbacks.get(key);
        assert.ok(done, `未请求原资源 ${name}`);
        const clip = { name, refs: 0, addRef() { this.refs++; return this; }, decRef() { this.refs--; }, getDuration() { return duration; } };
        callbacks.delete(key); done(null, clip);
        return clip;
    };
    return { ...audio, ready, Node, scene, game, started, traces, requested };
}

test('首次触摸启动原音乐，登录、选角与战斗独立切换，后台停止音效', () => {
    const f = fixture(), login = new f.Node(), roles = new f.Node(), field = new f.Node();
    const unbind = f.bindClientAudio(login, 'Login');
    assert.deepEqual(Array.from(f.requested), ['Log-in-long2', '103']);
    const lobby = f.ready('Log-in-long2', 20);
    f.ready('103');
    f.playClientCue('103');
    assert.deepEqual(f.started, []);
    login.emit('touch');
    f.playClientCue('103');
    f.game.totalTime += 30;
    f.playClientCue('103');
    f.bindClientAudio(roles, 'RoleSelect');
    unbind();
    assert.deepEqual(f.started, ['Log-in-long2', '103']);
    const menu = f.ready('main_theme', 30);
    assert.deepEqual(f.started, ['Log-in-long2', '103', 'main_theme']);
    f.bindClientAudio(field, 'Game');
    const battle = f.ready('Field2', 60);
    assert.ok(f.requested.has('470-2'));
    assert.ok(f.requested.has('M1-2'));
    const sound = f.scene.nodes[0];
    assert.equal(f.scene.nodes.length, 1);
    assert.equal(sound.channels[0].loop, true);
    assert.deepEqual(f.started, ['Log-in-long2', '103', 'main_theme', 'Field2']);
    f.game.emit('hide');
    f.playClientCue('103');
    assert.equal(sound.channels[0].playing, false);
    f.game.emit('show');
    assert.equal(f.started.at(-1), 'Field2');
    sound.emit('destroy');
    assert.equal(lobby.refs, 0);
    assert.equal(menu.refs, 0);
    assert.equal(battle.refs, 0);
    assert.deepEqual(f.traces.slice(0, 2).map(({ clip, loop }) => ({ clip, loop })), [
        { clip: 'Log-in-long2', loop: true }, { clip: '103', loop: false },
    ]);
});

test('旧音乐的加载回调晚到时只播放当前场景音乐', () => {
    const f = fixture(), login = new f.Node(), field = new f.Node();
    f.bindClientAudio(login, 'Login');
    login.emit('touch');
    f.bindClientAudio(new f.Node(), 'RoleSelect');
    f.bindClientAudio(field, 'Game');
    f.ready('Log-in-long2');
    f.ready('main_theme');
    assert.deepEqual(f.started, []);
    f.ready('Field2');
    assert.deepEqual(f.started, ['Field2']);
    f.scene.nodes[0].emit('destroy');
});

test('未导入的音乐或技能原件和没有源映射的事件保持静音', () => {
    const f = fixture(['Field2', 'M1-1', '470-4']), field = new f.Node();
    f.bindClientAudio(field, 'Game');
    field.emit('touch');
    for (const name of ['Field2', 'M1-1', '470-4', 'M11-1', 'tap']) assert.equal(f.requested.has(name), false);
    f.playClientCue(skillSounds.originalSkillAudio(1).cast);
    f.playClientCue(actorSounds.originalActorSound({ entityKind: 3, definitionId: 1 }, 'struck'));
    f.playClientCue(skillSounds.originalSkillAudio(27)?.cast);
    f.playClientCue('tap');
    assert.deepEqual(f.started, []);

    f.ready('M11-2');
    const thunder = skillSounds.originalSkillAudio(11);
    assert.equal(thunder.cast, undefined);
    f.playClientCue(thunder.impact);
    assert.deepEqual(f.started, ['M11-2']);
    f.scene.nodes[0].emit('destroy');
});

test('真实世界按当前附着播放挥砍、着体、稻草人与火球原声，旧相位与重复事件保持静音', () => {
    const f = fixture(), field = new f.Node();
    f.bindClientAudio(field, 'Game');
    for (const clip of ['51', '70', 'M25-1', '470-2', '470-4', 'M1-1', 'M1-2', 'M1-3']) f.ready(clip);
    field.emit('touch');

    const previousCue = clientAudio.playClientCue;
    clientAudio.playClientCue = f.playClientCue;
    const world = makeWorld(1), view = world.view, scope = { connection: 1, binding: '1', role: 'owner', scene: '1', baseline: '1', map: '1', travelId: '' };
    const own = state(), monster = state(3, 3, 10, 8);
    const origin = own.position, targetGrid = monster.position;
    const attack = { sceneEpoch: Long.UONE, eventId: Long.UONE, skillId: 25, casterLife: Long.UZERO, casterHandle: 1,
        targetHandle: 3, origin, targetGrid, direction: 2, attackSequence: 1, inputSequence: 1, delayMs: 0 };
    const impact = { sceneEpoch: Long.UONE, eventId: Long.UONE, skillId: 25, casterHandle: 1, targetHandle: 3,
        targetLife: Long.UZERO, targetGrid, amount: Long.ONE, outcome: 1, healing: false, castId: Long.UZERO, attackSequence: 1 };
    const cast = { sceneEpoch: Long.UONE, castId: Long.UONE, skillId: 1, casterHandle: 1, casterLife: Long.UZERO,
        targetHandle: 3, targetLife: Long.UZERO, origin, targetGrid, direction: 2, delayMs: 800, skillLevel: 0 };
    const update = now => { f.game.totalTime = now; world.update(now); };
    let current = true;
    try {
        view.bindPresentation(scope, () => current);
        world.apply([own, monster]);
        assert.equal(view.observeEquipment(scope, 1, { equipped: [{ slot: 0, item: { itemId: 218 } }] }), true);
        view.acceptAttack(attack);
        view.acceptAttack(attack);
        view.acceptImpact(impact);
        assert.deepEqual(f.started, ['51', 'M25-1', '70']);

        update(100);
        world.apply([own, { ...monster, hp: Long.fromInt(99), attackSequence: 1 }]);
        assert.deepEqual(f.started.slice(-2), ['470-2', '470-4']);
        view.acceptCast({ ...cast, casterLife: Long.UONE });
        assert.ok(!f.started.includes('M1-1'));
        view.acceptCast(cast);
        assert.equal(f.started.at(-1), 'M1-1');
        const flightDelay = Math.min(cast.delayMs, view.gameplay.skills.find(skill => skill.id === 1).impact_base_ms);
        const flight = 100 + flightDelay;
        update(flight - 1);
        assert.equal(f.started.at(-1), 'M1-1');
        update(flight);
        assert.equal(f.started.at(-1), 'M1-2');
        update(900);
        const spellHit = { ...impact, eventId: Long.fromInt(2, true), castId: cast.castId, skillId: 1, attackSequence: 0 };
        view.acceptImpact(spellHit);
        view.acceptImpact({ ...spellHit, eventId: Long.fromInt(3, true) });
        assert.equal(f.started.filter(name => name === 'M1-3').length, 1);

        view.acceptCast({ ...cast, castId: Long.fromInt(2, true) });
        assert.equal(f.started.filter(name => name === 'M1-1').length, 2);
        current = false;
        const before = f.started.slice();
        update(900 + flightDelay);
        world.apply([own, { ...monster, hp: Long.ZERO, attackSequence: 2 }]);
        view.acceptAttack({ ...attack, eventId: Long.fromInt(2, true) });
        view.acceptImpact({ ...spellHit, eventId: Long.fromInt(4, true) });
        assert.deepEqual(f.started, before);
    } finally {
        view.dispose();
        clientAudio.playClientCue = previousCue;
        f.scene.nodes[0].emit('destroy');
    }
});
