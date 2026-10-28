import { instantiate, isValid, Node, Sprite, UITransform, Vec3 } from 'cc';
import { AttackEvent, CastEvent, CastPlayback, CastStepEvent, CastStepPlayback, effectPoses, ImpactEvent, MeleePlayback, MeleeSample } from './CastPlayback';
import { effectFrameKeys, WorldAssets } from './WorldAssets';
import { isGroundSkill } from './SkillModel';
import type { GridProjection } from './GridProjection';
import { beamGeometry } from './BeamGeometry';
import type { DisplacementEvent } from './EntityDisplacement';

interface EffectNode { node: Node; sprite: Sprite; frame: string; }

// 只实例化已保存的游戏特效预制体；所有帧在入场前持有，退场统一释放。
export class CombatEffects {
    private readonly playback = new CastPlayback();
    private readonly melee = new MeleePlayback();
    private readonly steps = new CastStepPlayback();
    private readonly nodes = new Map<string, EffectNode>();
    private readonly teleports = new Map<string, { event: DisplacementEvent; started: number; owner: boolean }>();

    constructor(private readonly assets: WorldAssets, private readonly parent: Node, sceneEpoch: { toString(): string },
        private readonly grid: (x: number, y: number) => Vec3, private readonly position: (handle: number) => Vec3 | null,
        private readonly life: (handle: number) => string | null, private readonly attackSample: (handle: number) => MeleeSample | null = () => null,
        private readonly projection?: GridProjection) {
        this.playback.bind(sceneEpoch); this.melee.bind(sceneEpoch); this.steps.bind(sceneEpoch);
    }

    accept(event: CastEvent, now: number): void {
        const definition = this.assets.gameplay.skills?.find((skill: any) => skill.id === event.skillId);
        // 当前世界只预载本职业；缺少对应的真实素材时不创建替代图形。
        if (!this.assets.manifest.effects?.[String(event.skillId)]) return;
        if (event.casterLife != null && this.life(event.casterHandle) !== event.casterLife.toString()) return;
        const frames = effectFrameKeys(this.assets.manifest.effects[String(event.skillId)]);
        if (frames.some(key => !this.assets.frames.has(key))) return;
        this.playback.accept(event, definition, now);
    }
    displacement(event: DisplacementEvent, now: number, owner: boolean): void {
        if (event.kind !== 4 || !this.assets.manifest.teleport) return;
        const key = `teleport:${event.entityHandle}:${event.entityLife}:${event.movementRevision}`;
        this.teleports.set(key, { event, started: now, owner });
        if (this.teleports.size > 64) this.teleports.delete(this.teleports.keys().next().value!);
    }
    attack(event: AttackEvent, now: number): void {
        const resource = this.assets.manifest.effects?.[String(event.skillId)];
        if (!resource?.attack || event.casterLife == null || this.life(event.casterHandle) !== event.casterLife.toString()
            || effectFrameKeys(resource).some(key => !this.assets.frames.has(key))) return;
        this.melee.accept(event, now);
    }
    impact(event: ImpactEvent, now: number): void {
        if (this.life(event.targetHandle) !== event.targetLife.toString()) return;
        this.playback.impact(event, now);
    }
    step(event: CastStepEvent, now: number): void {
        const resource = this.assets.manifest.effects?.[String(event.skillId)], map = this.assets.map;
        if (!resource?.step || this.life(event.casterHandle) !== event.casterLife.toString()
            || !event.grid || event.grid.x < 0 || event.grid.y < 0 || event.grid.x >= map.width || event.grid.y >= map.height
            || resource.step.frames.some((key: string) => !this.assets.frames.has(key))) return;
        this.steps.accept(event, this.assets.gameplay.skills?.find((skill: any) => skill.id === event.skillId), now);
    }

    update(now: number): void {
        this.playback.expire(now, id => this.assets.manifest.effects[String(id)]?.hit?.duration_ms ?? 0,
            id => this.assets.manifest.effects[String(id)]?.cast?.duration_ms ?? 0,
            id => this.assets.manifest.effects[String(id)]?.beam?.duration_ms ?? 0);
        const active = new Set<string>();
        for (const [id, visual] of this.teleports) {
            const elapsed = Math.max(0, now - visual.started), resource = this.assets.manifest.teleport;
            if (elapsed >= Math.max(resource.out.duration_ms, resource.in.duration_ms)) { this.teleports.delete(id); continue; }
            for (const kind of visual.owner ? ['in'] : ['out', 'in']) {
                const part = resource[kind]; if (elapsed >= part.duration_ms) continue;
                if (kind === 'in' && this.life(visual.event.entityHandle) !== visual.event.entityLife.toString()) continue;
                const end = visual.event.path[0];
                const position = kind === 'out' ? this.grid(visual.event.origin.x, visual.event.origin.y)
                    : this.position(visual.event.entityHandle) ?? this.grid(end.x, end.y);
                const key = part.frames[Math.min(part.frames.length - 1, Math.floor(elapsed * part.frames.length / part.duration_ms))];
                active.add(`${id}:${kind}`); this.draw(`${id}:${kind}`, key, position);
            }
        }
        for (const cast of this.playback.casts) {
            const resource = this.assets.manifest.effects[String(cast.event.skillId)];
            for (const pose of effectPoses(cast, now, resource.cast?.duration_ms ?? 0, resource.beam?.duration_ms ?? 0)) {
                const part = resource[pose.kind];
                if (!part || (pose.kind === 'hit' && pose.elapsed >= part.duration_ms)) continue;
                const direction = pose.kind === 'projectile' ? this.projection?.visualDirectionBetween(cast.event.origin, cast.event.targetGrid, 16, cast.event.direction) ?? pose.direction
                    : pose.kind === 'beam' ? this.projection?.visualDirection(pose.direction) ?? pose.direction : pose.direction;
                const frames: string[] = pose.kind === 'projectile' || pose.kind === 'beam' ? part.directions[direction] : part.frames;
                const index = pose.kind === 'projectile' ? Math.floor(pose.elapsed / part.frame_ms) % frames.length
                    : Math.min(frames.length - 1, Math.floor(pose.elapsed * frames.length / part.duration_ms));
                const key = frames[index], definition = this.assets.manifest.frames[key], frame = this.assets.frames.get(key);
                if (!frame || definition.declared_empty) continue;
                const origin = this.grid(cast.event.origin.x, cast.event.origin.y), target = this.grid(cast.event.targetGrid.x, cast.event.targetGrid.y);
                let point: Vec3 | null;
                if (pose.kind === 'cast' || pose.kind === 'beam') point = cast.event.casterLife != null && this.life(cast.event.casterHandle) !== cast.event.casterLife.toString()
                    ? null : resource[`${pose.kind}_anchor`] === 'origin' ? origin : this.position(cast.event.casterHandle) ?? origin;
                else if (pose.kind === 'hit') point = isGroundSkill(cast.definition) ? target
                    : cast.event.targetLife != null && this.life(cast.event.targetHandle) !== cast.event.targetLife.toString()
                        ? null : this.position(cast.event.targetHandle) ?? target;
                else point = new Vec3(origin.x + (target.x - origin.x) * pose.fraction, origin.y + (target.y - origin.y) * pose.fraction);
                if (point) {
                    active.add(pose.key);
                    if (pose.kind === 'beam' && this.projection) this.drawBeam(pose.key, key, point, cast.event, resource);
                    else this.draw(pose.key, key, point);
                }
            }
        }
        for (const step of this.steps.active(now, this.assets.manifest.effects['9']?.step?.duration_ms ?? 0)) {
            const part = this.assets.manifest.effects[String(step.event.skillId)]?.step;
            if (!part) continue;
            const index = Math.min(part.frames.length - 1, Math.floor((now - step.started) * part.frames.length / part.duration_ms));
            const id = `step:${step.event.casterHandle}:${step.event.casterLife}:${step.event.castId}:${step.event.stepIndex}`;
            active.add(id); this.draw(id, part.frames[index], this.grid(step.event.grid.x, step.event.grid.y));
        }
        for (const pose of this.melee.poses(now, this.attackSample)) {
            if (this.life(pose.event.casterHandle) !== pose.event.casterLife?.toString()) continue;
            const part = this.assets.manifest.effects[String(pose.event.skillId)]?.attack;
            const directions = part?.levels ? part.levels[pose.event.skillLevel!]?.directions : part?.directions;
            const key = directions?.[pose.direction]?.[pose.frame], point = this.position(pose.event.casterHandle);
            if (!key || !point) continue;
            active.add(pose.key); this.draw(pose.key, key, point);
        }
        for (const [key, visual] of this.nodes) if (!active.has(key)) { visual.node.destroy(); this.nodes.delete(key); }
    }

    private drawBeam(id: string, key: string, origin: Vec3, event: CastEvent, resource: any): void {
        const definition = this.assets.gameplay.skills.find((row: any) => row.id === event.skillId);
        const frames = resource.beam.directions[this.projection!.visualDirection(event.direction)].map((name: string) => this.assets.manifest.frames[name]);
        const layout = beamGeometry(this.projection!, event.direction, definition.line_cells, resource.beam_native_direction_vectors, frames);
        const frame = this.assets.manifest.frames[key], texture = this.assets.frames.get(key); if (!texture) return;
        let visual = this.nodes.get(id);
        if (!visual) {
            const node = instantiate(this.assets.beamPrefab); node.parent = this.parent;
            visual = { node, sprite: node.getChildByName('Sprite')!.getComponent(Sprite)!, frame: '' }; this.nodes.set(id, visual);
        }
        visual.node.setPosition(origin); visual.node.angle = layout.angleDegrees;
        visual.node.getComponent(UITransform)!.setContentSize(layout.length, layout.height);
        const position = layout.position(frame); visual.sprite.node.setPosition(position.x, position.y); visual.sprite.node.angle = layout.spriteAngleDegrees;
        if (visual.frame === key) return;
        visual.sprite.spriteFrame = texture; visual.sprite.trim = false; visual.sprite.sizeMode = Sprite.SizeMode.CUSTOM;
        visual.sprite.node.getComponent(UITransform)!.setContentSize(frame.width, frame.height); visual.frame = key;
    }

    private draw(id: string, key: string, point: Vec3): void {
        const definition = this.assets.manifest.frames[key], frame = this.assets.frames.get(key);
        if (!definition || !frame || definition.declared_empty) return;
        let visual = this.nodes.get(id);
        if (!visual) {
            const node = instantiate(this.assets.effectPrefab); node.parent = this.parent;
            visual = { node, sprite: node.getComponent(Sprite)!, frame: '' }; this.nodes.set(id, visual);
        }
        visual.node.setPosition(point.x + definition.offset_x - 24, point.y + 16 - definition.offset_y);
        if (visual.frame === key) return;
        visual.sprite.spriteFrame = frame; visual.sprite.type = Sprite.Type.SIMPLE;
        visual.sprite.trim = false; visual.sprite.sizeMode = Sprite.SizeMode.CUSTOM;
        const transform = visual.node.getComponent(UITransform)!;
        transform.setAnchorPoint(0, 1); transform.setContentSize(definition.width, definition.height);
        visual.frame = key;
    }

    dispose(): void {
        this.playback.clear(); this.melee.clear(); this.steps.clear(); this.teleports.clear();
        for (const visual of this.nodes.values()) if (isValid(visual.node, true)) visual.node.destroy();
        this.nodes.clear();
    }
}
