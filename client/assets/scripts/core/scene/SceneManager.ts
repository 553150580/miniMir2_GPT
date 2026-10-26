import { SceneLoader, SceneOperationError, ScenePreloadTask, SceneProgress } from './SceneLoader';

export interface SceneState {
    readonly phase: 'idle' | 'preloading' | 'loading' | 'disposed';
    readonly currentScene: string | null;
    readonly targetScene: string | null;
    readonly progress: SceneProgress | null;
    readonly lastError: Error | null;
    readonly disposed: boolean;
}

interface Transition {
    readonly token: number;
    readonly target: string;
    readonly promise: Promise<string>;
    readonly resolve: (scene: string) => void;
    readonly reject: (error: Error) => void;
    phase: 'preloading' | 'loading';
    preload?: ScenePreloadTask;
}

// 只管理引擎场景切换；不管理弹窗、导航目标或共享资源的生命周期。
export class SceneManager {
    private readonly scenes = new Set<string>();
    private readonly listeners = new Set<(state: SceneState) => void>();
    private active?: Transition;
    private token = 0;
    private current: string | null;
    private progress: SceneProgress | null = null;
    private lastError: Error | null = null;
    private disposed = false;
    private disposal?: Promise<void>;

    constructor(private readonly loader: SceneLoader, sceneNames: readonly string[], initialScene: string | null = null) {
        sceneNames.forEach(name => this.registerScene(name));
        if (initialScene !== null && !this.scenes.has(initialScene)) throw this.unknown(initialScene);
        this.current = initialScene;
    }

    registerScene(name: string): void {
        if (this.disposed) throw new SceneOperationError('DISPOSED', '场景管理器已释放');
        if (!name || name.trim() !== name) throw this.unknown(name);
        this.scenes.add(name);
    }

    get state(): SceneState {
        return {
            phase: this.active?.phase ?? (this.disposed ? 'disposed' : 'idle'),
            currentScene: this.current,
            targetScene: this.active?.target ?? null,
            progress: this.progress ? { ...this.progress } : null,
            lastError: this.lastError,
            disposed: this.disposed,
        };
    }

    subscribe(listener: (state: SceneState) => void): () => void {
        if (this.disposed) throw new SceneOperationError('DISPOSED', '场景管理器已释放');
        this.listeners.add(listener);
        return () => { this.listeners.delete(listener); };
    }

    // 不使用 async 包装，确保同目标调用返回完全相同的 Promise。
    switchTo(target: string): Promise<string> {
        if (this.disposed) return Promise.reject(new SceneOperationError('DISPOSED', '场景管理器已释放'));
        if (!this.scenes.has(target)) return Promise.reject(this.unknown(target));
        if (this.active) return this.active.target === target ? this.active.promise
            : Promise.reject(new SceneOperationError('BUSY', `正在切换场景：${this.active.target}`));
        if (this.current === target) return Promise.resolve(target);
        let resolve!: Transition['resolve'];
        let reject!: Transition['reject'];
        const promise = new Promise<string>((ok, fail) => { resolve = ok; reject = fail; });
        const task: Transition = { token: ++this.token, target, promise, resolve, reject, phase: 'preloading' };
        this.active = task;
        this.progress = null;
        this.lastError = null;
        this.publish();
        if (!this.isActive(task)) return promise;
        task.preload = this.loader.preload(target, progress => {
            if (!this.isActive(task) || task.phase !== 'preloading') return;
            this.progress = progress;
            this.publish();
        });
        // 同步进度回调也可能触发取消；返回任务后补充释放其观察者。
        if (!this.isActive(task)) task.preload.cancel();
        task.preload.promise.then(() => {
            if (!this.isActive(task)) return;
            task.phase = 'loading';
            this.progress = null;
            const loading = this.loader.load(target);
            this.publish();
            loading.then(() => this.finish(task), error => this.finish(task, error));
        }, error => this.finish(task, error));
        return promise;
    }

    cancelPreload(): boolean {
        const task = this.active;
        if (!task || task.phase !== 'preloading') return false;
        this.active = undefined;
        ++this.token;
        this.progress = null;
        task.preload?.cancel();
        task.reject(new SceneOperationError('CANCELLED', `已取消场景预加载后续切换：${task.target}`));
        this.publish();
        return true;
    }

    // 加载已开始时只清本服务监听，等待真实结果；不销毁任何 UI 或全局资源。
    dispose(): Promise<void> {
        if (this.disposal) return this.disposal;
        this.disposed = true;
        this.listeners.clear();
        this.cancelPreload();
        this.disposal = this.active ? this.active.promise.then(() => {}, () => {}) : Promise.resolve();
        return this.disposal;
    }

    private finish(task: Transition, error?: Error): void {
        if (!this.isActive(task)) return;
        this.active = undefined;
        this.progress = null;
        this.lastError = error ?? null;
        if (error) {
            if (error instanceof SceneOperationError && error.code === 'LOAD_INTERRUPTED') this.current = null;
            task.reject(error);
        }
        else {
            this.current = task.target;
            task.resolve(task.target);
        }
        this.publish();
    }

    private isActive(task: Transition): boolean {
        return this.active === task && task.token === this.token;
    }

    private publish(): void {
        for (const listener of Array.from(this.listeners)) {
            if (this.disposed) break;
            if (!this.listeners.has(listener)) continue;
            try { listener(this.state); } catch { /* 隔离界面观察者 */ }
        }
    }

    private unknown(name: string): SceneOperationError {
        return new SceneOperationError('UNKNOWN_SCENE', `场景未登记或名称无效：${name}`);
    }
}
