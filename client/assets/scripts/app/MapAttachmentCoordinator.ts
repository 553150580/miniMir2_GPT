import { attachmentKey } from './MapTravelModel';
import type { AttachmentIdentity } from './MapTravelModel';

export class StaleMapAttachmentError extends Error { readonly staleMapAttachment = true; }
export interface DisposableMap { dispose(): void; }
export interface MapAttachmentSteps<A extends DisposableMap, V extends DisposableMap> {
    load(): Promise<A>;
    current(): boolean;
    mount(assets: A): V;
    prepare(view: V): Promise<void>;
    acknowledge(view: V): void;
}
// 同一个Game保存场景里替换表现；过期的异步资源只能释放自己，不能重新挂接或发送旧ACK。
export class MapAttachmentCoordinator<A extends DisposableMap, V extends DisposableMap> {
    private generation = 0;
    private target = '';
    private task?: Promise<V>;
    private view?: V;
    private disposed = false;
    private readonly released = new WeakSet<V>();
    attach(identity: AttachmentIdentity, steps: MapAttachmentSteps<A, V>): Promise<V> {
        if (this.disposed) return Promise.reject(new StaleMapAttachmentError('地图附着协调器已销毁'));
        const key = attachmentKey(identity);
        if (this.target === key && this.task) return this.task;
        this.target = key; const generation = ++this.generation;
        const active = (): boolean => !this.disposed && this.generation === generation && this.target === key && steps.current();
        const task = (async () => {
            let assets: A | undefined, mounted: V | undefined;
            try {
                assets = await steps.load(); if (!active()) throw new StaleMapAttachmentError('地图资源回调已过期');
                if (this.view) this.release(this.view); this.view = undefined;
                mounted = steps.mount(assets); assets = undefined; this.view = mounted;
                await steps.prepare(mounted); if (!active() || this.view !== mounted) throw new StaleMapAttachmentError('地图首视口回调已过期');
                steps.acknowledge(mounted); return mounted;
            } catch (error) {
                assets?.dispose();
                if (mounted) { if (this.view === mounted) this.view = undefined; this.release(mounted); }
                if (this.target === key && this.generation === generation) { this.target = ''; this.task = undefined; }
                throw error;
            }
        })();
        void task.catch(() => undefined); this.task = task; return task;
    }
    invalidate(): void { this.generation++; this.target = ''; this.task = undefined; }
    dispose(): void { if (this.disposed) return; this.disposed = true; this.invalidate(); if (this.view) this.release(this.view); this.view = undefined; }
    private release(view: V): void { if (!this.released.has(view)) { this.released.add(view); view.dispose(); } }
}
