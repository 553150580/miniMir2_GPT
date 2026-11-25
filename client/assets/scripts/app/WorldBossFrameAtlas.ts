import { JsonAsset, Rect, Size, SpriteFrame, Texture2D, resources } from 'cc';

const directory = 'development/world-boss-atlas';

interface AtlasPage {
    readonly name: string;
    readonly width: number;
    readonly height: number;
}

type AtlasRectangle = readonly [page: number, x: number, y: number, width: number, height: number, source: string];

interface AtlasCatalog {
    readonly schema_version: number;
    readonly pages: readonly AtlasPage[];
    readonly frames: Readonly<Record<string, AtlasRectangle>>;
}

interface OriginalFrame {
    readonly width: number;
    readonly height: number;
    readonly png_sha256: string;
}

// 只拥有本场的纹理引用和子帧。场景先解绑所有 Sprite，再释放图集帧和纹理。
export class WorldBossFrameAtlas {
    private readonly textures = new Set<Texture2D>();
    private readonly sprites = new Set<SpriteFrame>();

    static available(): boolean {
        return !!resources.getInfoWithPath(`${directory}/index`, JsonAsset);
    }

    async populate(keys: readonly string[], originals: Readonly<Record<string, OriginalFrame>>,
        target: Map<string, SpriteFrame>, progress: (loaded: number) => void): Promise<void> {
        const asset = await this.load<JsonAsset>(`${directory}/index`, JsonAsset);
        asset.addRef();
        try {
            const catalog = asset.json as AtlasCatalog;
            if (catalog?.schema_version !== 1 || !Array.isArray(catalog.pages) || !catalog.frames)
                throw new Error('世界首领图集索引格式错误');
            const pages = new Map<number, { key: string; source: AtlasRectangle }[]>();
            for (const key of keys) {
                const source = catalog.frames[key], original = originals[key];
                if (!Array.isArray(source) || source.length !== 6) throw new Error(`世界首领图集缺少原帧：${key}`);
                const [pageId, x, y, width, height, digest] = source;
                const page = catalog.pages[pageId];
                if (!page || !/^page-[0-9]+$/.test(page.name) || ![pageId, x, y, width, height, page.width, page.height].every(Number.isSafeInteger)
                    || pageId < 0 || x < 1 || y < 1 || width <= 0 || height <= 0 || x + width >= page.width || y + height >= page.height
                    || width !== original.width || height !== original.height || digest !== original.png_sha256)
                    throw new Error(`世界首领图集原帧尺寸或来源不符：${key}`);
                if (!pages.has(pageId)) pages.set(pageId, []);
                pages.get(pageId)!.push({ key, source });
            }

            // 同一页面只下载一次。等待所有已发请求结束后才允许外层回收引用。
            let done = 0;
            const jobs = Array.from(pages, async ([pageId, entries]) => {
                const page = catalog.pages[pageId];
                const texture = await this.load<Texture2D>(`${directory}/${page.name}/texture`, Texture2D);
                if (!this.textures.has(texture)) { texture.addRef(); this.textures.add(texture); }
                if (texture.width !== page.width || texture.height !== page.height) throw new Error(`世界首领图集纹理尺寸不符：${page.name}`);
                for (const { key, source: [, x, y, width, height] } of entries) {
                    const sprite = new SpriteFrame();
                    sprite.packable = false;
                    this.sprites.add(sprite);
                    sprite.reset({ texture, rect: new Rect(x, y, width, height), originalSize: new Size(width, height), isRotate: false });
                    target.set(key, sprite);
                }
                done += entries.length;
                progress(done);
            });
            const failed = (await Promise.allSettled(jobs)).find(job => job.status === 'rejected');
            if (failed?.status === 'rejected') throw failed.reason;
        } finally {
            asset.decRef();
        }
    }

    dispose(): void {
        for (const sprite of this.sprites) sprite.destroy();
        this.sprites.clear();
        for (const texture of this.textures) texture.decRef();
        this.textures.clear();
    }

    private load<T extends JsonAsset | Texture2D>(path: string, type: new () => T): Promise<T> {
        return new Promise((resolve, reject) => resources.load(path, type, (error, value) => error ? reject(error) : resolve(value)));
    }
}
