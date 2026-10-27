import { EventTouch, game, instantiate, isValid, Label, Node, ProgressBar, Sprite, UITransform, Vec2, Vec3 } from 'cc';
import { EntityAnimation } from './EntityAnimation';
import { EntityMotion } from './EntityMotion';
import { WorldAssets } from './WorldAssets';
import { CombatEffects } from './CombatEffects';
import { CastEvent } from './CastPlayback';

interface Visual {
    node: Node;
    body: Sprite[];
    state: any;
    profile: any;
    motion: EntityMotion;
    animation: EntityAnimation | null;
    keys: string[];
    orderDirection: number;
}

// 权威格坐标用于规则与点击；每帧显示位置及动作独立插值，不生成固定UI布局。
export class GameWorldView {
    manifest: any;
    map: any;
    gameplay: any;
    private assets?: WorldAssets;
    private effects?: CombatEffects;
    private visuals = new Map<number, Visual>();
    private disposed = false;
    private selected: (entity: any) => void = () => {};
    private playerPosition: { x: number; y: number } | null = null;
    private weaponEquipped = false;
    // 仅显式开发验收网址输出入场及后续两个窗口，不创建诊断UI或读取输入框。
    private audit = typeof location !== 'undefined' && new URLSearchParams(location.search).get('renderQA') === '1'
        ? { stage: 'initial', start: 0, last: 0, updates: 0, maxGap: 0, changedFrames: 0, intermediate: [0, 0, 0, 0] } : null;

    constructor(readonly viewport: Node, readonly world: Node, readonly entities: Node, readonly playerName: string, readonly job: number, readonly sceneEpoch: { toString(): string }) {
        if (isValid(this.viewport, true)) this.viewport.on(Node.EventType.SIZE_CHANGED, this.refitCamera, this);
    }

    prepare(assets: WorldAssets, onSelected: (entity: any) => void): void {
        if (this.disposed) { assets.dispose(); return; }
        this.assets = assets; this.selected = onSelected;
        this.manifest = assets.manifest; this.map = assets.map; this.gameplay = assets.gameplay;
        this.world.getComponent(UITransform)!.setContentSize(this.map.width * this.map.tile_width, this.map.height * this.map.tile_height);
        const sprite = this.world.getComponent(Sprite)!;
        sprite.spriteFrame = assets.background; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
        const effects = this.world.getChildByName('Effects');
        if (!effects) throw new Error('场景尚未保存游戏特效容器');
        this.effects = new CombatEffects(assets, effects, this.sceneEpoch, (x, y) => this.grid(x, y), handle => this.visuals.get(handle)?.node.position ?? null);
    }

    acceptCast(event: CastEvent): void { if (!this.disposed) this.effects?.accept(event, game.totalTime); }

    grid(x: number, y: number): Vec3 { return new Vec3((x + .5) * 48 - this.map.width * 24, (this.map.height - y - .5) * 32 - this.map.height * 16); }

    screenToGrid(point: Vec2): { x: number; y: number } | null {
        if (!this.map) return null;
        const local = this.world.getComponent(UITransform)!.convertToNodeSpaceAR(new Vec3(point.x, point.y));
        const x = Math.floor((local.x + this.map.width * 24) / 48), y = Math.floor((this.map.height * 16 - local.y) / 32);
        return x >= 0 && y >= 0 && x < this.map.width && y < this.map.height && !this.map.cells[y][x].blocked ? { x, y } : null;
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
                    animation: profile ? new EntityAnimation(profile.layers[0].actions) : null, keys: [], orderDirection: -1 };
                this.visuals.set(state.entityHandle, visual);
            }
            const previous = visual.state;
            if (!previous.hp.isZero() && state.hp.isZero()) visual.motion.releaseHold(now);
            visual.motion.push({ x: state.position.x, y: state.position.y, direction: state.direction,
                durationMs: state.moveIntervalMs, snap: state.entityKind === 4 || (previous.hp.isZero() && !state.hp.isZero()) }, now);
            visual.animation?.observe({ direction: state.direction, action: state.action, attackSequence: state.attackSequence, castSequence: state.castSequence,
                dead: state.hp.isZero(), injured: state.hp.lessThan(previous.hp) }, now);
            visual.state = state;
            const name = visual.node.getChildByName('Name')!;
            name.active = state.entityKind === 1 || state.entityKind === 4;
            name.getComponent(Label)!.string = state.entityKind === 1 ? this.playerName : '掉落';
            const hp = visual.node.getChildByName('HP')!;
            hp.active = state.entityKind !== 4;
            hp.getComponent(ProgressBar)!.progress = state.hpMax.isZero() ? 0
                : Math.max(0, Math.min(1, Number(BigInt(state.hp.toString()) * 10000n / BigInt(state.hpMax.toString())) / 10000));
        }
        for (const [id, visual] of this.visuals) if (!alive.has(id)) { visual.node.destroy(); this.visuals.delete(id); }
        // 首帧定位同样走显示模型；后续快照绝不直接设置节点为新格坐标。
        this.update(0, this.job, this.weaponEquipped);
    }

    // 镜头跟随显示位置，避免权威格跳变抵消实体插值；窗口变化仍能独立重夹。
    refitCamera(): void {
        if (this.disposed || !this.map || !this.playerPosition || !isValid(this.viewport, true) || !isValid(this.world, true)) return;
        const view = this.viewport.getComponent(UITransform), world = this.world.getComponent(UITransform);
        if (!isValid(view, true) || !isValid(world, true) || view.width <= 0 || view.height <= 0) return;
        const point = this.grid(this.playerPosition.x, this.playerPosition.y), scale = this.world.scale;
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
        for (const visual of this.visuals.values()) {
            const motion = visual.motion.sample(now);
            if (audit && motion.moving && visual.state.entityKind <= 3
                && (Math.abs(motion.x - Math.round(motion.x)) > .001 || Math.abs(motion.y - Math.round(motion.y)) > .001)) audit.intermediate[visual.state.entityKind]++;
            visual.node.setPosition(this.grid(motion.x, motion.y));
            if (visual.state.entityKind === 1) this.playerPosition = { x: motion.x, y: motion.y };
            const pose = visual.animation?.sample(now, motion);
            if (!pose) { for (const sprite of visual.body) sprite.node.active = false; continue; }
            if (pose.action === 'attack' || pose.action === 'struck' || pose.action === 'casting') visual.motion.holdUntil(visual.animation!.playbackUntil);
            const profile = visual.profile;
            if (visual.orderDirection !== pose.direction) {
                const order: string[] = profile.layer_order_by_direction?.[pose.direction] ?? profile.layers.map((layer: any) => layer.package);
                visual.body.map((sprite, index) => ({ sprite, rank: order.indexOf(profile.layers[index].package) }))
                    .sort((a, b) => a.rank - b.rank).forEach((entry, index) => entry.sprite.node.setSiblingIndex(index));
                visual.orderDirection = pose.direction;
            }
            visual.body.forEach((sprite, index) => {
                const layer = profile.layers[index];
                const animation = layer.actions[pose.action];
                const keys: string[] = animation.directions[pose.direction].frames;
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
        Array.from(this.visuals.values()).sort((a, b) => b.node.position.y - a.node.position.y || a.state.entityHandle - b.state.entityHandle)
            .forEach((visual, index) => { if (visual.node.getSiblingIndex() !== index) visual.node.setSiblingIndex(index); });
        this.effects?.update(now);
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

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        if (isValid(this.viewport, true)) this.viewport.off(Node.EventType.SIZE_CHANGED, this.refitCamera, this);
        this.playerPosition = null;
        for (const visual of this.visuals.values()) if (isValid(visual.node, true)) visual.node.destroy();
        this.visuals.clear(); this.effects?.dispose(); this.effects = undefined; this.assets?.dispose(); this.assets = undefined;
    }
}
