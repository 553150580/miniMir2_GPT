import type { WorldAssets } from './WorldAssets';
import type { SourceChunkLease } from './SourceChunkCache';
import type { SourceChunkAssets } from './SourceMapAssets';

// 只取得目标首视口的资源租约，不创建节点、不修改当前world；新视图取得自己的租约后再归还。
export class MapViewportPreparation {
    private leases: SourceChunkLease<SourceChunkAssets>[] = [];
    private released = false;
    constructor(private readonly assets: WorldAssets) {}
    async prepare(position: { x: number; y: number }, width: number, height: number): Promise<void> {
        if (this.released || ![width, height].every(n => Number.isFinite(n) && n > 0) || ![position.x, position.y].every(Number.isSafeInteger)
            || position.x < 0 || position.y < 0 || position.x >= this.assets.map.width || position.y >= this.assets.map.height) throw new Error('目标地图准备位置或视口无效');
        if (!this.assets.sourceMap) return;
        const source = this.assets.sourceMap, g = source.model.manifest, point = source.model.project(position.x + .5, position.y + .5);
        const x = width >= g.pixel_width ? g.pixel_width / 2 : Math.max(width / 2, Math.min(g.pixel_width - width / 2, point.x));
        const y = height >= g.pixel_height ? g.pixel_height / 2 : Math.max(height / 2, Math.min(g.pixel_height - height / 2, point.y));
        try {
            for (const chunk of source.model.queryChunks([x - width / 2, y - height / 2, x + width / 2, y + height / 2])) this.leases.push(source.acquireChunk(chunk.id));
            await Promise.all(this.leases.map(lease => lease.ready));
            if (this.released) throw new Error('目标地图资源准备已取消');
        } catch (error) { this.release(); throw error; }
    }
    release(): void { if (this.released) return; this.released = true; for (const lease of this.leases.splice(0)) lease.release(); }
}
