import { gfx, instantiate, isValid, Material, Node, Sprite, SpriteFrame, UITransform, Vec3 } from 'cc';
import type { SourceChunkLease, SourceChunkValue } from './SourceChunkCache';
import { compareSourceDraws } from './SourceMapModel';
import type { SourceDraw, SourcePoint, SourceRect } from './SourceMapModel';
import type { SourceChunkAssets, SourceMapAssets } from './SourceMapAssets';

export class SourceMapViewportCancelledError extends Error {}
export interface SourceMapEntityOrder {
    readonly id: string; readonly node: Node; readonly x: number; readonly y: number;
    // 默认按逻辑y所在行排序；移动显示坐标与权威行分开时由调用者明确提供row。
    readonly row?: number;
}
interface ChunkHold {
    readonly id: string; readonly lease: SourceChunkLease<SourceChunkAssets>; readonly assetsGeneration: number;
    promise: Promise<SourceChunkValue<SourceChunkAssets>>; loaded?: SourceChunkValue<SourceChunkAssets>;
}
interface Visual {
    readonly id: string; readonly owner: string; readonly node: Node; readonly sprite: Sprite; readonly transform: UITransform;
    key: string; blend: number; draw: SourceDraw;
}
interface ViewportJob {
    readonly revision: number; readonly promise: Promise<void>;
    readonly resolve: () => void; readonly reject: (error: unknown) => void;
    failed: boolean;
}
interface WorldOrder { readonly node: Node; readonly row: number; readonly kind: number; readonly x: number; readonly layer: number; readonly id: string; }

// 只实例化保存的MapProp预制体，不创建固定容器或满图节点。来源资源生命周期由外部owner管理。
// world的本地原点对应来源像素(0,0)，surface/entities沿用其零位移、单位缩放坐标系。
export class SourceMapView {
    private readonly chunks = new Map<string, ChunkHold>();
    private readonly visuals = new Map<string, Visual>();
    private entityOrder: readonly SourceMapEntityOrder[] = [];
    private viewport: SourceRect | null = null;
    private nowMs = 0;
    private revision = 0;
    private assetsGeneration: number;
    private job?: ViewportJob;
    private additiveMaterial?: Material;
    private disposed = false;
    private failure: unknown;
    private invalidated = false;
    private prepared = false;
    private stopInvalidation?: () => void;

    constructor(private readonly assets: SourceMapAssets, private readonly world: Node,
        private readonly surface: Node, private readonly entities: Node) {
        if (!this.containersValid() || surface === entities || surface === world || entities === world
            || surface.parent !== world || entities.parent !== world) throw new Error('来源地图必须注入保存的World及其独立Surface/Entities容器');
        for (const container of [surface, entities]) {
            if (container.position.x !== 0 || container.position.y !== 0 || container.position.z !== 0
                || container.scale.x !== 1 || container.scale.y !== 1 || container.scale.z !== 1
                || container.angle !== 0) throw new Error('来源地图Surface/Entities须沿用World的零位移、单位缩放坐标系');
        }
        if (assets.isDisposed) throw new Error('来源地图资产已释放');
        this.assetsGeneration = assets.generation;
        this.stopInvalidation = assets.onBeforeInvalidation(() => {
            if (this.disposed) return;
            // 资源owner归零引用前同步解绑，不能等下一帧轮询generation再清理Sprite。
            this.invalidated = true; this.prepared = false;
            this.cancelJob('来源地图资源即将失效'); this.clearChunks(); this.destroyMaterial();
        });
    }

    get ready(): Promise<void> { return this.job?.promise ?? Promise.resolve(); }
    get lastError(): unknown { return this.failure; }
    get isReady(): boolean { return !this.disposed && this.prepared && !this.invalidated && !this.assets.isDisposed && this.assetsGeneration === this.assets.generation; }
    get stats(): { chunks: number; pendingChunks: number; readyChunks: number; visibleNodes: number; floorNodes: number; objectNodes: number;
        viewportRevision: number; assetsGeneration: number; disposed: boolean } {
        const chunks = Array.from(this.chunks.values()), visuals = Array.from(this.visuals.values());
        return { chunks: chunks.length, pendingChunks: chunks.filter(value => !value.loaded).length, readyChunks: chunks.filter(value => !!value.loaded).length,
            visibleNodes: visuals.length, floorNodes: visuals.filter(value => value.draw.layer <= 2).length, objectNodes: visuals.filter(value => value.draw.layer >= 3).length,
            viewportRevision: this.revision, assetsGeneration: this.assetsGeneration, disposed: this.disposed };
    }

    // model.project给出格左上；实体既有身体偏移以格中心为原点，因此中心在同一投影上增加半格。
    grid(x: number, y: number): Vec3 { return this.pixelToLocal(this.assets.model.project(x + .5, y + .5)); }
    pixelToLocal(point: SourcePoint): Vec3 {
        if (![point.x, point.y].every(Number.isFinite)) throw new Error('来源像素坐标无效');
        return new Vec3(point.x, -point.y, 0);
    }
    localToPixel(point: SourcePoint): SourcePoint {
        if (![point.x, point.y].every(Number.isFinite)) throw new Error('来源本地坐标无效');
        return { x: point.x, y: -point.y };
    }
    cellAtLocal(point: SourcePoint): SourcePoint | null { return this.assets.model.cellAt(this.localToPixel(point)); }

    // 相同视口保留同一准备任务；动画时刻更新不会取消正在加载的块。
    setViewport(viewport: SourceRect, nowMs: number): Promise<void> {
        this.ensureAlive(); this.validateTime(nowMs); this.validateViewport(viewport);
        const needed = this.assets.model.queryChunks(viewport);
        const changedAssets = this.synchronizeAssets(); this.nowMs = nowMs;
        if (!changedAssets && this.viewport && this.viewport.every((value, index) => value === viewport[index]) && this.job && !this.job.failed) {
            this.render(); return this.job.promise;
        }
        this.cancelJob('来源地图视口已切换');
        this.viewport = Object.freeze(viewport.slice()) as unknown as SourceRect; this.failure = undefined;
        const job = this.createJob(); this.job = job;
        const ids = new Set(needed.map(info => info.id));
        for (const hold of Array.from(this.chunks.values())) if (!ids.has(hold.id)) this.dropChunk(hold);
        try {
            const pending: Promise<SourceChunkValue<SourceChunkAssets>>[] = [];
            for (const info of needed) {
                let hold = this.chunks.get(info.id);
                if (!hold) hold = this.acquireChunk(info.id);
                pending.push(hold.promise);
            }
            // 准备期间立即移除已离开视口的实例，同时保留已就绪的重叠部分。
            this.render();
            void Promise.all(pending).then(() => {
                if (this.job !== job || this.disposed) return;
                try { this.ensureAlive(); this.render(); this.prepared = true; job.resolve(); } catch (error) { this.failJob(job, error); }
            }, error => { this.failJob(job, error); });
        } catch (error) {
            this.clearChunks(); this.failJob(job, error);
        }
        return job.promise;
    }

    // 每帧只更新已取得租约的可视实例，不在播放中按单帧发起资源加载。
    update(nowMs: number): void {
        this.ensureAlive(); this.validateTime(nowMs); this.nowMs = nowMs;
        if (this.synchronizeAssets()) {
            if (this.viewport) void this.setViewport(this.viewport, nowMs).catch(() => undefined);
            return;
        }
        this.render();
    }

    // 外部实体只参与排序，位置与销毁仍由原实体owner管理。每源行先物件，随后该行实体。
    setEntities(entries: readonly SourceMapEntityOrder[]): void {
        this.ensureAlive(); const ids = new Set<string>(), nodes = new Set<Node>();
        const copied: SourceMapEntityOrder[] = [];
        for (const entry of entries) {
            if (!entry.id || ids.has(entry.id) || nodes.has(entry.node) || ![entry.x, entry.y].every(Number.isFinite)
                || (entry.row !== undefined && (!Number.isSafeInteger(entry.row) || entry.row < 0 || entry.row >= this.assets.model.manifest.height))) throw new Error('来源地图实体排序输入无效');
            if (!isValid(entry.node, true)) continue;
            if (entry.node.parent !== this.entities || Array.from(this.visuals.values()).some(visual => visual.node === entry.node)) throw new Error('来源地图实体须由外部owner放入保存的Entities容器');
            ids.add(entry.id); nodes.add(entry.node); copied.push(Object.freeze({ ...entry }));
        }
        this.entityOrder = Object.freeze(copied); this.sortNodes();
    }

    dispose(): void {
        if (this.disposed) return; this.disposed = true;
        this.prepared = false; this.stopInvalidation?.(); this.stopInvalidation = undefined;
        this.cancelJob('来源地图视图已销毁'); this.clearChunks(); this.destroyMaterial(); this.entityOrder = []; this.viewport = null;
    }

    private createJob(): ViewportJob {
        if (!Number.isSafeInteger(this.revision + 1)) throw new Error('来源地图视口代次耗尽');
        let resolve!: () => void, reject!: (error: unknown) => void;
        const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
        void promise.catch(() => undefined);
        return { revision: ++this.revision, promise, resolve, reject, failed: false };
    }
    private cancelJob(reason: string): void {
        this.prepared = false;
        this.job?.reject(new SourceMapViewportCancelledError(reason)); this.job = undefined;
    }
    private failJob(job: ViewportJob, error: unknown): void {
        if (this.job !== job || this.disposed) return;
        job.failed = true; this.prepared = false; this.failure = error; job.reject(error);
    }
    private acquireChunk(id: string): ChunkHold {
        const lease = this.assets.acquireChunk(id);
        const hold: ChunkHold = { id, lease, assetsGeneration: this.assetsGeneration, promise: lease.ready };
        this.chunks.set(id, hold);
        hold.promise = lease.ready.then(loaded => {
            if (this.disposed || this.chunks.get(id) !== hold || hold.assetsGeneration !== this.assets.generation) throw new SourceMapViewportCancelledError('来源块回调已过期');
            this.ensureAlive(); hold.loaded = loaded; this.render(); return loaded;
        }).catch(error => {
            if (this.chunks.get(id) === hold) this.dropChunk(hold);
            throw error;
        });
        void hold.promise.catch(() => undefined); return hold;
    }
    private dropChunk(hold: ChunkHold): void {
        if (this.chunks.get(hold.id) !== hold) return;
        this.chunks.delete(hold.id);
        for (const visual of Array.from(this.visuals.values())) if (visual.owner === hold.id) this.removeVisual(visual);
        // Sprite先解绑，随后才归还可能触发缓存淘汰的资源租约。
        hold.loaded = undefined; hold.lease.release();
    }
    private clearChunks(): void {
        for (const visual of Array.from(this.visuals.values())) this.removeVisual(visual);
        for (const hold of Array.from(this.chunks.values())) this.dropChunk(hold);
    }
    private synchronizeAssets(): boolean {
        if (!this.invalidated && this.assetsGeneration === this.assets.generation) return false;
        this.cancelJob('来源地图资源代次已过期'); this.clearChunks(); this.destroyMaterial();
        this.assetsGeneration = this.assets.generation; this.invalidated = false; return true;
    }
    private render(): void {
        if (!this.viewport || this.disposed) return;
        if (!this.containersValid() || this.assets.isDisposed) { this.dispose(); throw new SourceMapViewportCancelledError('来源地图保存容器或资产已销毁'); }
        const draws: SourceDraw[] = [];
        for (const hold of Array.from(this.chunks.values())) if (hold.loaded) draws.push(...this.assets.model.draws(hold.loaded.chunk, this.nowMs, this.viewport));
        draws.sort(compareSourceDraws);
        const prepared = draws.map(draw => {
            const frame = this.chunks.get(draw.owner)?.loaded?.value.frames.get(draw.key);
            if (!frame) throw new Error(`来源地图块缺少已准备帧：${draw.owner}/${draw.key}`);
            return { draw, frame, id: `${draw.owner}/${draw.x}/${draw.y}/${draw.layer}` };
        });
        const active = new Set(prepared.map(value => value.id));
        for (const visual of Array.from(this.visuals.values())) if (!active.has(visual.id)) this.removeVisual(visual);
        for (const { draw, frame, id } of prepared) this.draw(id, draw, frame);
        this.sortNodes();
    }
    private draw(id: string, draw: SourceDraw, frame: SpriteFrame): void {
        let visual = this.visuals.get(id);
        if (!visual) {
            const node = instantiate(this.assets.propPrefab); node.active = false;
            const sprite = node.getComponent(Sprite), transform = node.getComponent(UITransform);
            if (!sprite || !transform) { node.destroy(); throw new Error('保存的MapProp预制体必须含Sprite和UITransform'); }
            node.name = `SourceMap:${id}`; node.parent = draw.layer <= 2 ? this.surface : this.entities;
            node.setScale(1, 1, 1); node.angle = 0; transform.setAnchorPoint(0, 1);
            sprite.type = Sprite.Type.SIMPLE; sprite.trim = false; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
            visual = { id, owner: draw.owner, node, sprite, transform, key: '', blend: -1, draw };
            this.visuals.set(id, visual);
        }
        if (!isValid(visual.node, true)) throw new Error('来源地图动态节点被外部owner销毁');
        visual.draw = draw;
        if (visual.key !== draw.key || visual.sprite.spriteFrame !== frame) {
            visual.sprite.spriteFrame = frame; visual.transform.setContentSize(draw.frame.width, draw.frame.height); visual.key = draw.key;
        }
        if (visual.blend !== draw.blend) {
            visual.sprite.customMaterial = draw.blend ? this.getAdditiveMaterial(visual.sprite) : null; visual.blend = draw.blend;
        }
        // bounds已含来源底边锚点与火把特例偏移，不再次叠加WIL offset，也不缩放原PNG。
        visual.node.setPosition(draw.bounds[0], -draw.bounds[1], 0); visual.node.active = true;
    }
    private getAdditiveMaterial(sprite: Sprite): Material {
        if (this.additiveMaterial) return this.additiveMaterial;
        const base = sprite.getSharedMaterial(0); if (!base) throw new Error('MapProp尚无可复制的Creator Sprite材质');
        const material = new Material();
        try {
            material.name = '来源城镇加色';
            // 与保存Effect/Beam的SRC_ALPHA、ONE一致；通过公开材质API设置，避免写引擎私有字段。
            // 3.8.8的Material资产overridePipelineStates仅警告并返回，须在copy初始化时传入states。
            material.copy(base, { states: { blendState: { targets: [{ blend: true,
                blendSrc: gfx.BlendFactor.SRC_ALPHA, blendDst: gfx.BlendFactor.ONE,
                blendSrcAlpha: gfx.BlendFactor.ONE, blendDstAlpha: gfx.BlendFactor.ONE_MINUS_SRC_ALPHA }] } } });
        } catch (error) { material.destroy(); throw error; }
        this.additiveMaterial = material; return material;
    }
    private sortNodes(): void {
        const visuals = Array.from(this.visuals.values()).filter(value => isValid(value.node, true));
        visuals.filter(value => value.draw.layer <= 2).sort((a, b) => compareSourceDraws(a.draw, b.draw))
            .forEach((value, index) => { if (value.node.getSiblingIndex() !== index) value.node.setSiblingIndex(index); });
        const world: WorldOrder[] = visuals.filter(value => value.draw.layer >= 3).map(value => ({ node: value.node, row: value.draw.y, kind: 0, x: value.draw.x, layer: value.draw.layer, id: value.id }));
        for (const entity of this.entityOrder) if (isValid(entity.node, true) && entity.node.parent === this.entities)
            world.push({ node: entity.node, row: entity.row ?? Math.floor(entity.y), kind: 1, x: entity.x, layer: 0, id: entity.id });
        world.sort((a, b) => a.row - b.row || a.kind - b.kind || a.x - b.x || a.layer - b.layer || a.id.localeCompare(b.id))
            .forEach((value, index) => { if (value.node.getSiblingIndex() !== index) value.node.setSiblingIndex(index); });
    }
    private removeVisual(visual: Visual): void {
        this.visuals.delete(visual.id);
        if (!isValid(visual.node, true)) return;
        visual.node.active = false; visual.sprite.spriteFrame = null; visual.sprite.customMaterial = null;
        visual.node.parent = null; visual.node.destroy();
    }
    private destroyMaterial(): void {
        if (this.additiveMaterial) { this.additiveMaterial.destroy(); this.additiveMaterial = undefined; }
    }
    private containersValid(): boolean { return isValid(this.world, true) && isValid(this.surface, true) && isValid(this.entities, true); }
    private ensureAlive(): void {
        if (this.disposed) throw new SourceMapViewportCancelledError('来源地图视图已销毁');
        if (!this.containersValid() || this.assets.isDisposed) { this.dispose(); throw new SourceMapViewportCancelledError('来源地图保存容器或资产已销毁'); }
    }
    private validateTime(nowMs: number): void {
        if (!Number.isFinite(nowMs) || nowMs < 0 || nowMs > Number.MAX_SAFE_INTEGER) throw new Error('来源地图动画时刻无效');
    }
    private validateViewport(viewport: SourceRect): void {
        if (!Array.isArray(viewport) || viewport.length !== 4 || !viewport.every(Number.isFinite)
            || viewport[2] < viewport[0] || viewport[3] < viewport[1]) throw new Error('来源地图视口必须是有限像素半开矩形');
    }
}
