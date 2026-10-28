import { Color, EventTouch, game, Graphics, instantiate, isValid, Label, Node, ProgressBar, Sprite, UITransform, Vec2, Vec3 } from 'cc';
import { AnimationSample, EntityAnimation, movementDisplayDurationMs } from './EntityAnimation';
import { EntityMotion } from './EntityMotion';
import { WorldAssets } from './WorldAssets';
import { CombatEffects } from './CombatEffects';
import { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import { compareProjectedDepth, GridProjection, ProjectedDepth } from './GridProjection';
import { prepareMapSurface } from './MapSurface';
import { entityDisplayName, entityHeadLabels } from './EntityHeadAnchor';
import { StatusEffects } from './StatusEffects';
import { DisplacementEvent, EntityDisplacement } from './EntityDisplacement';
import { SourceProjection } from './SourceProjection';
import { SourceMapView } from './SourceMapView';
import type { SourceMapEntityOrder } from './SourceMapView';
import type { SourceRect } from './SourceMapModel';

interface Visual {
    node: Node;
    body: Sprite[];
    state: any;
    profile: any;
    motion: EntityMotion;
    animation: EntityAnimation | null;
    keys: string[];
    orderDirection: number;
    pose: AnimationSample | null;
    movement: EntityDisplacement;
}
// 同一保存World可被下一场景复用；旧owner的迟到解绑不能清理新owner的Surface。
const surfaceOwners = new WeakMap<Node, GameWorldView>();

// 权威格坐标用于规则与点击；每帧显示位置及动作独立插值，不生成固定UI布局。
export class GameWorldView {
    manifest: any;
    map: any;
    gameplay: any;
    private assets?: WorldAssets;
    private releaseAssetsInvalidation?: () => void;
    private effects?: CombatEffects;
    private statusEffects?: StatusEffects;
    private lastAttackId = 0n;
    private visuals = new Map<number, Visual>();
    private projection?: GridProjection;
    private sourceView?: SourceMapView;
    private mapReady = false;
    private mapPreparation?: Promise<void>;
    private mapFailure: unknown;
    private readonly props: Array<{ node: Node; foot: Vec3; id: string }> = [];
    private disposed = false;
    private selected: (entity: any) => void = () => {};
    private playerPosition: { x: number; y: number } | null = null;
    private weaponEquipped = false;
    // 仅显式开发验收网址输出入场及后续两个窗口，不创建诊断UI或读取输入框。
    private audit = typeof location !== 'undefined' && new URLSearchParams(location.search).get('renderQA') === '1'
        ? { stage: 'initial', start: 0, last: 0, updates: 0, maxGap: 0, changedFrames: 0, intermediate: [0, 0, 0, 0] } : null;

    constructor(readonly viewport: Node, readonly world: Node, readonly entities: Node, readonly playerName: string, readonly job: number, readonly sceneEpoch: { toString(): string }, readonly playerHandle: number) {
        if (!Number.isInteger(playerHandle) || playerHandle < 1 || playerHandle > 0xffffffff) throw new Error('入场玩家句柄无效');
        if (isValid(this.viewport, true)) this.viewport.on(Node.EventType.SIZE_CHANGED, this.refitCamera, this);
    }

    prepare(assets: WorldAssets, onSelected: (entity: any) => void): void {
        if (this.disposed) { assets.dispose(); return; }
        this.assets = assets; this.selected = onSelected;
        this.releaseAssetsInvalidation = assets.onBeforeInvalidation(reason => {
            if (reason !== 'dispose' || this.disposed) return;
            this.releaseVisuals(); this.disposed = true;
            this.releaseAssetsInvalidation?.(); this.releaseAssetsInvalidation = undefined; this.assets = undefined;
        });
        surfaceOwners.set(this.world, this);
        this.manifest = assets.manifest; this.map = assets.map; this.gameplay = assets.gameplay;
        this.mapReady = false;
        if (assets.sourceMap) {
            this.projection = new SourceProjection(assets.sourceMap.model);
            const surface = this.prepareSourceSurface();
            this.sourceView = new SourceMapView(assets.sourceMap, this.world, surface, this.entities);
        } else {
            this.projection = new GridProjection(this.map);
            // 同一保存场景可以先后承载来源城镇与个人图，只恢复运行数据，不重建保存布局。
            this.world.getComponent(UITransform)!.setAnchorPoint(.5, .5);
            for (const name of ['Soil', 'GridLines']) { const node = this.world.getChildByName(name); if (node) node.active = true; }
            const surface = this.world.getChildByName('Surface'); surface?.getComponent(UITransform)?.setAnchorPoint(.5, .5);
            const sprite = surface?.getComponent(Sprite); if (sprite) sprite.enabled = true;
            prepareMapSurface(this.world, assets, this.projection);
        }
        this.statusEffects = new StatusEffects(assets, this.sceneEpoch, (handle, life) => {
            const visual = this.visuals.get(handle);
            return visual?.state.life?.toString() === life ? visual.node.getChildByName('AttachedEffects') : null;
        });
        for (const prop of assets.sourceMap ? [] : this.map.props ?? []) {
            const definition = this.manifest.frames[prop.frame], frame = assets.frames.get(prop.frame);
            if (!frame || !definition) throw new Error(`地图物件尚未预载：${prop.frame}`);
            const node = instantiate(assets.propPrefab); node.name = prop.id; node.parent = this.entities;
            const transform = node.getComponent(UITransform)!; transform.setAnchorPoint(prop.anchor_x, prop.anchor_y); transform.setContentSize(definition.width, definition.height);
            const sprite = node.getComponent(Sprite)!; sprite.spriteFrame = frame; sprite.trim = false; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
            // 小装饰采用显式脚点及原图像素偏移，不参与HUD布局。
            const foot = this.grid(prop.display_x ?? prop.x, prop.display_y ?? prop.y); node.setPosition(foot.x + prop.pixel_offset_x, foot.y + prop.pixel_offset_y);
            const id = prop.group_id ? `${prop.group_id}:${String(prop.part_index).padStart(4, '0')}:${prop.id}` : prop.id;
            this.props.push({ node, foot, id });
        }
        const effects = this.world.getChildByName('Effects');
        if (!effects) throw new Error('场景尚未保存游戏特效容器');
        this.effects = new CombatEffects(assets, effects, this.sceneEpoch, (x, y) => this.grid(x, y), handle => this.visuals.get(handle)?.node.position ?? null,
            handle => this.visuals.get(handle)?.state.life?.toString() ?? null, handle => {
                const visual = this.visuals.get(handle), pose = visual?.pose;
                return pose?.action === 'attack' && visual?.animation ? { attackSequence: visual.animation.currentAttackSequence, frame: pose.frame, direction: this.projection!.visualDirection(pose.direction) } : null;
            }, this.projection);
        this.mapReady = !this.sourceView;
    }

    get isMapReady(): boolean { return !this.disposed && this.mapReady && (!this.sourceView || this.sourceView.isReady); }
    get mapPreparationError(): unknown { return this.mapFailure; }
    // 来源地图须先取得玩家基线/权威位置，不能用任意出生点准备另一个视口后提前声明就绪。
    prepareViewport(position?: { x: number; y: number }): Promise<void> {
        if (this.disposed || !this.assets) return Promise.reject(new Error('世界视图尚未准备或已经销毁'));
        if (!this.sourceView) return Promise.resolve();
        if (position) {
            if (![position.x, position.y].every(Number.isFinite) || position.x < 0 || position.y < 0 || position.x >= this.map.width || position.y >= this.map.height) return Promise.reject(new Error('来源地图准备位置越界'));
            this.playerPosition = { ...position };
        }
        if (!this.playerPosition) return Promise.reject(new Error('来源地图尚无玩家基线位置'));
        this.refitCamera();
        return this.mapPreparation ?? Promise.reject(new Error('来源地图视口尺寸尚未就绪'));
    }

    acceptCast(event: CastEvent): void { if (!this.disposed) this.effects?.accept(event, game.totalTime); }
    acceptCastStep(event: CastStepEvent): void { if (!this.disposed) this.effects?.step(event, game.totalTime); }
    acceptImpact(event: ImpactEvent): void { if (!this.disposed) this.effects?.impact(event, game.totalTime); }
    acceptDisplacement(event: DisplacementEvent): void {
        if (this.disposed || !this.map) return;
        const inside = (point: { x: number; y: number }): boolean => !!point && point.x >= 0 && point.y >= 0 && point.x < this.map.width && point.y < this.map.height;
        if (!inside(event.origin) || !event.path?.every(inside)) return;
        const visual = this.visuals.get(event.entityHandle), accepted = visual?.movement.accept(event);
        if (!visual || !accepted) return;
        const now = game.totalTime;
        visual.animation?.displace(event.attackSequence, event.castSequence);
        visual.motion.displace(event, now, event.entityHandle === this.playerHandle, accepted.recover);
        visual.state = { ...visual.state, position: visual.movement.position, direction: event.facingDirection,
            movementRevision: event.movementRevision, movementKind: event.kind };
        this.effects?.displacement(event, now, event.entityHandle === this.playerHandle);
        this.update(0, this.job, this.weaponEquipped);
    }
    acceptAttack(event: AttackEvent): void {
        if (this.disposed || event.sceneEpoch.toString() !== this.sceneEpoch.toString()) return;
        const id = BigInt(event.eventId.toString()); if (id <= this.lastAttackId) return; this.lastAttackId = id;
        const visual = this.visuals.get(event.casterHandle);
        if (event.casterLife != null && visual?.state.life?.toString() !== event.casterLife.toString()) return;
        visual?.animation?.acceptAttack(event.attackSequence, event.direction);
        this.effects?.attack(event, game.totalTime);
    }

    grid(x: number, y: number): Vec3 { const point = (this.projection ??= new GridProjection(this.map)).project(x, y); return new Vec3(point.x, point.y); }
    logicalDirection(x: number, y: number): number | null { return (this.projection ??= new GridProjection(this.map)).logicalDirectionFromScreen(x, y); }

    showAim(grid: { x: number; y: number } | null): void {
        const marker = this.world.getChildByName('TargetMarker');
        if (!marker) throw new Error('场景尚未保存地面目标标记');
        marker.active = !!grid; if (grid) marker.setPosition(this.grid(grid.x, grid.y));
    }

    screenToGrid(point: Vec2, requireWalkable = true): { x: number; y: number } | null {
        if (!this.map) return null;
        const local = this.world.getComponent(UITransform)!.convertToNodeSpaceAR(new Vec3(point.x, point.y));
        const cell = (this.projection ??= new GridProjection(this.map)).cellAt(local);
        const walkable = !cell || !requireWalkable || (this.assets?.sourceMap ? this.assets.sourceMap.model.isWalkable(cell.x, cell.y)
            : !this.map.blocked.some((point: { x: number; y: number }) => point.x === cell.x && point.y === cell.y));
        return cell && walkable ? cell : null;
    }

    apply(baseline: any): void {
        if (this.disposed || !this.assets) return;
        const now = game.totalTime;
        const alive = new Set<number>();
        for (const state of baseline.entities) {
            alive.add(state.entityHandle);
            let visual = this.visuals.get(state.entityHandle);
            if (!visual) {
                const node = instantiate(this.assets.prefab); node.parent = this.entities;
                const profile = this.assets.profile(state, this.job);
                const body = node.getChildByName('Body')!;
                if (profile) {
                    const name = node.getChildByName('Name')!, hp = node.getChildByName('HP')!, transform = body.getComponent(UITransform)!;
                    const anchor = entityHeadLabels(profile, this.manifest.frames, { bodyY: body.position.y, bodyHeight: transform.height,
                        bodyAnchorY: transform.anchorPoint.y, bodyScaleY: body.scale.y, nameY: name.position.y, hpY: hp.position.y });
                    name.setPosition(name.position.x, anchor.nameY, name.position.z); hp.setPosition(hp.position.x, anchor.hpY, hp.position.z);
                }
                const sprites = [body.getComponent(Sprite)!];
                // 外观层是动态实体内容，沿用已保存的实体层模板。
                for (let index = 1; index < (profile?.layers.length ?? 1); index++) {
                    const layer = instantiate(body); layer.name = `AppearanceLayer${index}`; layer.parent = node;
                    sprites.push(layer.getComponent(Sprite)!);
                }
                // 原PNG和WIL偏移已包含透明边；恢复auto-trim几何，避免逐帧伸缩。
                for (const sprite of sprites) { sprite.type = Sprite.Type.SIMPLE; sprite.trim = false; }
                node.on(Node.EventType.TOUCH_END, (event: EventTouch) => {
                    event.propagationStopped = true;
                    const current = this.visuals.get(state.entityHandle); if (current) this.selected(current.state);
                });
                visual = { node, body: sprites, state, profile, motion: new EntityMotion(),
                    animation: profile ? new EntityAnimation(profile.layers[0].actions) : null, keys: [], orderDirection: -1, pose: null,
                    movement: new EntityDisplacement(this.sceneEpoch, state.entityHandle) };
                this.visuals.set(state.entityHandle, visual);
            }
            const previous = visual.state;
            if (previous.life?.toString() !== state.life?.toString()) {
                visual.motion.reset(); visual.animation = visual.profile ? new EntityAnimation(visual.profile.layers[0].actions) : null;
            }
            if (!previous.hp.isZero() && state.hp.isZero()) visual.motion.releaseHold(now);
            const decision = state.movementRevision == null ? 'walk'
                : visual.movement.observe(state.life, state.movementRevision, state.movementKind, state.position);
            if (decision === 'stale') visual.motion.touchSnapshot(now);
            else visual.motion.push({ x: state.position.x, y: state.position.y, direction: state.direction,
                durationMs: movementDisplayDurationMs(state.entityKind, visual.profile?.layers[0].actions.walking, state.moveIntervalMs),
                snap: decision === 'initial' || decision === 'recover' || state.entityKind === 4 || (previous.hp.isZero() && !state.hp.isZero()) }, now);
            visual.animation?.observe({ direction: state.direction, action: state.action, attackSequence: state.attackSequence, castSequence: state.castSequence,
                dead: state.hp.isZero(), injured: state.hp.lessThan(previous.hp) }, now);
            if ((decision === 'initial' || decision === 'recover') && state.movementKind >= 2) visual.animation?.displace(state.attackSequence, state.castSequence);
            visual.state = decision === 'stale' ? { ...state, position: visual.movement.position, direction: previous.direction } : state;
            const name = visual.node.getChildByName('Name')!;
            const displayName = entityDisplayName(state.entityKind, state.definitionId, state.entityHandle === this.playerHandle ? this.playerName : '', this.job, this.gameplay);
            name.active = !!displayName;
            name.getComponent(Label)!.string = displayName;
            const hp = visual.node.getChildByName('HP')!;
            hp.active = state.entityKind !== 4;
            hp.getComponent(ProgressBar)!.progress = state.hpMax.isZero() ? 0
                : Math.max(0, Math.min(1, Number(BigInt(state.hp.toString()) * 10000n / BigInt(state.hpMax.toString())) / 10000));
        }
        for (const [id, visual] of Array.from(this.visuals)) if (!alive.has(id)) { this.removeVisual(visual); this.visuals.delete(id); }
        if (baseline.stateSequence != null) this.statusEffects?.observe(baseline.stateSequence, baseline.entities, now);
        // 首帧定位同样走显示模型；后续快照绝不直接设置节点为新格坐标。
        this.update(0, this.job, this.weaponEquipped);
    }

    // 镜头跟随显示位置，避免权威格跳变抵消实体插值；窗口变化仍能独立重夹。
    refitCamera(): void {
        if (this.disposed || !this.map || !this.playerPosition || !isValid(this.viewport, true) || !isValid(this.world, true)) return;
        const view = this.viewport.getComponent(UITransform), world = this.world.getComponent(UITransform);
        if (!isValid(view, true) || !isValid(world, true) || view.width <= 0 || view.height <= 0) return;
        const point = this.grid(this.playerPosition.x, this.playerPosition.y), scale = this.world.scale;
        if (this.sourceView) {
            if (![scale.x, scale.y].every(value => Number.isFinite(value) && value > 0)) return;
            const width = this.map.pixel_width, height = this.map.pixel_height, visibleWidth = view.width / scale.x, visibleHeight = view.height / scale.y;
            const centerX = visibleWidth >= width ? width / 2 : Math.max(visibleWidth / 2, Math.min(width - visibleWidth / 2, point.x));
            const centerY = visibleHeight >= height ? height / 2 : Math.max(visibleHeight / 2, Math.min(height - visibleHeight / 2, -point.y));
            this.world.setPosition((.5 - view.anchorPoint.x) * view.width - centerX * scale.x,
                (.5 - view.anchorPoint.y) * view.height + centerY * scale.y, this.world.position.z);
            this.requestSourceViewport(); return;
        }
        const rangeX = Math.max(0, (world.width * scale.x - view.width) / 2);
        const rangeY = Math.max(0, (world.height * scale.y - view.height) / 2);
        this.world.setPosition(Math.max(-rangeX, Math.min(rangeX, -point.x * scale.x)),
            Math.max(-rangeY, Math.min(rangeY, -point.y * scale.y)), this.world.position.z);
    }

    update(_dt: number, _job: number, weaponEquipped: boolean): void {
        if (this.disposed || !this.assets) return;
        this.weaponEquipped = weaponEquipped;
        const now = game.totalTime;
        const audit = _dt > 0 ? this.audit : null;
        if (audit) {
            if (!audit.updates) audit.start = now;
            else audit.maxGap = Math.max(audit.maxGap, now - audit.last);
            audit.last = now; audit.updates++;
        }
        const sourceOrder: SourceMapEntityOrder[] = [];
        for (const visual of Array.from(this.visuals.values())) {
            const motion = visual.motion.sample(now);
            if (audit && motion.moving && visual.state.entityKind <= 3
                && (Math.abs(motion.x - Math.round(motion.x)) > .001 || Math.abs(motion.y - Math.round(motion.y)) > .001)) audit.intermediate[visual.state.entityKind]++;
            visual.node.setPosition(this.grid(motion.x, motion.y));
            if (this.sourceView) sourceOrder.push({ id: `entity_${visual.state.entityHandle}`, node: visual.node,
                x: motion.x, y: motion.y, row: visual.state.position.y });
            if (visual.state.entityHandle === this.playerHandle) this.playerPosition = { x: motion.x, y: motion.y };
            const pose = visual.animation?.sample(now, motion);
            visual.pose = pose ?? null;
            if (!pose) { for (const sprite of visual.body) sprite.node.active = false; continue; }
            if (pose.action === 'attack' || pose.action === 'struck' || pose.action === 'casting') visual.motion.holdUntil(visual.animation!.playbackUntil);
            const profile = visual.profile;
            const visualDirection = this.projection!.visualDirection(pose.direction);
            if (visual.orderDirection !== visualDirection) {
                const order: string[] = profile.layer_order_by_direction?.[visualDirection] ?? profile.layers.map((layer: any) => layer.package);
                visual.body.map((sprite, index) => ({ sprite, rank: order.indexOf(profile.layers[index].package) }))
                    .sort((a, b) => a.rank - b.rank).forEach((entry, index) => entry.sprite.node.setSiblingIndex(index));
                visual.orderDirection = visualDirection;
            }
            visual.body.forEach((sprite, index) => {
                const layer = profile.layers[index];
                const animation = layer.actions[pose.action];
                const keys: string[] = animation.directions[visualDirection].frames;
                const key = keys[Math.min(keys.length - 1, pose.frame)];
                const definition = this.manifest.frames[key];
                const visible = !(layer.package === 'Weapon' && !weaponEquipped) && !definition.declared_empty;
                if (sprite.node.active !== visible) sprite.node.active = visible;
                if (!visible || visual.keys[index] === key) return;
                // 所有非空动作帧已在取票前加载；播放中不清空Sprite或发异步资源请求。
                const frame = this.assets!.frames.get(key);
                if (!frame) throw new Error(`动作帧尚未准备：${key}`);
                sprite.spriteFrame = frame; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
                const transform = sprite.node.getComponent(UITransform)!;
                transform.setAnchorPoint(0, 1); transform.setContentSize(definition.width, definition.height);
                sprite.node.setPosition(definition.offset_x - 24, 16 - definition.offset_y);
                visual.keys[index] = key;
                if (audit) audit.changedFrames++;
            });
        }
        // 依据当帧脚底位置处理遮挡，HUD不参与世界深度排序。
        if (this.sourceView) this.sourceView.setEntities(sourceOrder);
        else {
            const depth: Array<ProjectedDepth & { node: Node }> = this.props.map(prop => ({ node: prop.node, x: prop.foot.x, y: prop.foot.y, layer: 0, id: prop.id }));
            for (const visual of Array.from(this.visuals.values())) depth.push({ node: visual.node, x: visual.node.position.x, y: visual.node.position.y, layer: 1, id: `entity_${visual.state.entityHandle}` });
            depth.sort(compareProjectedDepth).forEach((entry, index) => { if (entry.node.getSiblingIndex() !== index) entry.node.setSiblingIndex(index); });
        }
        this.effects?.update(now);
        this.statusEffects?.update(now);
        this.refitCamera();
        if (audit && now - audit.start >= 5000) {
            console.info('表现验收', JSON.stringify({ scope: 'development_render', stage: audit.stage, duration_ms: Math.round(now - audit.start),
                updates: audit.updates, mean_update_ms: (now - audit.start) / (audit.updates - 1), max_update_ms: audit.maxGap,
                intermediate_grid_samples: { player: audit.intermediate[1], pet: audit.intermediate[2], monster: audit.intermediate[3] },
                changed_animation_frames: audit.changedFrames, preloaded_frames: this.assets.frames.size }));
            this.audit = audit.stage === 'initial' ? { stage: 'steady', start: now, last: now, updates: 1,
                maxGap: 0, changedFrames: 0, intermediate: [0, 0, 0, 0] } : null;
        }
    }

    private prepareSourceSurface(): Node {
        const surface = this.world.getChildByName('Surface'); if (!surface) throw new Error('场景尚未保存来源地图Surface容器');
        const transform = this.world.getComponent(UITransform)!; transform.setAnchorPoint(0, 1); transform.setContentSize(this.map.pixel_width, this.map.pixel_height);
        const worldSprite = this.world.getComponent(Sprite); if (worldSprite) worldSprite.enabled = false;
        surface.setPosition(0, 0, 0); surface.setScale(1, 1, 1); surface.angle = 0;
        surface.getComponent(UITransform)!.setAnchorPoint(0, 1); surface.getComponent(UITransform)!.setContentSize(this.map.pixel_width, this.map.pixel_height);
        const background = surface.getComponent(Sprite); if (background) { background.spriteFrame = null; background.enabled = false; }
        this.entities.setPosition(0, 0, 0); this.entities.setScale(1, 1, 1); this.entities.angle = 0;
        for (const name of ['Soil', 'GridLines']) {
            const node = this.world.getChildByName(name); if (node) { node.getComponent(Graphics)?.clear(); node.active = false; }
        }
        const marker = this.world.getChildByName('TargetMarker');
        if (marker) {
            marker.active = false; const sprite = marker.getComponent(Sprite); if (sprite) sprite.enabled = false;
            const graphics = marker.getChildByName('Diamond')?.getComponent(Graphics);
            if (graphics) {
                graphics.clear(); graphics.fillColor = new Color(255, 220, 90, 65); graphics.rect(-24, -16, 48, 32); graphics.fill();
                graphics.strokeColor = new Color(255, 220, 90, 210); graphics.lineWidth = 1.5; graphics.rect(-24, -16, 48, 32); graphics.stroke();
            }
        }
        return surface;
    }
    private requestSourceViewport(): void {
        if (!this.sourceView) return;
        const view = this.viewport.getComponent(UITransform)!, world = this.world.getComponent(UITransform)!;
        const left = -view.width * view.anchorPoint.x, bottom = -view.height * view.anchorPoint.y;
        const points = [[left, bottom], [left + view.width, bottom], [left + view.width, bottom + view.height], [left, bottom + view.height]]
            .map(([x, y]) => world.convertToNodeSpaceAR(view.convertToWorldSpaceAR(new Vec3(x, y))));
        const rect: SourceRect = [Math.min(...points.map(point => point.x)), -Math.max(...points.map(point => point.y)),
            Math.max(...points.map(point => point.x)), -Math.min(...points.map(point => point.y))];
        let preparation: Promise<void>;
        try { preparation = this.sourceView.setViewport(rect, game.totalTime); }
        catch (error) { preparation = Promise.reject(error); }
        if (preparation === this.mapPreparation) return;
        this.mapReady = false; this.mapFailure = undefined; this.mapPreparation = preparation;
        void preparation.then(() => { if (!this.disposed && this.mapPreparation === preparation) this.mapReady = true; }, error => {
            if (!this.disposed && this.mapPreparation === preparation) { this.mapReady = false; this.mapFailure = error; }
        });
    }


    private removeVisual(visual: Visual): void {
        for (const sprite of visual.body) if (isValid(sprite)) { sprite.spriteFrame = null; sprite.customMaterial = null; }
        if (isValid(visual.node, true)) { visual.node.active = false; visual.node.parent = null; visual.node.destroy(); }
    }
    private releaseVisuals(): void {
        this.mapReady = false; this.sourceView?.dispose(); this.sourceView = undefined; this.mapPreparation = undefined;
        if (isValid(this.viewport, true)) this.viewport.off(Node.EventType.SIZE_CHANGED, this.refitCamera, this);
        this.playerPosition = null;
        // 附体/战斗特效先解除帧，再销毁其所属实体；Creator的destroy实际在帧尾完成。
        this.effects?.dispose(); this.effects = undefined; this.statusEffects?.dispose(); this.statusEffects = undefined;
        for (const visual of Array.from(this.visuals.values())) this.removeVisual(visual);
        this.visuals.clear();
        for (const prop of this.props) if (isValid(prop.node, true)) { const sprite = prop.node.getComponent(Sprite); if (sprite) sprite.spriteFrame = null; prop.node.destroy(); }
        this.props.length = 0;
        if (isValid(this.world, true) && surfaceOwners.get(this.world) === this) {
            const surface = this.world.getChildByName('Surface')?.getComponent(Sprite);
            if (surface) { surface.spriteFrame = null; surface.enabled = false; }
            surfaceOwners.delete(this.world);
        }
    }
    dispose(): void {
        if (this.disposed && !this.assets) return;
        this.releaseVisuals(); this.disposed = true;
        this.releaseAssetsInvalidation?.(); this.releaseAssetsInvalidation = undefined;
        this.assets?.dispose(); this.assets = undefined;
    }
}
