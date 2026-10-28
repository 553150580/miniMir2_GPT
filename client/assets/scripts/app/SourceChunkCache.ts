import { SourceChunkInfo, SourceMapChunk, SourceMapModel } from './SourceMapModel';

export class SourceChunkCancelledError extends Error {}
export class SourceChunkCapacityError extends Error {}
export interface SourceChunkCancellation {
    readonly cancelled: boolean;
    onCancel(listener: () => void): () => void;
}
export interface LoadedSourceChunk<T> {
    readonly chunk: unknown; readonly value: T;
    // 桥接层持有的JSON、纹理等引用统一由此释放，模型保证每个成功加载结果仅调用一次。
    readonly release: () => void;
}
export interface SourceChunkValue<T> { readonly chunk: SourceMapChunk; readonly value: T; }
export interface SourceChunkLease<T> { readonly ready: Promise<SourceChunkValue<T>>; release(): void; }
export interface SourceChunkCacheOptions<T> {
    readonly model: SourceMapModel; readonly concurrency: number; readonly capacity: number;
    // 可取得原始JSON字节的桥接层应先按info.sha256核对，再返回对象；本缓存只验证结构与生命周期。
    // Cocos JsonAsset仅交付导入后的解析对象，该桥接须明确记录运行原字节SHA未验证；
    // 来源摘要由资源生成复验、实际资源对拍另行记录，结构验证不能冒称原字节摘要验证或验签。
    readonly loader: (info: SourceChunkInfo, cancellation: SourceChunkCancellation) => Promise<LoadedSourceChunk<T>>;
}
class Cancellation implements SourceChunkCancellation {
    cancelled = false;
    private readonly listeners = new Set<() => void>();
    onCancel(listener: () => void): () => void {
        if (this.cancelled) listener(); else this.listeners.add(listener);
        return () => { this.listeners.delete(listener); };
    }
    cancel(): void {
        if (this.cancelled) return; this.cancelled = true;
        const snapshot = Array.from(this.listeners); this.listeners.clear();
        for (const listener of snapshot) { try { listener(); } catch { /* 取消不能被桥接回调打断。 */ } }
    }
}
interface Waiter<T> {
    readonly resolve: (value: SourceChunkValue<T>) => void;
    readonly reject: (reason: unknown) => void;
}
interface Entry<T> {
    readonly id: string; readonly generation: number; readonly cancel: Cancellation;
    readonly waiters: Set<Waiter<T>>;
    state: 'queued' | 'loading' | 'ready' | 'removed'; lastUsed: number;
    result?: SourceChunkValue<T>; resource?: LoadedSourceChunk<T>;
}
// 容量统计包含排队、加载和已缓存块。已释放的就绪块保留缓存引用，至淘汰/换代/销毁时释放。
// 不可取消的旧加载仍占并发名额，迟到结果必须释放后才允许补入新的请求。
export class SourceChunkCache<T> {
    private readonly entries = new Map<string, Entry<T>>();
    private active = 0;
    private epoch = 0;
    private clock = 0;
    private disposed = false;
    private releaseError: unknown;
    private releaseErrorCount = 0;
    readonly model: SourceMapModel;
    private readonly concurrency: number;
    private readonly capacity: number;
    private readonly loader: SourceChunkCacheOptions<T>['loader'];
    constructor(options: SourceChunkCacheOptions<T>) {
        if (!Number.isSafeInteger(options.concurrency) || options.concurrency <= 0
            || !Number.isSafeInteger(options.capacity) || options.capacity <= 0) throw new Error('来源块缓存必须显式提供正整数并发及容量');
        this.model = options.model; this.concurrency = options.concurrency; this.capacity = options.capacity; this.loader = options.loader;
    }
    get generation(): number { return this.epoch; }
    get stats(): { entries: number; active: number; queued: number; ready: number; pinned: number; releaseErrorCount: number; lastReleaseError: unknown } {
        const entries = Array.from(this.entries.values());
        return { entries: entries.length, active: this.active, queued: entries.filter(e => e.state === 'queued').length,
            ready: entries.filter(e => e.state === 'ready').length, pinned: entries.filter(e => e.waiters.size > 0).length,
            releaseErrorCount: this.releaseErrorCount, lastReleaseError: this.releaseError };
    }
    acquire(id: string): SourceChunkLease<T> {
        if (this.disposed) throw new SourceChunkCancelledError('来源块缓存已经销毁');
        this.model.chunkInfo(id);
        let entry = this.entries.get(id);
        if (!entry) {
            this.makeRoom();
            entry = { id, generation: this.epoch, cancel: new Cancellation(), waiters: new Set(), state: 'queued', lastUsed: ++this.clock };
            this.entries.set(id, entry);
        }
        const owned = entry; let resolve!: Waiter<T>['resolve'], reject!: Waiter<T>['reject'], released = false;
        const ready = new Promise<SourceChunkValue<T>>((yes, no) => { resolve = yes; reject = no; });
        // 调用方可以先释放再决定是否等待；内部处理拒绝，仍保留ready对调用方的拒绝语义。
        void ready.catch(() => undefined);
        const waiter: Waiter<T> = { resolve, reject }; owned.waiters.add(waiter); owned.lastUsed = ++this.clock;
        if (owned.state === 'ready') resolve(owned.result!);
        this.pump();
        return { ready, release: () => {
            if (released) return; released = true; owned.waiters.delete(waiter);
            reject(new SourceChunkCancelledError('来源块引用已释放'));
            owned.lastUsed = ++this.clock;
            if (!owned.waiters.size && (owned.state === 'queued' || owned.state === 'loading')) this.remove(owned, '来源块请求已取消');
            this.pump();
        } };
    }
    replaceGeneration(): number {
        if (this.disposed) throw new SourceChunkCancelledError('来源块缓存已经销毁');
        if (!Number.isSafeInteger(this.epoch + 1)) throw new Error('来源块缓存代次耗尽');
        this.epoch++;
        for (const entry of Array.from(this.entries.values())) this.remove(entry, '来源块代次已过期');
        return this.epoch;
    }
    dispose(): void {
        if (this.disposed) return; this.disposed = true;
        for (const entry of Array.from(this.entries.values())) this.remove(entry, '来源块缓存已经销毁');
    }
    private makeRoom(): void {
        while (this.entries.size >= this.capacity) {
            const idle = Array.from(this.entries.values()).filter(e => e.state === 'ready' && !e.waiters.size).sort((a, b) => a.lastUsed - b.lastUsed);
            if (!idle.length) throw new SourceChunkCapacityError('来源块缓存容量已被使用中的引用占满');
            this.remove(idle[0], '来源块缓存淘汰');
        }
    }
    private remove(entry: Entry<T>, reason: string): void {
        if (entry.state === 'removed') return;
        if (this.entries.get(entry.id) === entry) this.entries.delete(entry.id);
        entry.state = 'removed'; entry.cancel.cancel();
        for (const waiter of Array.from(entry.waiters)) waiter.reject(new SourceChunkCancelledError(reason));
        entry.waiters.clear();
        if (entry.resource) { const resource = entry.resource; entry.resource = undefined; this.release(resource); }
        entry.result = undefined;
    }
    private release(resource: LoadedSourceChunk<T>): void {
        try { resource.release(); } catch (error) { this.releaseError = error; this.releaseErrorCount++; }
    }
    private pump(): void {
        if (this.disposed) return;
        for (const entry of Array.from(this.entries.values())) {
            if (this.active >= this.concurrency) break;
            if (entry.state !== 'queued') continue;
            entry.state = 'loading'; this.active++; void this.load(entry);
        }
    }
    private async load(entry: Entry<T>): Promise<void> {
        let resource: LoadedSourceChunk<T> | undefined;
        try {
            resource = await this.loader(this.model.chunkInfo(entry.id), entry.cancel);
            if (!resource || typeof resource.release !== 'function') throw new Error('来源块加载器必须返回可释放的资源');
            if (entry.cancel.cancelled || this.disposed || entry.generation !== this.epoch || this.entries.get(entry.id) !== entry) {
                this.release(resource); resource = undefined; return;
            }
            const chunk = this.model.parseChunk(resource.chunk, entry.id);
            entry.result = Object.freeze({ chunk, value: resource.value }); entry.resource = resource; resource = undefined;
            entry.state = 'ready'; entry.lastUsed = ++this.clock;
            for (const waiter of Array.from(entry.waiters)) waiter.resolve(entry.result);
        } catch (error) {
            if (resource && typeof resource.release === 'function') this.release(resource);
            if (this.entries.get(entry.id) === entry) {
                this.entries.delete(entry.id); entry.state = 'removed'; entry.cancel.cancel();
                for (const waiter of Array.from(entry.waiters)) waiter.reject(error); entry.waiters.clear();
            }
        } finally { this.active--; this.pump(); }
    }
}
