import { instantiate, isValid, Label, Node, Sprite, UITransform } from 'cc';
import { profileHeadTop } from './EntityHeadAnchor';
import { GridProjection } from './GridProjection';
import { TownNpc, townNpcFrames } from './TownMapModel';
import type { WorldAssets } from './WorldAssets';

export class TownNpcView {
    readonly node: Node;
    private readonly sprites: Sprite[] = [];
    constructor(assets: WorldAssets, entities: Node, projection: GridProjection, readonly npc: TownNpc, select: (npc: TownNpc) => void) {
        const node = this.node = instantiate(assets.prefab);
        const body = node.getChildByName('Body')!, label = node.getChildByName('Name')!;
        node.name = `npc_${npc.id}`;
        node.getChildByName('HP')!.active = false;
        label.getComponent(Label)!.string = npc.name;
        label.setPosition(label.position.x, profileHeadTop(assets.manifest.profiles[npc.profile], assets.manifest.frames) + 14, label.position.z);
        const keys = townNpcFrames(assets.manifest, npc);
        for (const [index, key] of keys.entries()) {
            const part = index === 0 ? body : instantiate(body);
            const frame = assets.frames.get(key), definition = assets.manifest.frames[key];
            if (!frame) throw new Error(`城镇NPC站立帧尚未准备：${key}`);
            if (index > 0) { part.parent = node; part.setSiblingIndex(index); }
            const sprite = part.getComponent(Sprite)!;
            this.sprites.push(sprite); sprite.spriteFrame = frame; sprite.trim = false; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
            const transform = part.getComponent(UITransform)!;
            transform.setAnchorPoint(0, 1); transform.setContentSize(definition.width, definition.height);
            part.setPosition(definition.offset_x - 24, 16 - definition.offset_y);
        }
        const foot = projection.project(npc.position.x, npc.position.y);
        node.setPosition(foot.x, foot.y); node.parent = entities;
        node.on(Node.EventType.TOUCH_END, (event: { propagationStopped: boolean }) => { event.propagationStopped = true; select(npc); });
    }
    dispose(): void {
        for (const sprite of this.sprites) if (isValid(sprite)) sprite.spriteFrame = null;
        if (isValid(this.node, true)) { this.node.active = false; this.node.parent = null; this.node.destroy(); }
    }
}
