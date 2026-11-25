import { travelInteger } from './MapTravelModel';
import type { TravelInteger } from './MapTravelModel';

interface SceneIdentity {
    sceneEpoch: TravelInteger; baselineId: TravelInteger; roomId: TravelInteger; sceneKind: number;
}

export interface DungeonSceneGrant {
    challengeId: Uint8Array; sourceSceneEpoch: TravelInteger; targetSceneEpoch: TravelInteger;
    targetBaselineId: TravelInteger; targetMapId: TravelInteger; targetSceneKind: number;
}

export interface DungeonBattleProgress {
    challengeId: Uint8Array; sceneEpoch: TravelInteger; stage: number; stageCount: number; state: number;
}

function challengeIdentity(value: Uint8Array): string {
    return value instanceof Uint8Array && value.length === 16
        ? Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('') : '';
}

// 当前连接先收到服务端授权，再安装精确匹配的副本或原地图基线。
export class DungeonTransition {
    private home: { id: string; map: string; kind: number } | null = null;
    private grant: { id: string; source: string; scene: string; baseline: string; map: string; kind: number } | null = null;
    private progress: { stage: number; state: number } | null = null;

    clear(): void { this.home = this.grant = this.progress = null; }

    observe(value: DungeonSceneGrant, current: SceneIdentity): void {
        const id = challengeIdentity(value.challengeId);
        if (!id) return;
        const source = travelInteger(value.sourceSceneEpoch, true), scene = travelInteger(value.targetSceneEpoch, true);
        const baseline = travelInteger(value.targetBaselineId, true), map = travelInteger(value.targetMapId, true), kind = value.targetSceneKind;
        if (source !== travelInteger(current.sceneEpoch, true) || BigInt(scene) <= BigInt(source) ||
            BigInt(baseline) <= BigInt(travelInteger(current.baselineId, true))) return;
        if (kind === 5) {
            if (this.home || ![1, 2].includes(current.sceneKind)) return;
        } else if (current.sceneKind !== 5 || !this.home || this.home.id !== id || this.home.map !== map || this.home.kind !== kind) return;
        // 同一原场景只接收一份授权；重复包不能改写目标或场次。
        if (!this.grant) this.grant = { id, source, scene, baseline, map, kind };
    }

    accept(id: string, next: SceneIdentity, current: SceneIdentity): boolean {
        const grant = this.grant;
        if (!grant || grant.id !== id || grant.kind !== next.sceneKind || grant.source !== travelInteger(current.sceneEpoch, true) ||
            grant.scene !== travelInteger(next.sceneEpoch, true) || grant.baseline !== travelInteger(next.baselineId, true) ||
            grant.map !== travelInteger(next.roomId, true)) return false;
        this.home = next.sceneKind === 5 ? { id, map: travelInteger(current.roomId, true), kind: current.sceneKind } : null;
        this.grant = null;
        this.progress = null;
        return true;
    }

    observeProgress(value: DungeonBattleProgress, current: SceneIdentity): void {
        if (current.sceneKind !== 5 || !this.home || challengeIdentity(value.challengeId) !== this.home.id ||
            travelInteger(value.sceneEpoch, true) !== travelInteger(current.sceneEpoch, true) ||
            value.stageCount !== 4 || !Number.isInteger(value.stage) || value.stage < 1 || value.stage > value.stageCount ||
            !Number.isInteger(value.state) || value.state < 1 || value.state > 6) return;
        const previous = this.progress;
        if (previous && (value.stage < previous.stage || value.state < previous.state || previous.state >= 4)) return;
        this.progress = { stage: value.stage, state: value.state };
    }

    // 只报告原场次已经确认的最终段胜利，供回场后的待领查询使用。
    confirmedVictory(challengeId: Uint8Array): boolean {
        return this.progress?.stage === 4 && this.progress.state === 4 && this.home?.id === challengeIdentity(challengeId);
    }

    get objective(): string {
        const p = this.progress;
        if (!p || p.state === 1) return '讨伐 · 等待队伍就绪';
        if (p.state === 2) return `讨伐 · 第 ${p.stage}/4 段`;
        return ['正在结算', '已通关', '挑战失败', '系统中断'][p.state - 3];
    }
}
