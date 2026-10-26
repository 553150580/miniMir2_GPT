import { SceneBackend } from './CreatorSceneBackend';

export type SceneErrorCode = 'PRELOAD_FAILED' | 'LOAD_FAILED' | 'LOAD_REJECTED' | 'LOAD_INTERRUPTED'
    | 'CANCELLED' | 'BUSY' | 'UNKNOWN_SCENE' | 'DISPOSED';

export class SceneOperationError extends Error {
    constructor(readonly code: SceneErrorCode, message: string, readonly detail?: unknown) {
        super(message);
        this.name = 'SceneOperationError';
    }
}

export interface SceneProgress {
    readonly completed: number;
    readonly total: number;
    // 总数未知或无效时没有百分比；非法计数按 0 展示，避免污染 UI 数值。
    readonly ratio: number | null;
    readonly item: unknown;
}

export interface ScenePreloadTask {
    readonly promise: Promise<void>;
    // 仅停止本任务回调和后续切换，不取消引擎底层资源请求。
    cancel(): boolean;
}

export class SceneLoader {
    constructor(private readonly backend: SceneBackend) {}

    preload(name: string, observer?: (progress: SceneProgress) => void): ScenePreloadTask {
        let settled = false;
        let rejectTask!: (error: Error) => void;
        const promise = new Promise<void>((resolve, reject) => {
            rejectTask = reject;
            const finish = (error: Error | null): void => {
                if (settled) return;
                settled = true;
                observer = undefined;
                if (error) reject(new SceneOperationError('PRELOAD_FAILED', `场景预加载失败：${name}`, error));
                else resolve();
            };
            try {
                this.backend.preload(name, (completed, total, item) => {
                    if (settled) return;
                    const ratio = Number.isFinite(total) && total > 0 && Number.isFinite(completed)
                        ? Math.min(1, Math.max(0, completed / total)) : null;
                    // 界面观察者异常不能中断引擎的资源加载回调。
                    try {
                        observer?.({
                            completed: Number.isFinite(completed) ? Math.max(0, completed) : 0,
                            total: Number.isFinite(total) ? Math.max(0, total) : 0,
                            ratio, item,
                        });
                    } catch { /* 隔离界面观察者 */ }
                }, finish);
            } catch (error) {
                finish(error instanceof Error ? error : new Error(String(error)));
            }
        });
        return {
            promise,
            cancel: () => {
                if (settled) return false;
                settled = true;
                observer = undefined;
                rejectTask(new SceneOperationError('CANCELLED', `已取消场景预加载后续切换：${name}`));
                return true;
            },
        };
    }

    // Creator loadScene 没有进度或取消接口；完成回调表示实际启动结果。
    load(name: string): Promise<void> {
        return new Promise<void>((resolve, reject) => {
            let settled = false;
            const finish = (error: Error | null, code: SceneErrorCode = 'LOAD_FAILED'): void => {
                if (settled) return;
                settled = true;
                if (error) reject(new SceneOperationError(code, `场景加载失败：${name}`, error));
                else resolve();
            };
            try {
                if (!this.backend.load(name, error => finish(error))) {
                    finish(new Error('Creator 拒绝启动加载'), 'LOAD_REJECTED');
                }
            } catch (error) {
                // 同步异常可能发生于旧场景销毁之后，不能声称旧场景仍可用。
                finish(error instanceof Error ? error : new Error(String(error)), 'LOAD_INTERRUPTED');
            }
        });
    }
}
