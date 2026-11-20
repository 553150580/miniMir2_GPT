const assert = require('node:assert/strict');
const test = require('node:test');
const { load } = require('./helpers/session_harness.cjs');

function fixture() {
    const events = () => {
        const listeners = new Map();
        return {
            on(event, handler, context) { listeners.set(event, { handler, context }); },
            off(event, handler) { if (listeners.get(event)?.handler === handler) listeners.delete(event); },
            emit(event, ...args) { const listener = listeners.get(event); listener?.handler.apply(listener.context, args); },
        };
    };
    const started = [], callbacks = new Map(), traces = [], game = { totalTime: 0, ...events() };
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
    const cc = { Node, AudioSource, AudioClip: class {}, director, game, input, isValid: node => !node.destroyed,
        Game: { EVENT_HIDE: 'hide', EVENT_SHOW: 'show' }, Input: { EventType: { TOUCH_END: 'touch' } },
        resources: { load(key, type, done) { callbacks.set(key, done); } } };
    const audio = load('client/assets/scripts/app/ClientAudio.ts', id => id === 'cc' ? cc : { traceInput: trace => traces.push(trace) });
    const ready = name => {
        const clip = { name, refs: 0, addRef() { this.refs++; return this; }, decRef() { this.refs--; } };
        callbacks.get('audio/' + name)(null, clip);
        return clip;
    };
    return { ...audio, ready, Node, scene, game, started, traces };
}

test('音乐由首次触摸启动，选角继承大厅音乐，战斗切换且后台静音', () => {
    const f = fixture(), login = new f.Node(), roles = new f.Node(), field = new f.Node();
    const unbind = f.bindClientAudio(login, 'Login');
    const lobby = f.ready('dusk_gate');
    f.ready('tap');
    f.playClientCue('tap');
    assert.deepEqual(f.started, []);
    login.emit('touch');
    f.playClientCue('tap');
    f.game.totalTime += 30;
    f.playClientCue('tap');
    f.bindClientAudio(roles, 'RoleSelect');
    unbind();
    assert.deepEqual(f.started, ['dusk_gate', 'tap']);
    f.bindClientAudio(field, 'Game');
    const battle = f.ready('ember_path');
    const sound = f.scene.nodes[0];
    assert.equal(f.scene.nodes.length, 1);
    assert.equal(sound.channels[0].loop, true);
    assert.deepEqual(f.started, ['dusk_gate', 'tap', 'ember_path']);
    f.game.emit('hide');
    f.playClientCue('tap');
    assert.equal(sound.channels[0].playing, false);
    f.game.emit('show');
    assert.equal(f.started.at(-1), 'ember_path');
    sound.emit('destroy');
    assert.equal(lobby.refs, 0);
    assert.equal(battle.refs, 0);
    assert.deepEqual(f.traces.slice(0, 2).map(({ clip, loop }) => ({ clip, loop })), [
        { clip: 'dusk_gate', loop: true }, { clip: 'tap', loop: false },
    ]);
});

test('旧音乐的加载回调晚到时只播放当前场景音乐', () => {
    const f = fixture(), login = new f.Node(), field = new f.Node();
    f.bindClientAudio(login, 'Login');
    login.emit('touch');
    f.bindClientAudio(field, 'Game');
    f.ready('dusk_gate');
    assert.deepEqual(f.started, []);
    f.ready('ember_path');
    assert.deepEqual(f.started, ['ember_path']);
});
