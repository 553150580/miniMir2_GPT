import { instantiate, isValid, Node, Sprite, UITransform } from 'cc';
import { CombatEntity } from './CombatStatusModel';
import { StatusPlayback } from './StatusPlayback';
import { PotionInteger } from './PotionModel';
import { WorldAssets } from './WorldAssets';

// 附体特效只实例化保存Effect模板，位置跟实体脚点，不参与HUD或导航状态。
export class StatusEffects {
    private readonly playback = new StatusPlayback();
    private readonly nodes = new Map<string, { node: Node; sprite: Sprite; key: string }>();
    constructor(private readonly assets: WorldAssets, private readonly scene: PotionInteger, private readonly container: (handle: number, life: string) => Node | null) {
        this.playback.bind(scene);
    }
    observe(sequence: PotionInteger, entities: readonly CombatEntity[], now: number): void { this.playback.observe(this.scene, sequence, entities, now); }
    update(now: number): void {
        const active = new Set<string>(), resource = this.assets.manifest.statuses?.['31'];
        for (const pose of this.playback.poses(now, resource?.struck?.duration_ms ?? 0)) {
            const parent = this.container(pose.entityHandle, pose.life), part = resource?.[pose.part];
            if (!parent || !part || !part.frames.length) continue;
            const key = part.frames[Math.floor(Math.max(0, pose.elapsed) * part.frames.length / part.duration_ms) % part.frames.length];
            const definition = this.assets.manifest.frames[key], frame = this.assets.frames.get(key);
            if (!definition || definition.declared_empty || !frame) continue;
            let visual = this.nodes.get(pose.key);
            if (!visual) {
                const node = instantiate(this.assets.effectPrefab); node.parent = parent;
                visual = { node, sprite: node.getComponent(Sprite)!, key: '' }; this.nodes.set(pose.key, visual);
            }
            active.add(pose.key);
            if (visual.key === key) continue;
            visual.sprite.spriteFrame = frame; visual.sprite.type = Sprite.Type.SIMPLE; visual.sprite.trim = false; visual.sprite.sizeMode = Sprite.SizeMode.CUSTOM;
            visual.node.getComponent(UITransform)!.setAnchorPoint(0, 1); visual.node.getComponent(UITransform)!.setContentSize(definition.width, definition.height);
            visual.node.setPosition(definition.offset_x - 24, 16 - definition.offset_y); visual.key = key;
        }
        for (const [key, visual] of Array.from(this.nodes)) if (!active.has(key)) { this.removeVisual(visual); this.nodes.delete(key); }
    }
    private removeVisual(visual: { node: Node; sprite: Sprite }): void {
        if (isValid(visual.sprite)) { visual.sprite.spriteFrame = null; visual.sprite.customMaterial = null; }
        if (isValid(visual.node, true)) { visual.node.active = false; visual.node.parent = null; visual.node.destroy(); }
    }
    dispose(): void { this.playback.clear(); for (const visual of Array.from(this.nodes.values())) this.removeVisual(visual); this.nodes.clear(); }
}
