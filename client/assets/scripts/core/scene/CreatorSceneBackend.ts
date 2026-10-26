export type SceneProgressCallback = (completed: number, total: number, item: unknown) => void;
export type SceneCompleteCallback = (error: Error | null) => void;

export interface SceneBackend {
    preload(name: string, progress: SceneProgressCallback, complete: SceneCompleteCallback): void;
    load(name: string, complete: SceneCompleteCallback): boolean;
}

// 对齐 Creator 3.8.8 director.ts 的三参预加载与返回 boolean 的加载签名。
// Bootstrap 后续注入从 cc 导入的真实 director；本层不创建节点或声明伪 cc 模块。
export interface CreatorDirector {
    preloadScene(name: string, progress: SceneProgressCallback, complete: SceneCompleteCallback): void;
    loadScene(name: string, complete?: SceneCompleteCallback, unloaded?: () => void): boolean;
}

export class CreatorSceneBackend implements SceneBackend {
    constructor(private readonly director: CreatorDirector) {}

    preload(name: string, progress: SceneProgressCallback, complete: SceneCompleteCallback): void {
        this.director.preloadScene(name, progress, complete);
    }

    load(name: string, complete: SceneCompleteCallback): boolean {
        return this.director.loadScene(name, complete);
    }
}
