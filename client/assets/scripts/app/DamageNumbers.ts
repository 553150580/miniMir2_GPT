import { Color, instantiate, isValid, Label, Node, Prefab, UIOpacity, UITransform, Vec3 } from 'cc';
import type { ImpactEvent } from './CastPlayback';
import { travelInteger } from './MapTravelModel';

const capacity = 10;
const lifetimeMs = 720;
const risePixels = 42;
const offsets = [0, -32, 32, -60, 60];
const criticalColor = new Color(255, 230, 140); // 暴击沿用数字字形，以金色区分。

interface NumberNode { node: Node; label: Label; opacity: UIOpacity; normalColor: Color; }
interface ActiveNumber extends NumberNode {
    eventId: string;
    targetHandle: number;
    targetLife: string;
    amount: string;
    started: number;
    origin: Vec3;
    lane: number;
}

// 只消费当前世界的权威命中。数字独立于攻击画面和本地生命差值。
export class DamageNumbers {
    private readonly active: ActiveNumber[] = [];
    private readonly spare: NumberNode[] = [];
    private readonly transform: UITransform;
    private readonly scene: string;
    private lastEventId = 0n;
    private disposed = false;

    constructor(private readonly prefab: Prefab, private readonly parent: Node, scene: { toString(): string },
        private readonly life: (handle: number) => string | null, private readonly head: (handle: number) => Vec3 | null) {
        const transform = parent.getComponent(UITransform);
        if (!transform) throw new Error('伤害数字缺少已保存的世界特效容器');
        this.transform = transform;
        this.scene = travelInteger(scene, true);
    }

    accept(event: ImpactEvent, now: number): void {
        if (this.disposed || event.sceneEpoch.toString() !== this.scene || !Number.isFinite(now)) return;
        let eventId: string;
        try { eventId = travelInteger(event.eventId, true); } catch { return; }
        const id = BigInt(eventId);
        if (id <= this.lastEventId) return;
        this.lastEventId = id;
        if (event.outcome !== 1 || event.healing || !Number.isInteger(event.targetHandle) || event.targetHandle <= 0 || event.targetHandle > 0xffffffff) return;

        let amount: string, targetLife: string;
        try {
            amount = travelInteger(event.amount, true);
            targetLife = travelInteger(event.targetLife);
        } catch { return; }
        if (BigInt(amount) > 0x7fffffffffffffffn || this.life(event.targetHandle) !== targetLife) return;
        const head = this.head(event.targetHandle);
        if (!head) return;
        const position = this.transform.convertToNodeSpaceAR(head);
        if (![position.x, position.y, position.z].every(Number.isFinite)) return;

        this.update(now);
        const sameTarget = (value: ActiveNumber): boolean => value.targetHandle === event.targetHandle && value.targetLife === targetLife;
        const staggered = this.active.filter(sameTarget);
        // 单个目标的数字也要留出横向间距，满槽优先回收它最早的一条。
        if (staggered.length >= offsets.length) this.recycle(this.active.indexOf(staggered[0]));
        if (this.active.length >= capacity) this.recycle(0);
        const lane = offsets.findIndex((_offset, index) => !this.active.some(value => sameTarget(value) && value.lane === index));
        const origin = new Vec3(position.x + offsets[lane], position.y + 29, position.z);
        const entry = this.spare.pop() ?? this.create();
        entry.label.string = amount;
        entry.label.color = event.critical === true ? criticalColor : entry.normalColor;
        entry.opacity.opacity = 255;
        entry.node.setPosition(origin);
        entry.node.parent = this.parent;
        entry.node.active = true;
        this.active.push({ ...entry, eventId, targetHandle: event.targetHandle, targetLife, amount, started: now, origin, lane });
    }

    update(now: number): void {
        if (this.disposed) return;
        for (let index = this.active.length - 1; index >= 0; index--) {
            const value = this.active[index];
            const progress = Math.max(0, now - value.started) / lifetimeMs;
            if (progress >= 1 || this.life(value.targetHandle) !== value.targetLife) { this.recycle(index); continue; }
            const { x, y, z } = value.origin;
            value.node.setPosition(x, y + risePixels * progress * (2 - progress), z);
            value.opacity.opacity = Math.round(255 * (1 - Math.max(0, progress - .2) / .8));
        }
    }

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        for (const { node, label } of [...this.active, ...this.spare]) {
            if (isValid(label, true)) { label.string = ''; label.font = null; }
            if (isValid(node, true)) { node.active = false; node.parent = null; node.destroy(); }
        }
        this.active.length = 0;
        this.spare.length = 0;
    }

    private create(): NumberNode {
        const node = instantiate(this.prefab), label = node.getComponent(Label), opacity = node.getComponent(UIOpacity);
        if (!label || !opacity || !node.getComponent(UITransform)) {
            node.destroy();
            throw new Error('伤害数字预制体根缺少标签、透明度或位置组件');
        }
        return { node, label, opacity, normalColor: label.color.clone() };
    }

    private recycle(index: number): void {
        const [{ node, label, opacity, normalColor }] = this.active.splice(index, 1);
        node.active = false;
        node.parent = null;
        label.string = '';
        this.spare.push({ node, label, opacity, normalColor });
    }
}
