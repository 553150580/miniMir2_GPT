import { CombatEntity } from './CombatStatusModel';
import { PotionInteger } from './PotionModel';
interface Shield { key: string; entityHandle: number; life: string; started: number; hurtAt: number; hp: bigint; }
// 常驻表现只观察完整状态基线；0秒不自行删盾，重复更新也不重启动画。
export class StatusPlayback {
    private scene = '';
    private sequence = -1n;
    private values = new Map<string, Shield>();
    bind(scene: PotionInteger): void { this.clear(); this.scene = scene.toString(); }
    observe(scene: PotionInteger, sequence: PotionInteger, entities: readonly CombatEntity[], now: number): boolean {
        const revision = BigInt(sequence.toString());
        if (scene.toString() !== this.scene || revision <= this.sequence || !Number.isFinite(now)) return false;
        this.sequence = revision;
        const retained = new Set<string>();
        for (const entity of entities) {
            if (!entity.statuses?.some(status => status.statusId === 31)) continue;
            const life = entity.life.toString(), key = `${this.scene}:${entity.entityHandle}:${life}:31`, hp = BigInt(entity.hp.toString());
            retained.add(key);
            const current = this.values.get(key);
            if (current) { if (hp < current.hp) current.hurtAt = now; current.hp = hp; }
            else this.values.set(key, { key, entityHandle: entity.entityHandle, life, started: now, hurtAt: -Infinity, hp });
        }
        for (const key of Array.from(this.values.keys())) if (!retained.has(key)) this.values.delete(key);
        return true;
    }
    poses(now: number, hurtDuration: number) {
        return Array.from(this.values.values()).map(value => ({ ...value, part: now - value.hurtAt < hurtDuration ? 'struck' : 'idle',
            elapsed: now - value.hurtAt < hurtDuration ? now - value.hurtAt : now - value.started }));
    }
    clear(): void { this.values.clear(); this.sequence = -1n; this.scene = ''; }
}
