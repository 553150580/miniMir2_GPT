// 来源城镇使用原生矩形坐标；本模块不依赖Creator，也不改变个人地图的菱形投影。
export interface SourcePoint { readonly x: number; readonly y: number; }
export type SourceRect = readonly [number, number, number, number];
export interface SourceFrame {
    readonly png: string | null; readonly width: number; readonly height: number;
    readonly offset_x: number; readonly offset_y: number; readonly declared_empty: boolean;
    readonly [key: string]: unknown;
}
export interface SourceChunkInfo {
    readonly id: string; readonly x: number; readonly y: number; readonly path: string;
    readonly cell_bounds: SourceRect; readonly draw_bounds: SourceRect | null; readonly sha256: string;
}
export type SourceFloor = readonly [number, number, number, string];
export type SourceObject = readonly [number, number, number, string, number, number, number, number, number, number];
export interface SourceMapChunk {
    readonly schema_version: 1; readonly id: string; readonly x: number; readonly y: number;
    readonly cell_bounds: SourceRect; readonly cells_base64: string;
    readonly floor: readonly SourceFloor[]; readonly objects: readonly SourceObject[];
}
export interface SourceMapManifest {
    readonly schema_version: 1; readonly id: string; readonly width: number; readonly height: number;
    readonly tile_width: 48; readonly tile_height: 32; readonly projection: 'source_rectangular';
    readonly soil_thickness_px: 0; readonly source_origin: SourcePoint; readonly chunk_size_cells: 16;
    readonly pixel_width: number; readonly pixel_height: number;
    readonly frames: Readonly<Record<string, SourceFrame>>; readonly chunks: readonly SourceChunkInfo[];
    readonly minimap?: { readonly package: string; readonly raw_frame: number; readonly frame: string };
    readonly collision: { readonly runtime_bits: string };
    readonly spatial_index: { readonly bucket_width_px: 768; readonly bucket_height_px: 512;
        readonly buckets: Readonly<Record<string, readonly string[]>> };
}
export interface SourceDraw {
    readonly owner: string; readonly x: number; readonly y: number; readonly layer: number;
    readonly key: string; readonly frame: SourceFrame; readonly bounds: SourceRect;
    readonly blend: 0 | 1; readonly door_id: number; readonly door_offset: number;
}
export class SourceMapValidationError extends Error {}
function fail(field: string): never { throw new SourceMapValidationError(`来源地图字段无效：${field}`); }
function record(value: unknown, field: string): Record<string, unknown> {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return fail(field);
    const proto = Object.getPrototypeOf(value);
    if (proto !== Object.prototype && proto !== null) return fail(field);
    return value as Record<string, unknown>;
}
function integer(value: unknown, field: string, min = 0, max = Number.MAX_SAFE_INTEGER): number {
    if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min || value > max) return fail(field);
    return value;
}
function text(value: unknown, field: string): string {
    if (typeof value !== 'string' || !value.length || /[\x00-\x1f]/.test(value)) return fail(field);
    return value;
}
function array(value: unknown, field: string): unknown[] { if (!Array.isArray(value)) return fail(field); return value; }
function sameRect(a: SourceRect | null, b: SourceRect | null): boolean {
    return a === null || b === null ? a === b : a.every((n, i) => n === b[i]);
}
function rect(value: unknown, field: string, cells = false): SourceRect {
    const a = array(value, field); if (a.length !== 4) return fail(field);
    const r = a.map((n, i) => integer(n, `${field}[${i}]`, cells ? 0 : -Number.MAX_SAFE_INTEGER));
    if (cells ? r[2] <= 0 || r[3] <= 0 : r[2] <= r[0] || r[3] <= r[1]) return fail(field);
    return Object.freeze(r) as unknown as SourceRect;
}
function assetPath(value: unknown, field: string): string {
    const result = text(value, field);
    if (/[\\:]/.test(result) || result.startsWith('/') || result.split('/').some(p => !p || p === '.' || p === '..')) return fail(field);
    return result;
}
function hash(value: unknown, field: string): string {
    if (typeof value !== 'string' || !/^[a-f0-9]{64}$/.test(value)) return fail(field);
    return value;
}
function hashes(row: Record<string, unknown>, field: string): void {
    for (const key of Object.keys(row)) if (key.endsWith('_sha256') || key === 'sha256') {
        if (row[key] !== null) hash(row[key], `${field}.${key}`);
    }
}
// ID不经过Number；JSON使用十进制字符串，内存调用也可显式传入BigInt。
export function sourceMapId(value: unknown): string {
    const result = typeof value === 'bigint' ? value.toString() : value;
    if (typeof result !== 'string' || !/^(0|[1-9][0-9]*)$/.test(result) || BigInt(result) > 18446744073709551615n) return fail('id');
    return result;
}
const BASE64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
// 不依赖浏览器atob或Node Buffer，拒绝非规范填充与被截断的原格/碰撞数据。
export function decodeSourceBase64(value: unknown, expectedBytes: number, field = 'base64'): Uint8Array {
    integer(expectedBytes, `${field}.bytes`);
    if (typeof value !== 'string' || value.length !== Math.ceil(expectedBytes / 3) * 4
        || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return fail(field);
    const result = new Uint8Array(expectedBytes); let next = 0;
    for (let i = 0; i < value.length; i += 4) {
        const a = BASE64.indexOf(value[i]), b = BASE64.indexOf(value[i + 1]);
        const c = value[i + 2] === '=' ? 0 : BASE64.indexOf(value[i + 2]);
        const d = value[i + 3] === '=' ? 0 : BASE64.indexOf(value[i + 3]);
        if ((value[i + 2] === '=' && (b & 15)) || (value[i + 3] === '=' && (c & 3))) return fail(field);
        for (const byte of [(a << 2) | (b >> 4), ((b & 15) << 4) | (c >> 2), ((c & 3) << 6) | d]) {
            if (next < expectedBytes) result[next++] = byte;
        }
    }
    const padding = expectedBytes % 3 === 1 ? '==' : expectedBytes % 3 === 2 ? '=' : '';
    if (padding ? !value.endsWith(padding) || (padding === '=' && value.endsWith('==')) : value.includes('=')) return fail(field);
    return result;
}
function bitset(value: unknown, count: number, field: string): Uint8Array {
    const bits = decodeSourceBase64(value, Math.ceil(count / 8), field);
    if (count % 8 && (bits[bits.length - 1] >>> (count % 8))) return fail(`${field}尾位`);
    return bits;
}
function intersects(a: SourceRect, b: SourceRect): boolean { return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]; }
function union(a: SourceRect | null, b: SourceRect | null): SourceRect | null {
    return !a ? b : !b ? a : [Math.min(a[0], b[0]), Math.min(a[1], b[1]), Math.max(a[2], b[2]), Math.max(a[3], b[3])];
}
function bucketKeys(bounds: SourceRect): string[] {
    const keys: string[] = [];
    for (let y = Math.floor(bounds[1] / 512); y < Math.ceil(bounds[3] / 512); y++)
        for (let x = Math.floor(bounds[0] / 768); x < Math.ceil(bounds[2] / 768); x++) keys.push(`${x},${y}`);
    return keys;
}
export function parseSourceMapManifest(value: unknown): SourceMapManifest {
    const row = record(value, 'manifest'); if (row.schema_version !== 1) return fail('schema_version');
    const id = sourceMapId(row.id), width = integer(row.width, 'width', 1), height = integer(row.height, 'height', 1);
    if (!Number.isSafeInteger(width * height * 12) || !Number.isSafeInteger(width * 48) || !Number.isSafeInteger(height * 32)) return fail('地图大小');
    const origin = record(row.source_origin, 'source_origin');
    if (row.tile_width !== 48 || row.tile_height !== 32 || row.projection !== 'source_rectangular' || row.soil_thickness_px !== 0
        || row.chunk_size_cells !== 16 || origin.x !== 0 || origin.y !== 0 || row.pixel_width !== width * 48 || row.pixel_height !== height * 32) return fail('原生几何');
    const frames: Record<string, SourceFrame> = Object.create(null);
    for (const [key, value] of Object.entries(record(row.frames, 'frames'))) {
        text(key, 'frame.key'); const frame = record(value, `frames.${key}`);
        if (typeof frame.declared_empty !== 'boolean') return fail(`frames.${key}.declared_empty`);
        const empty = frame.declared_empty;
        const png = frame.png == null && empty ? null : assetPath(frame.png, `frames.${key}.png`);
        if (png && !png.endsWith('.png')) return fail(`frames.${key}.png`);
        hashes(frame, `frames.${key}`);
        frames[key] = Object.freeze({ ...frame, png, declared_empty: empty,
            // 已核验WIL头使用16位宽高及有符号16位偏移；拒绝异常巨大尺寸扩大空间桶循环。
            width: integer(frame.width, 'frame.width', empty ? 0 : 1, 65535), height: integer(frame.height, 'frame.height', empty ? 0 : 1, 65535),
            offset_x: integer(frame.offset_x, 'frame.offset_x', -32768, 32767), offset_y: integer(frame.offset_y, 'frame.offset_y', -32768, 32767) });
    }
    let minimap: SourceMapManifest['minimap'];
    if (row.minimap !== undefined) {
        const mini = record(row.minimap, 'minimap'), name = text(mini.package, 'minimap.package');
        const index = integer(mini.raw_frame, 'minimap.raw_frame'), key = text(mini.frame, 'minimap.frame'), frame = frames[key];
        if (!/^[A-Za-z0-9_]+$/.test(name) || key !== `${name}:${index}` || !frame || frame.declared_empty
            || frame.package !== name || frame.raw_frame !== index) return fail('minimap来源帧身份');
        minimap = Object.freeze({ package: name, raw_frame: index, frame: key });
    }
    const chunks: SourceChunkInfo[] = [], byId = new Map<string, SourceChunkInfo>(), paths = new Set<string>();
    for (const value of array(row.chunks, 'chunks')) {
        const chunk = record(value, 'chunk'), x = integer(chunk.x, 'chunk.x'), y = integer(chunk.y, 'chunk.y');
        const chunkId = text(chunk.id, 'chunk.id'), bounds = rect(chunk.cell_bounds, 'chunk.cell_bounds', true);
        const expected: SourceRect = [x * 16, y * 16, Math.min(16, width - x * 16), Math.min(16, height - y * 16)];
        if (chunkId !== `${x}_${y}` || byId.has(chunkId) || !sameRect(bounds, expected) || expected[2] <= 0 || expected[3] <= 0) return fail('chunk格范围');
        const item = Object.freeze({ id: chunkId, x, y, cell_bounds: bounds, path: assetPath(chunk.path, 'chunk.path'),
            draw_bounds: chunk.draw_bounds === null ? null : rect(chunk.draw_bounds, 'chunk.draw_bounds'), sha256: hash(chunk.sha256, 'chunk.sha256') });
        if (paths.has(item.path)) return fail('重复chunk路径'); paths.add(item.path);
        chunks.push(item); byId.set(chunkId, item);
    }
    if (chunks.length !== Math.ceil(width / 16) * Math.ceil(height / 16)) return fail('chunks未覆盖完整原图');
    chunks.sort((a, b) => a.y - b.y || a.x - b.x);
    const spatial = record(row.spatial_index, 'spatial_index'), buckets: Record<string, readonly string[]> = Object.create(null);
    if (spatial.bucket_width_px !== 768 || spatial.bucket_height_px !== 512) return fail('spatial_index桶大小');
    for (const [key, value] of Object.entries(record(spatial.buckets, 'spatial_index.buckets'))) {
        if (!/^(0|-?[1-9]\d*),(0|-?[1-9]\d*)$/.test(key)) return fail('bucket坐标');
        const [x, y] = key.split(',').map(Number); integer(x, 'bucket.x', -Number.MAX_SAFE_INTEGER); integer(y, 'bucket.y', -Number.MAX_SAFE_INTEGER);
        const bounds: SourceRect = [x * 768, y * 512, (x + 1) * 768, (y + 1) * 512], ids: string[] = [];
        if (!bounds.every(Number.isSafeInteger)) return fail('bucket像素范围');
        for (const v of array(value, `bucket.${key}`)) {
            const id = text(v, 'bucket.owner'), chunk = byId.get(id);
            if (!chunk || ids.includes(id) || !chunk.draw_bounds || !intersects(bounds, chunk.draw_bounds)) return fail('bucket.owner');
            ids.push(id);
        }
        if (!ids.length) return fail('空bucket'); buckets[key] = Object.freeze(ids);
    }
    const collision = record(row.collision, 'collision'), count = width * height, bits = bitset(collision.runtime_bits, count, 'collision.runtime_bits');
    if (collision.bit_count !== undefined && collision.bit_count !== count) return fail('collision.bit_count');
    if (collision.encoding !== undefined && collision.encoding !== 'row_major_lsb0_blocked_bitset_base64') return fail('collision.encoding');
    hashes(collision, 'collision');
    const checkCount = (name: string, data: Uint8Array): void => {
        if (collision[name] === undefined) return;
        let count = 0; for (const byte of data) { let bits = byte; while (bits) { bits &= bits - 1; count++; } }
        if (collision[name] !== count) return fail(`collision.${name}`);
    };
    checkCount('runtime_count', bits);
    if (collision.static_bits !== undefined || collision.closed_door_bits !== undefined) {
        const staticBits = bitset(collision.static_bits, count, 'collision.static_bits');
        const doors = bitset(collision.closed_door_bits, count, 'collision.closed_door_bits');
        checkCount('static_count', staticBits); checkCount('closed_door_count', doors);
        if (bits.some((n, i) => n !== (staticBits[i] | doors[i]))) return fail('collision.runtime必须为static OR closed_door');
    }
    return Object.freeze({ schema_version: 1, id, width, height, tile_width: 48, tile_height: 32, projection: 'source_rectangular',
        soil_thickness_px: 0, source_origin: Object.freeze({ x: 0, y: 0 }), chunk_size_cells: 16, pixel_width: width * 48, pixel_height: height * 32,
        frames: Object.freeze(frames), chunks: Object.freeze(chunks), ...(minimap ? { minimap } : {}), collision: Object.freeze({ runtime_bits: collision.runtime_bits as string }),
        spatial_index: Object.freeze({ bucket_width_px: 768, bucket_height_px: 512, buckets: Object.freeze(buckets) }) });
}
export function sourceAnimationKey(key: string, count: number, tick: number, nowMs: number): string {
    integer(count, 'animation.count', 0, 127); integer(tick, 'animation.tick', 0, 255);
    if (!Number.isFinite(nowMs) || nowMs < 0 || nowMs > Number.MAX_SAFE_INTEGER) return fail('animation.now_ms');
    if (count <= 1) return key;
    const offset = Math.floor((Math.floor(nowMs / 100) % (count * (1 + tick))) / (1 + tick));
    return offsetFrameKey(key, offset);
}
function offsetFrameKey(key: string, offset: number): string {
    const match = /^(.*:)?(0|[1-9]\d*)$/.exec(key); if (!match) return fail('animation.key');
    return `${match[1] ?? ''}${BigInt(match[2]) + BigInt(offset)}`;
}
function frameRect(x: number, y: number, layer: number, frame: SourceFrame, offsetMode = 0): SourceRect | null {
    if (frame.declared_empty) return null;
    const left = x * 48 + (offsetMode ? frame.offset_x : 0), top = (layer <= 2 ? y * 32 : (y + 1) * 32 - frame.height) + (offsetMode ? frame.offset_y : 0);
    const bounds: SourceRect = [left, top, left + frame.width, top + frame.height];
    if (!bounds.every(Number.isSafeInteger)) return fail('绘制像素范围'); return bounds;
}
// 地板原生顺序为y/x/层；立体物件逐行绘制，再由桥接层插入该行实体。
export function compareSourceDraws(a: SourceDraw, b: SourceDraw): number {
    return Number(a.layer >= 3) - Number(b.layer >= 3) || a.y - b.y || a.x - b.x || a.layer - b.layer || a.owner.localeCompare(b.owner);
}
export class SourceMapModel {
    readonly manifest: SourceMapManifest;
    private readonly chunks = new Map<string, SourceChunkInfo>();
    private readonly indexedBuckets = new Map<string, Set<string>>();
    private readonly bucketNames: readonly string[];
    private readonly validatedChunks = new WeakSet<SourceMapChunk>();
    private readonly collision: Uint8Array;
    constructor(value: unknown) {
        this.manifest = parseSourceMapManifest(value);
        this.bucketNames = Object.freeze(Object.keys(this.manifest.spatial_index.buckets));
        this.collision = bitset(this.manifest.collision.runtime_bits, this.manifest.width * this.manifest.height, 'collision.runtime_bits');
        for (const chunk of this.manifest.chunks) { this.chunks.set(chunk.id, chunk); this.indexedBuckets.set(chunk.id, new Set()); }
        for (const [key, ids] of Object.entries(this.manifest.spatial_index.buckets)) for (const id of ids) this.indexedBuckets.get(id)!.add(key);
    }
    chunkInfo(id: string): SourceChunkInfo { const chunk = this.chunks.get(id); if (!chunk) return fail('未知chunk'); return chunk; }
    project(x: number, y: number): SourcePoint {
        if (![x * 48, y * 32].every(Number.isFinite)) return fail('投影坐标'); return { x: x * 48, y: y * 32 };
    }
    unproject(point: SourcePoint): SourcePoint {
        if (![point.x, point.y].every(Number.isFinite)) return fail('像素坐标'); return { x: point.x / 48, y: point.y / 32 };
    }
    cellAt(point: SourcePoint): SourcePoint | null {
        if (![point.x, point.y].every(Number.isFinite)) return null;
        const x = Math.floor(point.x / 48), y = Math.floor(point.y / 32); return this.contains(x, y) ? { x, y } : null;
    }
    contains(x: number, y: number): boolean { return Number.isSafeInteger(x) && Number.isSafeInteger(y) && x >= 0 && y >= 0 && x < this.manifest.width && y < this.manifest.height; }
    isWalkable(x: number, y: number): boolean {
        if (!this.contains(x, y)) return false; const index = y * this.manifest.width + x; return !(this.collision[Math.floor(index / 8)] & (1 << (index % 8)));
    }
    queryChunks(viewport: SourceRect): readonly SourceChunkInfo[] {
        if (!viewport.every(Number.isFinite) || viewport[2] < viewport[0] || viewport[3] < viewport[1]) return fail('viewport');
        if (viewport[2] === viewport[0] || viewport[3] === viewport[1]) return [];
        const ids = new Set<string>(), buckets = this.manifest.spatial_index.buckets, existing = this.bucketNames;
        const x0 = Math.floor(viewport[0] / 768), x1 = Math.ceil(viewport[2] / 768), y0 = Math.floor(viewport[1] / 512), y1 = Math.ceil(viewport[3] / 512);
        const add = (key: string): void => { for (const id of buckets[key] ?? []) ids.add(id); };
        // 常规视口只查询覆盖的桶；超大视口改扫已存在桶，避免构造无界空桶循环。
        if ((x1 - x0) * (y1 - y0) <= existing.length) {
            for (let y = y0; y < y1; y++) for (let x = x0; x < x1; x++) add(`${x},${y}`);
        } else for (const key of existing) {
            const [x, y] = key.split(',').map(Number);
            if (intersects(viewport, [x * 768, y * 512, (x + 1) * 768, (y + 1) * 512])) add(key);
        }
        return Array.from(ids).map(id => this.chunkInfo(id)).filter(chunk => chunk.draw_bounds && intersects(viewport, chunk.draw_bounds)).sort((a, b) => a.y - b.y || a.x - b.x);
    }
    parseChunk(value: unknown, expectedId?: string): SourceMapChunk {
        const row = record(value, 'chunk'), info = this.chunkInfo(text(row.id, 'chunk.id'));
        if (expectedId !== undefined && info.id !== expectedId) return fail('chunk请求身份');
        if (row.schema_version !== 1 || row.x !== info.x || row.y !== info.y || !sameRect(rect(row.cell_bounds, 'chunk.cell_bounds', true), info.cell_bounds)) return fail('chunk身份/范围');
        decodeSourceBase64(row.cells_base64, info.cell_bounds[2] * info.cell_bounds[3] * 12, 'chunk.cells_base64');
        const seen = new Set<string>(), requiredBuckets = new Set<string>(); let drawBounds: SourceRect | null = null;
        const own = (a: unknown[], length: number): void => {
            if (a.length !== length || !this.contains(a[0] as number, a[1] as number)) return fail('draw元组');
            const x = a[0] as number, y = a[1] as number, b = info.cell_bounds;
            if (x < b[0] || y < b[1] || x >= b[0] + b[2] || y >= b[1] + b[3]) return fail('draw不属于owner chunk');
            const key = `${x},${y},${a[2]}`; if (seen.has(key)) return fail('重复draw'); seen.add(key);
            text(a[3], 'draw.key'); if (!this.manifest.frames[a[3] as string]) return fail('缺少frame');
        };
        const addRect = (bounds: SourceRect | null): void => {
            drawBounds = union(drawBounds, bounds); if (bounds) for (const key of bucketKeys(bounds)) requiredBuckets.add(key);
        };
        const floor = array(row.floor, 'floor').map(value => {
            const a = array(value, 'floor元组'); own(a, 4); integer(a[2], 'floor.layer', 0, 2);
            addRect(frameRect(a[0] as number, a[1] as number, a[2] as number, this.manifest.frames[a[3] as string]));
            return Object.freeze(a.slice()) as unknown as SourceFloor;
        });
        const objects = array(row.objects, 'objects').map(value => {
            const a = array(value, 'object元组'); own(a, 10); integer(a[2], 'object.layer', 3, 4);
            integer(a[4], 'object.count', 0, 127); integer(a[5], 'object.tick', 0, 255); integer(a[6], 'object.blend', 0, 1);
            integer(a[7], 'object.offset_mode', 0, 1); integer(a[8], 'object.door_id', 0, 127); integer(a[9], 'object.door_offset', 0, 255);
            for (let i = 0; i < Math.max(a[4] as number, 1); i++) {
                const key = i ? offsetFrameKey(a[3] as string, i) : a[3] as string, frame = this.manifest.frames[key];
                if (!frame) return fail('缺少动画frame'); addRect(frameRect(a[0] as number, a[1] as number, a[2] as number, frame, a[7] as number));
            }
            return Object.freeze(a.slice()) as unknown as SourceObject;
        });
        const actual = this.indexedBuckets.get(info.id)!;
        if (!sameRect(drawBounds, info.draw_bounds) || actual.size !== requiredBuckets.size || Array.from(requiredBuckets).some(key => !actual.has(key))) return fail('chunk绘制范围/空间桶与实例不一致');
        const parsed: SourceMapChunk = Object.freeze({ schema_version: 1, id: info.id, x: info.x, y: info.y, cell_bounds: info.cell_bounds, cells_base64: row.cells_base64 as string,
            floor: Object.freeze(floor), objects: Object.freeze(objects) });
        this.validatedChunks.add(parsed); return parsed;
    }
    draws(chunk: SourceMapChunk, nowMs: number, viewport?: SourceRect): readonly SourceDraw[] {
        if (!this.validatedChunks.has(chunk)) return fail('绘制块未由当前地图验证');
        if (!Number.isFinite(nowMs) || nowMs < 0 || nowMs > Number.MAX_SAFE_INTEGER) return fail('animation.now_ms');
        if (viewport && (!viewport.every(Number.isFinite) || viewport[2] < viewport[0] || viewport[3] < viewport[1])) return fail('viewport');
        if (viewport && (viewport[2] === viewport[0] || viewport[3] === viewport[1])) return [];
        const draws: SourceDraw[] = [];
        for (const a of chunk.floor) this.pushDraw(draws, chunk.id, a[0], a[1], a[2], a[3], 0, 0, 0, 0, viewport);
        for (const a of chunk.objects) this.pushDraw(draws, chunk.id, a[0], a[1], a[2], sourceAnimationKey(a[3], a[4], a[5], nowMs), a[6] as 0 | 1, a[7], a[8], a[9], viewport);
        return draws.sort(compareSourceDraws);
    }
    private pushDraw(draws: SourceDraw[], owner: string, x: number, y: number, layer: number, key: string, blend: 0 | 1, offset: number, doorId: number, doorOffset: number, viewport?: SourceRect): void {
        const frame = this.manifest.frames[key]; if (!frame) return fail('缺少绘制frame');
        const bounds = frameRect(x, y, layer, frame, offset);
        if (bounds && (!viewport || intersects(bounds, viewport))) draws.push({ owner, x, y, layer, key, frame, bounds, blend, door_id: doorId, door_offset: doorOffset });
    }
}
export function parseSourceMapChunk(value: unknown, model: SourceMapModel, expectedId?: string): SourceMapChunk { return model.parseChunk(value, expectedId); }
