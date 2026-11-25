// 本文件为镜像竞技持有独立的等距地图、资源和表现流水，不修改原场景或其网络附着。
import { isValid, Node } from 'cc';
import { GameWorldView } from './GameWorldView';
import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';
import type { MirrorArenaFrame } from './MirrorArenaModel';
import { WorldAssets } from './WorldAssets';

// 共享预载与准备的实际资源计数，供已保存的面板更新状态。
type Progress = (completed: number, total: number) => void;

interface MirrorPreload {
    key: string; // 规则、地图、职业组成的只读资源身份。
    load?: Promise<WorldAssets | null>; // 过期结果只归还资源，不交给新场次。
    assets?: WorldAssets; // 始终由当前预载任务或当前视图之一持有。
    lastProgress?: [completed: number, total: number];
    listeners: Set<Progress>; // 支持先打开大厅，后进入同一场战斗时观察同批进度。
}

interface MirrorView {
    scope: string; // 原连接、角色与镜像场次均组成这一附着的表现身份。
    battleKey: string; // 纯模型已核验的镜像身份，每次换票独立。
    current: () => boolean;
    view: GameWorldView;
    sequence: bigint; // 镜像快照的消费水位独立于普通场景。
}

export class MirrorArenaPresentation {
    private readonly viewport: Node;
    private readonly world: Node;
    private readonly entities: Node;
    private queued?: MirrorPreload;
    private active?: MirrorView;
    private generation = 0;
    private disposed = false;

    constructor(overlay: Node) {
        const viewport = overlay.getChildByName('MapViewport');
        const world = viewport?.getChildByName('World');
        const entities = world?.getChildByName('Entities');
        if (!viewport || !world || !entities) throw new Error('镜像竞技缺少保存的战斗地图布局');
        this.viewport = viewport; this.world = world; this.entities = entities;
        this.world.active = false;
    }

    // 大厅可在预占入场次数前预载。资源任务按地图、规则和职业复用且没有角色资产。
    async preload(mapId: TravelInteger, rule: TravelInteger, job: number, current: () => boolean, progress?: Progress): Promise<boolean> {
        if (this.disposed || !current()) return false;
        const pending = this.queueAssets(mapId, rule, job);
        const assets = await this.waitForAssets(pending, current, progress);
        return !!assets && this.queued === pending && !this.disposed && current();
    }

    // 权威场次确认后才挂接已经准备好的资源；成功后调用方可发原场次首帧确认。
    async prepare(identity: AttachmentIdentity, battleKey: string, rule: TravelInteger, playerName: string, job: number,
        current: () => boolean, progress?: Progress): Promise<boolean> {
        if (this.disposed || !battleKey || !current()) return false;
        const scope = attachmentKey(identity);
        if (this.active?.scope === scope && this.active.battleKey === battleKey && this.active.current()) return true;
        const generation = ++this.generation;
        this.clearView();
        const pending = this.queueAssets(identity.map, rule, job);
        const belongs = () => !this.disposed && generation === this.generation && current();
        const assets = await this.waitForAssets(pending, belongs, progress);
        if (!assets || this.queued !== pending || !belongs()) return false;
        this.queued = undefined;

        // 协议固定句柄 1 为进攻方，视线跟随镜像中的进攻身体。
        const view = new GameWorldView(this.viewport, this.world, this.entities, playerName, job, identity.scene, 1);
        const active: MirrorView = { scope, battleKey, current, view, sequence: 0n };
        this.active = active;
        try {
            view.bindPresentation(identity, () => this.active === active && current());
            view.prepare(assets, () => {});
            if (!view.isMapReady) throw new Error('镜像竞技地图尚未准备');
            return true;
        } catch (error) {
            this.clearView();
            throw error;
        }
    }

    // 只消费当前场次的全量状态与副流事件。先播放位移经过格，再用同帧权威位置核物理落点。
    apply(frame: MirrorArenaFrame): boolean {
        const active = this.active;
        if (!active?.current() || active.battleKey !== frame.battleKey) return false;
        const sequence = BigInt(travelInteger(frame.sequence, true));
        if (sequence <= active.sequence) return false;
        const view = active.view;
        const inside = ({ x, y }: { x: number; y: number }) => x >= 0 && y >= 0 && x < view.map.width && y < view.map.height;
        if (frame.entities.some(({ position }) => !inside(position))) return false;

        for (const event of frame.observations) if (event.messageId === 30013) view.acceptDisplacement(event.value);
        view.apply({ stateSequence: frame.sequence, entities: frame.entities, fireWallCells: frame.fireWallCells?.filter(inside) ?? [] });
        for (const event of frame.observations) {
            switch (event.messageId) {
            case 30006: view.acceptCast(event.value); break;
            case 30007: view.acceptAttack(event.value); break;
            case 30008: view.acceptImpact(event.value); break;
            case 30012: view.acceptCastStep(event.value); break;
            case 30015:
                // 资源未就绪期间经过的时间计入出场动作；原视觉和音效按自己的完成时钟退出。
                view.presentPetBorn({ event: event.value, elapsedMs: event.value.elapsedMs + Math.max(0, performance.now() - frame.receivedAt) });
                break;
            }
        }
        active.sequence = sequence;
        this.world.active = true;
        return true;
    }

    update(dt: number): void {
        const active = this.active;
        if (active?.current() && active.sequence > 0n) active.view.update(dt, active.view.job);
    }

    reset(): void {
        ++this.generation;
        this.clearView();
        this.discardPreload();
    }

    dispose(): void {
        this.disposed = true;
        this.reset();
    }

    private clearView(): void {
        const active = this.active;
        this.active = undefined;
        if (isValid(this.world, true)) this.world.active = false;
        active?.view.dispose();
    }

    private discardPreload(): void {
        const queued = this.queued;
        this.queued = undefined;
        queued?.assets?.dispose();
        queued?.listeners.clear();
    }

    private queueAssets(map: TravelInteger, rule: TravelInteger, job: number): MirrorPreload {
        const mapId = travelInteger(map, true), version = travelInteger(rule, true);
        const key = JSON.stringify([mapId, version, job]);
        if (this.queued?.key === key) return this.queued;
        this.discardPreload();
        const pending: MirrorPreload = { key, listeners: new Set() };
        this.queued = pending;
        pending.load = WorldAssets.prepare(job, (completed, total) => {
            if (this.queued !== pending) return;
            pending.lastProgress = [completed, total];
            for (const listener of Array.from(pending.listeners)) listener(completed, total);
        }, mapId, true).then(assets => {
            if (this.queued !== pending) { assets.dispose(); return null; }
            const arena = assets.gameplay.mirror_arena;
            if (assets.sceneKind !== 6 || assets.mapId !== mapId || String(assets.gameplay.rule_version) !== version
                || arena?.enabled !== true || arena.map_id !== mapId) {
                assets.dispose();
                throw new Error('镜像竞技地图或资源规则与当前场次不符');
            }
            pending.assets = assets;
            return assets;
        }).catch(error => {
            if (this.queued !== pending) return null;
            this.queued = undefined;
            throw error;
        });
        return pending;
    }

    private async waitForAssets(pending: MirrorPreload, current: () => boolean, progress?: Progress): Promise<WorldAssets | null> {
        const listener: Progress = (completed, total) => { if (current()) progress?.(completed, total); };
        pending.listeners.add(listener);
        try {
            if (pending.lastProgress) listener(...pending.lastProgress);
            return await pending.load ?? null;
        } finally { pending.listeners.delete(listener); }
    }
}
