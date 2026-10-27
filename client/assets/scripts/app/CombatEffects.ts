import { instantiate, isValid, Node, Sprite, UITransform, Vec3 } from 'cc';
import { CastEvent, CastPlayback, effectPoses, ImpactEvent } from './CastPlayback';
import { WorldAssets } from './WorldAssets';
import { isGroundSkill } from './SkillModel';

interface EffectNode { node: Node; sprite: Sprite; frame: string; }

// 只实例化已保存的游戏特效预制体；所有帧在入场前持有，退场统一释放。
export class CombatEffects {
    private readonly playback = new CastPlayback();
    private readonly nodes = new Map<string, EffectNode>();

    constructor(private readonly assets: WorldAssets, private readonly parent: Node, sceneEpoch: { toString(): string },
        private readonly grid: (x: number, y: number) => Vec3, private readonly position: (handle: number) => Vec3 | null,
        private readonly life: (handle: number) => string | null) {
        this.playback.bind(sceneEpoch);
    }

    accept(event: CastEvent, now: number): void {
        const definition = this.assets.gameplay.skills?.find((skill: any) => skill.id === event.skillId);
        // 当前世界只预载本职业；缺少对应的真实素材时不创建替代图形。
        if (!this.assets.manifest.effects?.[String(event.skillId)]) return;
        if (event.casterLife && this.life(event.casterHandle) !== event.casterLife.toString()) return;
        const frames = this.assets.manifest.effects[String(event.skillId)].cast.frames as string[];
        if (frames.some(key => !this.assets.frames.has(key))) return;
        this.playback.accept(event, definition, now);
    }
    impact(event: ImpactEvent, now: number): void {
        if (this.life(event.targetHandle) !== event.targetLife.toString()) return;
        this.playback.impact(event, now);
    }

    update(now: number): void {
        this.playback.expire(now, id => this.assets.manifest.effects[String(id)]?.hit?.duration_ms ?? 0,
            id => this.assets.manifest.effects[String(id)]?.cast?.duration_ms ?? 0);
        const active = new Set<string>();
        for (const cast of this.playback.casts) {
            const resource = this.assets.manifest.effects[String(cast.event.skillId)];
            for (const pose of effectPoses(cast, now, resource.cast.duration_ms)) {
                const part = resource[pose.kind];
                if (!part || (pose.kind === 'hit' && pose.elapsed >= part.duration_ms)) continue;
                const frames: string[] = pose.kind === 'projectile' ? part.directions[pose.direction] : part.frames;
                const index = pose.kind === 'projectile' ? Math.floor(pose.elapsed / part.frame_ms) % frames.length
                    : Math.min(frames.length - 1, Math.floor(pose.elapsed * frames.length / part.duration_ms));
                const key = frames[index], definition = this.assets.manifest.frames[key], frame = this.assets.frames.get(key);
                if (!frame || definition.declared_empty) continue;
                active.add(pose.key);
                let visual = this.nodes.get(pose.key);
                if (!visual) {
                    const node = instantiate(this.assets.effectPrefab); node.parent = this.parent;
                    visual = { node, sprite: node.getComponent(Sprite)!, frame: '' }; this.nodes.set(pose.key, visual);
                }
                const origin = this.grid(cast.event.origin.x, cast.event.origin.y), target = this.grid(cast.event.targetGrid.x, cast.event.targetGrid.y);
                let point: Vec3 | null;
                if (pose.kind === 'cast') point = cast.event.casterLife && this.life(cast.event.casterHandle) !== cast.event.casterLife.toString()
                    ? null : this.position(cast.event.casterHandle) ?? origin;
                else if (pose.kind === 'hit') point = isGroundSkill(cast.definition) ? target
                    : cast.event.targetLife && this.life(cast.event.targetHandle) !== cast.event.targetLife.toString()
                        ? null : this.position(cast.event.targetHandle) ?? target;
                else point = new Vec3(origin.x + (target.x - origin.x) * pose.fraction, origin.y + (target.y - origin.y) * pose.fraction);
                if (!point) { active.delete(pose.key); continue; }
                visual.node.setPosition(point.x + definition.offset_x - 24, point.y + 16 - definition.offset_y);
                if (visual.frame === key) continue;
                visual.sprite.spriteFrame = frame; visual.sprite.type = Sprite.Type.SIMPLE;
                visual.sprite.trim = false; visual.sprite.sizeMode = Sprite.SizeMode.CUSTOM;
                const transform = visual.node.getComponent(UITransform)!;
                transform.setAnchorPoint(0, 1); transform.setContentSize(definition.width, definition.height);
                visual.frame = key;
            }
        }
        for (const [key, visual] of this.nodes) if (!active.has(key)) { visual.node.destroy(); this.nodes.delete(key); }
    }

    dispose(): void {
        this.playback.clear();
        for (const visual of this.nodes.values()) if (isValid(visual.node, true)) visual.node.destroy();
        this.nodes.clear();
    }
}
