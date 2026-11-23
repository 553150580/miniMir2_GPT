import { travelInteger } from './MapTravelModel';
import type { TravelInteger } from './MapTravelModel';

interface SceneIdentity {
    sceneEpoch: TravelInteger;
    roomId: TravelInteger;
    sceneKind: number;
}

// 只接受已收到首领状态的进场，以及随后返回同一张挂机图的服务端迁移。
export class BossTransition {
    private visit: { id: string; kind: number; target: string; home: string; inside: boolean; finished: boolean } | null = null;

    clear(): void { this.visit = null; }

    observe(kind: 3 | 4, id: string, state: number, scene: TravelInteger, current: SceneIdentity | null, error: number): void {
        if (!current || error || !/^[0-9a-f]{32}$/.test(id)) return;
        const epoch = travelInteger(scene, true);
        const currentEpoch = travelInteger(current.sceneEpoch, true);
        if (state === 2 && current.sceneKind === 1 && BigInt(epoch) > BigInt(currentEpoch) && !this.visit?.inside) {
            this.visit = { id, kind, target: epoch, home: travelInteger(current.roomId, true), inside: false, finished: false };
        } else if (state >= 4 && state <= 7 && this.visit?.id === id && this.visit.kind === kind) {
            // 终态可能紧跟首基线，仍须核对终态发生在此次挑战场景。
            if (epoch === this.visit.target) this.visit.finished = true;
        }
    }

    accept(id: string, next: SceneIdentity, current: SceneIdentity | null): boolean {
        const visit = this.visit;
        if (!visit || visit.id !== id || !current) return false;
        if (!visit.inside && next.sceneKind === visit.kind && current.sceneKind === 1
            && travelInteger(next.sceneEpoch, true) === visit.target && travelInteger(current.roomId, true) === visit.home) {
            visit.inside = true;
            return true;
        }
        if (visit.inside && visit.finished && current.sceneKind === visit.kind && next.sceneKind === 1
            && travelInteger(current.sceneEpoch, true) === visit.target && travelInteger(next.roomId, true) === visit.home) {
            this.clear();
            return true;
        }
        return false;
    }
}
