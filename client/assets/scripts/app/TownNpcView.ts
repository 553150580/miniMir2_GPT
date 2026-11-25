import { instantiate, isValid, Label, Node, Sprite, UITransform } from 'cc';
import { profileHeadTop } from './EntityHeadAnchor';
import { GridProjection } from './GridProjection';
import { townNpcAppearance } from './TownMapModel';
import type { TownNpc } from './TownMapModel';
import type { WorldAssets } from './WorldAssets';
import { bindTownTouchHit } from './TownTouchHit';

interface NpcLayer {
    readonly sprite: Sprite;
    readonly frames: readonly string[];
    readonly frameMs: number;
    current?: string;
    hitMask?: Uint8Array;
}

export class TownNpcView {
    readonly node: Node;
    private readonly layers: NpcLayer[] = [];
    private started?: number;

    constructor(private readonly assets: WorldAssets, entities: Node, projection: GridProjection, readonly npc: TownNpc, select: (npc: TownNpc) => void) {
        const node = this.node = instantiate(assets.prefab);
        const body = node.getChildByName('Body')!, label = node.getChildByName('Name')!;
        const appearance = townNpcAppearance(assets.manifest, npc);
        node.name = `npc_${npc.id}`;
        node.getChildByName('HP')!.active = false;
        label.getComponent(Label)!.string = npc.name;
        label.setPosition(label.position.x, profileHeadTop(appearance.profile, assets.manifest.frames) + 14, label.position.z);
        const selectBody = (event: { propagationStopped: boolean }) => { event.propagationStopped = true; select(npc); };
        for (const [index, sequence] of appearance.parts.entries()) {
            const part = index === 0 ? body : instantiate(body);
            if (index > 0) { part.parent = node; part.setSiblingIndex(index); }
            const sprite = part.getComponent(Sprite)!;
            sprite.trim = false; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
            const layer: NpcLayer = { ...sequence, sprite };
            this.layers.push(layer);
            this.paint(layer, sequence.frames[0]);
            this.bindVisibleHit(layer);
            part.on(Node.EventType.TOUCH_END, selectBody);
        }
        const foot = projection.project(npc.position.x, npc.position.y);
        node.setPosition(foot.x, foot.y); node.parent = entities;
    }

    // 仅更新原站立序列的表现，脚点、遮挡格和交互节点保持固定。
    update(now: number): void {
        this.started ??= now;
        for (const layer of this.layers) {
            if (!layer.frameMs) continue;
            const elapsed = Math.max(0, now - this.started);
            this.paint(layer, layer.frames[Math.floor(elapsed / layer.frameMs) % layer.frames.length]);
        }
    }

    // 先保留 Creator 的相机、视口和父遮罩判定，再过滤当前站立帧的透明像素。
    // 在触摸开始前完成过滤，空地仍由原地图视口认领整次手势。
    private bindVisibleHit(layer: NpcLayer): void {
        if (!layer.frames.some(key => this.assets.manifest.townNpcHitMasks?.[key])) return;
        bindTownTouchHit(layer.sprite.node, (x, y, width) => {
            if (!layer.hitMask) return true;
            const pixel = Math.floor(y) * width + Math.floor(x);
            return (layer.hitMask[pixel >> 3] & (1 << (pixel & 7))) !== 0;
        });
    }

    private paint(layer: NpcLayer, key: string): void {
        if (layer.current === key) return;
        const frame = this.assets.frames.get(key), definition = this.assets.manifest.frames[key];
        if (!frame) throw new Error(`城镇NPC站立帧尚未准备：${key}`);
        layer.sprite.spriteFrame = frame;
        const part = layer.sprite.node, transform = part.getComponent(UITransform)!;
        transform.setAnchorPoint(0, 1); transform.setContentSize(definition.width, definition.height);
        part.setPosition(definition.offset_x - 24, 16 - definition.offset_y);
        layer.hitMask = this.assets.manifest.townNpcHitMasks?.[key];
        layer.current = key;
    }

    dispose(): void {
        for (const { sprite } of this.layers) if (isValid(sprite)) sprite.spriteFrame = null;
        if (isValid(this.node, true)) { this.node.active = false; this.node.parent = null; this.node.destroy(); }
    }
}
