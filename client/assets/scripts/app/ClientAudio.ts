import { AudioClip, AudioSource, director, Game, game, Input, input, isValid, Node, resources } from 'cc';
import { traceInput } from './InputTrace';

const music = { dusk_gate: 0.53, ember_path: 0.45 } as const;
const cues = { tap: 0.46, swing: 0.72, spell: 0.67, impact: 0.65, mend: 0.63 } as const;
type Track = keyof typeof music;
type Cue = keyof typeof cues;
type Clip = Track | Cue;

// 常驻节点只承载声道，场景布局和地图资源仍由各自的组件管理。
class ClientAudio {
    private readonly background: AudioSource;
    private readonly effects = new Map<Cue, AudioSource>();
    private readonly clips = new Map<Clip, AudioClip>();
    private readonly loading = new Set<Clip>();
    private readonly playedAt = new Map<Cue, number>();
    private track: Track | null = null;
    private musicRequested = false;
    private unlocked = false;
    private hidden = false;

    constructor(private readonly node: Node) {
        this.background = this.channel(true);
        node.on(AudioSource.EventType.STARTED, this.traceStarted, this);
        node.on(Node.EventType.NODE_DESTROYED, this.dispose, this);
        input.on(Input.EventType.TOUCH_END, this.unlock, this);
        game.on(Game.EVENT_HIDE, this.hide, this);
        game.on(Game.EVENT_SHOW, this.show, this);
    }

    bind(root: Node, scene: string): () => void {
        const selected = scene === 'Boot' ? null : scene === 'Game' ? 'ember_path' : 'dusk_gate';
        if (selected !== this.track) {
            this.track = selected;
            this.musicRequested = false;
            if (this.background.clip) this.background.stop();
            this.background.clip = null;
            this.stopEffects();
        }
        this.load(selected ?? 'dusk_gate');
        for (const cue of Object.keys(cues) as Cue[]) this.load(cue);
        this.syncMusic();
        // 保存场景的根节点在捕获阶段先于按钮收到真实触摸。
        root.on(Node.EventType.TOUCH_END, this.unlock, this, true);
        return () => {
            if (isValid(root, true)) root.off(Node.EventType.TOUCH_END, this.unlock, this, true);
        };
    }

    play(cue: Cue): void {
        const clip = this.clips.get(cue);
        if (!this.unlocked || this.hidden || !clip) return;
        const now = game.totalTime;
        if (now - (this.playedAt.get(cue) ?? -Infinity) < 85) return;
        this.playedAt.set(cue, now);
        let source = this.effects.get(cue);
        if (!source) {
            source = this.channel(false);
            source.clip = clip;
            source.volume = cues[cue];
            this.effects.set(cue, source);
        } else if (source.playing) {
            source.stop();
        }
        source.currentTime = 0;
        source.play();
    }

    private channel(loop: boolean): AudioSource {
        const source = this.node.addComponent(AudioSource);
        source.loop = loop;
        source.playOnAwake = false;
        return source;
    }

    private load(key: Clip): void {
        if (this.clips.has(key) || this.loading.has(key)) return;
        this.loading.add(key);
        resources.load(`audio/${key}`, AudioClip, (error, clip) => {
            this.loading.delete(key);
            if (error || !clip || !isValid(this.node, true)) return;
            clip.addRef();
            this.clips.set(key, clip);
            this.syncMusic();
        });
    }

    private syncMusic(): void {
        if (!this.track || !this.unlocked || this.hidden || this.musicRequested) return;
        const clip = this.clips.get(this.track);
        if (!clip) return;
        if (this.background.clip !== clip) this.background.clip = clip;
        this.background.volume = music[this.track];
        this.musicRequested = true;
        this.background.play();
    }

    private unlock(): void {
        if (this.unlocked) return;
        this.unlocked = true;
        this.syncMusic();
    }

    private hide(): void {
        this.hidden = true;
        if (this.musicRequested) this.background.pause();
        this.musicRequested = false;
        this.stopEffects();
    }

    private show(): void {
        this.hidden = false;
        this.syncMusic();
    }

    private stopEffects(): void {
        for (const source of this.effects.values()) source.stop();
        this.playedAt.clear();
    }

    private traceStarted(source: AudioSource): void {
        if (source.clip) traceInput({ event: 'client_audio', clip: source.clip.name, loop: source.loop });
    }

    private dispose(): void {
        input.off(Input.EventType.TOUCH_END, this.unlock, this);
        game.off(Game.EVENT_HIDE, this.hide, this);
        game.off(Game.EVENT_SHOW, this.show, this);
        for (const source of [this.background, ...Array.from(this.effects.values())]) {
            source.stop();
            source.clip = null;
        }
        for (const clip of this.clips.values()) clip.decRef();
        this.clips.clear();
        if (runtime === this) runtime = undefined;
    }
}

let runtime: ClientAudio | undefined;

export function bindClientAudio(root: Node, sceneKind: string): (() => void) | undefined {
    if (!runtime) {
        const scene = director?.getScene?.();
        if (!scene) return;
        const node = new Node('ClientAudio');
        scene.addChild(node);
        director.addPersistRootNode(node);
        runtime = new ClientAudio(node);
    }
    return runtime.bind(root, sceneKind);
}

export function playClientCue(cue: Cue): void {
    runtime?.play(cue);
}
