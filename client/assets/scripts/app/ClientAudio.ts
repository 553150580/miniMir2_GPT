import { AudioClip, AudioSource, director, Game, game, Input, input, isValid, Node, resources } from 'cc';
import { traceInput } from './InputTrace';
import { originalActorAudioClips } from './OriginalActorAudio';
import { originalSkillAudioClips } from './OriginalSkillAudio';

export const originalSceneMusic = { Login: 'Log-in-long2', RoleSelect: 'main_theme', Game: 'Field2' } as const;
export const originalButtonSound = '103';
const effectClips = new Set([originalButtonSound, ...originalActorAudioClips, ...originalSkillAudioClips]);
const effectChannels = 8;

interface EffectChannel { source: AudioSource; busyUntil: number; }

// 常驻节点只承载声道，场景布局和地图资源仍由各自的组件管理。
class ClientAudio {
    private readonly background: AudioSource;
    private readonly effects: EffectChannel[] = [];
    private readonly clips = new Map<string, AudioClip>();
    private readonly loading = new Set<string>();
    private readonly playedAt = new Map<string, number>();
    private track: string | null = null;
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
        const selected = scene in originalSceneMusic ? originalSceneMusic[scene as keyof typeof originalSceneMusic] : null;
        if (selected !== this.track) {
            this.track = selected;
            this.musicRequested = false;
            if (this.background.clip) this.background.stop();
            this.background.clip = null;
            this.stopEffects();
        }
        this.load(selected ?? originalSceneMusic.Login);
        this.load(originalButtonSound);
        if (scene === 'Game') for (const cue of effectClips) this.load(cue);
        this.syncMusic();
        // 保存场景的根节点在捕获阶段先于按钮收到真实触摸。
        root.on(Node.EventType.TOUCH_END, this.unlock, this, true);
        return () => {
            if (isValid(root, true)) root.off(Node.EventType.TOUCH_END, this.unlock, this, true);
        };
    }

    play(cue: string | undefined, gain: number): void {
        if (!cue || !effectClips.has(cue) || !this.unlocked || this.hidden || gain <= 0) return;
        const clip = this.clips.get(cue);
        if (!clip) return;
        const now = game.totalTime;
        if (now - (this.playedAt.get(cue) ?? -Infinity) < 85) return;
        let channel = this.effects.find(voice => voice.busyUntil <= now);
        if (!channel && this.effects.length < effectChannels) {
            channel = { source: this.channel(false), busyUntil: 0 };
            this.effects.push(channel);
        }
        if (!channel) return;
        this.playedAt.set(cue, now);
        channel.busyUntil = now + clip.getDuration() * 1000;
        const source = channel.source;
        if (source.playing) source.stop();
        source.clip = clip;
        source.volume = Math.min(1, gain) * 0.66;
        source.currentTime = 0;
        source.play();
    }

    private channel(loop: boolean): AudioSource {
        const source = this.node.addComponent(AudioSource);
        source.loop = loop;
        source.playOnAwake = false;
        return source;
    }

    private load(key: string): void {
        if (this.clips.has(key) || this.loading.has(key)) return;
        const path = `audio/${key}`;
        // 声音原件按本地来源合同导入。没有安装原件的开发副本可静音运行。
        if (!resources.getInfoWithPath(path, AudioClip)) return;
        this.loading.add(key);
        resources.load(path, AudioClip, (error, clip) => {
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
        this.background.volume = 0.38;
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
        for (const { source } of this.effects) { source.stop(); source.clip = null; }
        for (const voice of this.effects) voice.busyUntil = 0;
        this.playedAt.clear();
    }

    private traceStarted(source: AudioSource): void {
        if (source.clip) traceInput({ event: 'client_audio', clip: source.clip.name, loop: source.loop });
    }

    private dispose(): void {
        input.off(Input.EventType.TOUCH_END, this.unlock, this);
        game.off(Game.EVENT_HIDE, this.hide, this);
        game.off(Game.EVENT_SHOW, this.show, this);
        this.background.stop();
        this.background.clip = null;
        this.stopEffects();
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

export function playClientCue(cue: string | undefined, gain = 1): void {
    runtime?.play(cue, gain);
}
