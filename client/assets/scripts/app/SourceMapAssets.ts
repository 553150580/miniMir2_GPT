import { Asset, JsonAsset, Prefab, resources, SpriteFrame } from 'cc';
import { SourceChunkCache, SourceChunkCancellation, SourceChunkCancelledError, SourceChunkLease, LoadedSourceChunk } from './SourceChunkCache';
import { SourceChunkInfo, SourceMapModel, sourceAnimationKey } from './SourceMapModel';

export interface SourceMapAssetOptions {
    readonly concurrency: number;
    readonly capacity: number;
}
export interface SourceChunkAssets { readonly frames: ReadonlyMap<string, SpriteFrame>; }
export type SourceMapAssetProgress = (completed: number, total: number) => void;

interface AssetRequest {
    readonly needed: () => boolean;
    readonly start: () => void;
    readonly reject: (reason: unknown) => void;
}

// JSON、帧和预制体共用这一实际请求队列；块并发不能再放大为无界的帧并发。
class SourceAssetQueue {
    private readonly pending: AssetRequest[] = [];
    private active = 0;
    private disposed = false;
    constructor(private readonly concurrency: number) {}
    get stats(): { active: number; queued: number } { return { active: this.active, queued: this.pending.length }; }

    load<T extends Asset>(path: string, type: any, needed: () => boolean): Promise<T> {
        return new Promise<T>((resolve, reject) => {
            if (this.disposed || !needed()) { reject(new SourceChunkCancelledError('来源资源请求已经取消')); return; }
            const request: AssetRequest = { needed, reject, start: () => {
                let settled = false;
                const complete = (error: Error | null, value?: T): void => {
                    if (settled) return; settled = true;
                    try {
                        if (error) reject(error);
                        else if (!value) reject(new Error(`来源资源加载为空：${path}`));
                        else {
                            // 即使底层无法取消，迟到结果也先取得并成对释放自己的引用。
                            value.addRef();
                            if (this.disposed || !needed()) { value.decRef(); reject(new SourceChunkCancelledError('来源资源结果已经过期')); }
                            else resolve(value);
                        }
                    } catch (failure) { reject(failure); }
                    finally { this.active--; this.pump(); }
                };
                try { resources.load(path, type, (error, value) => complete(error, value as T)); }
                catch (error) { complete(error instanceof Error ? error : new Error(String(error))); }
            } };
            this.pending.push(request); this.pump();
        });
    }

    prune(): void {
        for (let index = this.pending.length - 1; index >= 0; index--) {
            const request = this.pending[index];
            if (this.disposed || !request.needed()) { this.pending.splice(index, 1); request.reject(new SourceChunkCancelledError('来源资源排队请求已经取消')); }
        }
        this.pump();
    }
    dispose(): void { if (this.disposed) return; this.disposed = true; this.prune(); }
    private pump(): void {
        while (!this.disposed && this.active < this.concurrency && this.pending.length) {
            const request = this.pending.shift()!;
            if (!request.needed()) { request.reject(new SourceChunkCancelledError('来源资源排队请求已经过期')); continue; }
            this.active++; request.start();
        }
    }
}

interface FrameEntry {
    users: number;
    asset: SpriteFrame | null;
    promise?: Promise<SpriteFrame>;
}
interface FrameLease { readonly ready: Promise<SpriteFrame>; release(): void; }

// 只持有已准备清单、保存的物件模板及有界缓存块；不创建布局，也不预载整图。
export class SourceMapAssets {
    private readonly queue: SourceAssetQueue;
    readonly resourceRoot: string;
    private readonly frameEntries = new Map<string, FrameEntry>();
    private manifestAsset: JsonAsset | null = null;
    private savedPropPrefab: Prefab | null = null;
    private minimapLease: FrameLease | null = null;
    private minimapFrame: SpriteFrame | null = null;
    private sourceModel: SourceMapModel | null = null;
    private chunkCache: SourceChunkCache<SourceChunkAssets> | null = null;
    private readonly beforeInvalidation = new Set<() => void>();
    private invalidating = false;
    private disposed = false;
    // Creator导入JsonAsset仅提供解析对象；本类逐块执行SourceMapModel结构验证，运行原字节SHA保持false。
    // 原始文件摘要由资源生成复验、实际资源对拍另行核对；不对导入对象重编码冒称取得原始JSON或完成验签。
    readonly integrity = Object.freeze({ chunkValidation: 'creator_imported_json_structure_on_load', originalChunkBytesHashVerifiedAtRuntime: false });

    private constructor(readonly manifestKey: string, private readonly options: SourceMapAssetOptions) {
        // 配置提供完整manifest资源键；只在此处明确取一次父目录，不猜测目录或重复裁剪。
        if (!/^(?:[A-Za-z0-9_-]+\/)+manifest$/.test(manifestKey)) throw new Error('来源地图必须提供以/manifest结尾的完整资源键');
        this.resourceRoot = manifestKey.slice(0, -'/manifest'.length);
        if (!Number.isSafeInteger(options.concurrency) || options.concurrency <= 0
            || !Number.isSafeInteger(options.capacity) || options.capacity <= 0) throw new Error('来源地图资源必须显式提供正整数并发及缓存容量');
        this.queue = new SourceAssetQueue(options.concurrency);
    }

    // 进度对应实际准备的manifest、模板及显式配置的小地图；首次视口的块由绘制层另取租约。
    static async prepare(manifestKey: string, options: SourceMapAssetOptions, progress?: SourceMapAssetProgress): Promise<SourceMapAssets> {
        const result = new SourceMapAssets(manifestKey, { concurrency: options.concurrency, capacity: options.capacity });
        try {
            progress?.(0, 2);
            result.manifestAsset = await result.queue.load<JsonAsset>(manifestKey, JsonAsset, () => !result.disposed);
            result.sourceModel = new SourceMapModel(result.manifestAsset.json);
            const minimapKey = result.sourceModel.manifest.minimap?.frame;
            const total = minimapKey ? 3 : 2;
            progress?.(1, total);
            result.savedPropPrefab = await result.queue.load<Prefab>('prefabs/MapProp', Prefab, () => !result.disposed);
            if (minimapKey) {
                progress?.(2, total);
                result.minimapLease = result.retainFrame(minimapKey);
                result.minimapFrame = await result.minimapLease.ready;
            }
            result.chunkCache = new SourceChunkCache<SourceChunkAssets>({ model: result.sourceModel,
                concurrency: result.options.concurrency, capacity: result.options.capacity, loader: (info, cancellation) => result.loadChunk(info, cancellation) });
            progress?.(total, total);
            return result;
        } catch (error) { result.dispose(); throw error; }
    }

    get model(): SourceMapModel { if (!this.sourceModel) throw new Error('来源地图清单尚未准备'); return this.sourceModel; }
    get propPrefab(): Prefab { if (!this.savedPropPrefab) throw new SourceChunkCancelledError('来源地图模板尚未准备或已经释放'); return this.savedPropPrefab; }
    // 缺配置时保持null，城镇HUD不能借用个人地图背景冒充来源小地图。
    get minimap(): SpriteFrame | null { return this.minimapFrame; }
    get generation(): number { return this.chunkCache?.generation ?? 0; }
    get isDisposed(): boolean { return this.disposed; }
    get stats() {
        const entries = Array.from(this.frameEntries.values());
        return { disposed: this.disposed, generation: this.generation, chunks: this.chunkCache?.stats ?? null,
            requests: this.queue.stats, sharedFrames: entries.length, retainedFrames: entries.filter(entry => entry.asset !== null).length,
            frameReferences: entries.reduce((sum, entry) => sum + entry.users, 0) };
    }

    acquireChunk(id: string): SourceChunkLease<SourceChunkAssets> {
        if (this.disposed || this.invalidating || !this.chunkCache) throw new SourceChunkCancelledError('来源地图资源已经释放或正在换代');
        return this.chunkCache.acquire(id);
    }
    // 所有使用本实例SpriteFrame的视图都应同步解绑，再由本类释放底层资源引用。
    onBeforeInvalidation(listener: () => void): () => void {
        if (this.disposed || this.invalidating) throw new SourceChunkCancelledError('来源地图资源已经释放或正在换代');
        this.beforeInvalidation.add(listener);
        return () => { this.beforeInvalidation.delete(listener); };
    }
    replaceGeneration(): number {
        if (this.disposed || !this.chunkCache) throw new SourceChunkCancelledError('来源地图资源已经释放');
        this.beginInvalidation();
        try { const generation = this.chunkCache.replaceGeneration(); this.queue.prune(); return generation; }
        finally { this.invalidating = false; }
    }

    private beginInvalidation(): void {
        if (this.invalidating) throw new Error('来源地图资源失效通知不能重入');
        this.invalidating = true;
        let failed = false, failure: unknown;
        for (const listener of Array.from(this.beforeInvalidation)) {
            try { listener(); } catch (error) { if (!failed) { failed = true; failure = error; } }
        }
        // 解绑回调失败时保留资源，防止仍有Sprite绑定却继续销毁Frame；所有订阅者仍各通知一次。
        if (failed) { this.invalidating = false; throw failure; }
    }

    private retainFrame(key: string): FrameLease {
        if (this.disposed) throw new SourceChunkCancelledError('来源地图资源已经释放');
        let entry = this.frameEntries.get(key);
        if (entry) entry.users++;
        else {
            const definition = this.model.manifest.frames[key];
            if (!definition || definition.declared_empty || !definition.png) throw new Error(`来源图块缺少可显示原帧：${key}`);
            const owned: FrameEntry = { users: 1, asset: null };
            entry = owned; this.frameEntries.set(key, owned);
            const path = `${this.resourceRoot}/${definition.png.replace(/\.png$/, '')}/spriteFrame`;
            owned.promise = this.queue.load<SpriteFrame>(path, SpriteFrame, () => !this.disposed && owned.users > 0).then(asset => {
                if (this.disposed || !owned.users) { asset.decRef(); throw new SourceChunkCancelledError('来源原帧引用已经过期'); }
                owned.asset = asset; return asset;
            }).catch(error => { if (this.frameEntries.get(key) === owned) this.frameEntries.delete(key); throw error; });
            // 最后一个消费者可以在回调到达前取消，保留调用方拒绝语义并处理内部拒绝。
            void owned.promise.catch(() => undefined);
        }
        const owned = entry; let released = false;
        return { ready: owned.promise!, release: () => {
            if (released) return; released = true; owned.users--;
            if (!owned.users) {
                // 待拒绝的旧Promise也立即离开索引，避免同帧新代请求复用已取消的排队任务。
                if (this.frameEntries.get(key) === owned) this.frameEntries.delete(key);
                if (owned.asset) { const asset = owned.asset; owned.asset = null; asset.decRef(); }
            }
            this.queue.prune();
        } };
    }

    private async loadChunk(info: SourceChunkInfo, cancellation: SourceChunkCancellation): Promise<LoadedSourceChunk<SourceChunkAssets>> {
        let json: JsonAsset | null = null, transferred = false, failed = false, failure: unknown;
        const leases: FrameLease[] = [], frames = new Map<string, SpriteFrame>();
        const needed = (): boolean => !this.disposed && !cancellation.cancelled;
        const check = (): void => { if (!needed()) throw new SourceChunkCancelledError('来源图块加载代次已经过期'); };
        const releaseFrames = (): void => { for (const lease of leases.splice(0)) lease.release(); };
        const stopCancellation = cancellation.onCancel(() => { releaseFrames(); this.queue.prune(); });
        try {
            check();
            if (!info.path.endsWith('.json')) throw new Error('来源图块必须是显式JSON资源');
            json = await this.queue.load<JsonAsset>(`${this.resourceRoot}/${info.path.replace(/\.json$/, '')}`, JsonAsset, needed);
            check();
            const chunk = this.model.parseChunk(json.json, info.id), keys = new Set<string>();
            const add = (key: string): void => {
                const frame = this.model.manifest.frames[key];
                if (!frame) throw new Error(`来源图块缺少原帧定义：${key}`);
                if (!frame.declared_empty) keys.add(key);
            };
            for (const draw of chunk.floor) add(draw[3]);
            for (const draw of chunk.objects) for (let index = 0; index < Math.max(draw[4], 1); index++) {
                add(sourceAnimationKey(draw[3], draw[4], 0, index * 100));
            }
            const pending = Array.from(keys); let next = 0;
            // 每块只创建有界工作者；所有工作者仍受全局实际请求队列限制。
            await Promise.all(Array.from({ length: Math.min(this.options.concurrency, pending.length) }, async () => {
                while (!failed && needed() && next < pending.length) {
                    const key = pending[next++];
                    try {
                        const lease = this.retainFrame(key); leases.push(lease);
                        const frame = await lease.ready; check();
                        if (failed) break;
                        frames.set(key, frame);
                    } catch (error) { if (!failed) { failed = true; failure = error; } releaseFrames(); }
                }
            }));
            check();
            if (failed) throw failure;
            let released = false;
            const resource: LoadedSourceChunk<SourceChunkAssets> = { chunk, value: Object.freeze({ frames }), release: () => {
                if (released) return; released = true;
                releaseFrames(); frames.clear();
                if (json) { json.decRef(); json = null; }
            } };
            transferred = true; return resource;
        } finally {
            stopCancellation();
            if (!transferred) { releaseFrames(); frames.clear(); if (json) json.decRef(); }
        }
    }

    // 绘制层应先清理自己的节点及租约，再由地图生命周期所有者释放本资源实例。
    dispose(): void {
        if (this.disposed) return;
        this.beginInvalidation();
        try {
            this.disposed = true;
            this.chunkCache?.dispose();
            this.minimapLease?.release(); this.minimapLease = null; this.minimapFrame = null;
            this.queue.dispose();
            for (const entry of Array.from(this.frameEntries.values())) {
                if (entry.asset) { const asset = entry.asset; entry.asset = null; asset.decRef(); }
            }
            this.frameEntries.clear();
            if (this.savedPropPrefab) { this.savedPropPrefab.decRef(); this.savedPropPrefab = null; }
            if (this.manifestAsset) { this.manifestAsset.decRef(); this.manifestAsset = null; }
        } finally {
            this.beforeInvalidation.clear(); this.invalidating = false;
        }
    }
}
