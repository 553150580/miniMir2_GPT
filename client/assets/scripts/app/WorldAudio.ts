import { AudioClip, AudioSource, input, Input, isValid, Node } from 'cc';
import type { WorldAssets } from './WorldAssets';

interface Voice { source: AudioSource; token: number; valid?: () => boolean; until: number; cue?: string; identity?: string; elapsedMs?: number; playCalls: number; playCallAt?: number; }
// AudioClip由WorldAssets入场预载并持有；声道只挂保存容器，不创建固定节点或不可撤销的one-shot播放器。
export class WorldAudio {
    private disposed = false;
    private readonly voices: Voice[];
    private unlocked = false;
    private gestureAt: number | null = null;
    private gestureSource: 'saved_ui_capture' | 'global_input' | null = null;
    private inputRootRetired = false;
    get audit() { return Object.freeze({ loaded: Object.freeze(Array.from(this.assets.audioClips.keys())), gestureUnlocked: this.unlocked, gestureAt: this.gestureAt, gestureSource: this.gestureSource,
        voices: Object.freeze(this.voices.map(voice => Object.freeze({ cue: voice.cue ?? '', identity: voice.identity ?? '', elapsedMs: voice.elapsedMs ?? 0,
            playCalls: voice.playCalls, playCallAt: voice.playCallAt ?? null, playing: isValid(voice.source) && voice.source.playing, clipBound: isValid(voice.source) && !!voice.source.clip }))) }); }
    constructor(private readonly assets: WorldAssets, parent: Node, private readonly inputRoot: Node, private readonly now: () => number, private readonly current: () => boolean) {
        this.voices = [0, 1].map(() => { const source = parent.addComponent(AudioSource); source.playOnAwake = false; source.loop = false;
            return { source, token: 0, until: 0, playCalls: 0 }; });
        const clip = assets.audioClips.values().next().value;
        if (clip) this.voices[0].source.clip = clip;
        // Creator的UI指针分发会先吞掉已认领触摸；保存Safe的捕获阶段先于Button停止冒泡。
        inputRoot.on(Node.EventType.TOUCH_END, this.captureGesture, this, true);
        // 引擎先发此事件，再清事件表、销毁子树，最后才调用外层AppRoot.onDestroy。
        inputRoot.on(Node.EventType.NODE_DESTROYED, this.retireInputRoot, this);
        input.on(Input.EventType.TOUCH_END, this.globalGesture, this);
    }
    private retireInputRoot(): void { this.inputRootRetired = true; this.dispose(); }
    // 引擎公开AudioSource在真实触屏回调中解锁；没有改浏览器自动播放策略，也不在拒绝施法时有声播放。
    private captureGesture(): void { this.unlock('saved_ui_capture'); }
    private globalGesture(): void { this.unlock('global_input'); }
    private unlock(source: 'saved_ui_capture' | 'global_input'): void {
        if (this.disposed || !this.current() || this.unlocked) return;
        const voice = this.voices[0]; if (!isValid(voice.source, true) || !voice.source.clip) return;
        voice.source.volume = 0; voice.source.play(); voice.source.stop(); this.unlocked = true; this.gestureAt = this.now(); this.gestureSource = source;
    }
    play(channel: 0 | 1, key: string, valid: () => boolean, until: number, elapsedMs = 0, identity = ''): void {
        if (this.disposed || !this.unlocked || !valid() || this.now() >= until) return;
        const voice = this.voices[channel], token = ++voice.token;
        this.stop(voice); voice.valid = valid; voice.until = until; const requested = this.now();
        // 仍经过一次异步交接：迟到回调必须核原声道/原Life/原世界，不能清理或播放替换后的声道。
        Promise.resolve(this.assets.audioClips.get(key)).then((clip: AudioClip | undefined) => {
            if (this.disposed || voice.token !== token || !valid() || this.now() >= until || !clip || !isValid(voice.source, true)) return;
            const elapsed = elapsedMs + this.now() - requested;
            if (elapsed >= clip.getDuration() * 1000) return;
            voice.source.clip = clip; voice.source.volume = 1; voice.source.currentTime = Math.max(0, elapsed / 1000); voice.source.play();
            // 动画窗口只约束开始资格；原WAV独立播放到完整剩余时长，不能被600/1000ms图像时长截短。
            voice.until = this.now() + clip.getDuration() * 1000 - elapsed;
            voice.cue = key; voice.identity = identity; voice.elapsedMs = elapsed; voice.playCallAt = this.now(); voice.playCalls++;
        });
    }
    update(): void {
        for (const voice of this.voices) if (voice.valid && (!voice.valid() || this.now() >= voice.until)) { ++voice.token; this.stop(voice); }
    }
    private stop(voice: Voice): void {
        voice.valid = undefined;
        // 排队销毁但尚存活的组件仍要先解绑clip；子树已实际销毁时AudioSource.onDestroy已完成此事。
        if (isValid(voice.source)) { voice.source.stop(); voice.source.clip = null; }
    }
    cancelAll(): void { for (const voice of this.voices) { ++voice.token; this.stop(voice); } }
    dispose(): void {
        if (this.disposed) return; this.disposed = true;
        input.off(Input.EventType.TOUCH_END, this.globalGesture, this);
        // strict不检查Destroying；公开销毁信号单独标记，事件表由NodeEventProcessor.destroy自行清空。
        if (!this.inputRootRetired && isValid(this.inputRoot, true)) {
            this.inputRoot.off(Node.EventType.TOUCH_END, this.captureGesture, this, true);
            this.inputRoot.off(Node.EventType.NODE_DESTROYED, this.retireInputRoot, this);
        }
        for (const voice of this.voices) { ++voice.token; this.stop(voice); if (isValid(voice.source, true)) voice.source.destroy(); }
    }
}
