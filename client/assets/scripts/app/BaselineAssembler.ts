import Long from 'long';

// 有界组装当前连接的基线；预加载完成不能替代完整基线确认。
export class BaselineAssembler {
    private sequence = Long.UZERO;
    private expected = 0;
    private chunks = new Map<number, any>();
    private identity = '';
    private navigationIdentity: string | null = null;
    private safeZoneIdentity: string | null = null;

    reset(): void { this.sequence = Long.UZERO; this.expected = 0; this.chunks.clear(); this.identity = ''; this.navigationIdentity = null; this.safeZoneIdentity = null; }

    accept(chunk: any, scene: Long, baseline: Long): any | null {
        if (!chunk.sceneEpoch.equals(scene) || !chunk.baselineId.equals(baseline)) return null;
        const identity = `${scene}:${baseline}`;
        if (identity !== this.identity) { this.reset(); this.identity = identity; }
        if (!Number.isInteger(chunk.chunkCount) || chunk.chunkCount < 1 || chunk.chunkCount > 64 || !Number.isInteger(chunk.chunkIndex) || chunk.chunkIndex < 0 || chunk.chunkIndex >= chunk.chunkCount || !Array.isArray(chunk.entities) || chunk.entities.length > 2048) throw new Error('场景基线分块越界');
        if (chunk.stateSequence.lessThan(this.sequence)) return null;
        if (chunk.stateSequence.greaterThan(this.sequence)) { this.sequence = chunk.stateSequence; this.chunks.clear(); this.expected = chunk.chunkCount; this.navigationIdentity = null; this.safeZoneIdentity = null; }
        if (this.expected !== chunk.chunkCount) throw new Error('同一基线序列的分块数不一致');
        const safeZone = chunk.safeZoneId;
        if (safeZone != null && (!Long.isLong(safeZone) || !safeZone.unsigned && safeZone.isNegative())) throw new Error('安全区64位身份必须保留Long');
        const safeZoneIdentity = safeZone?.toUnsigned().toString() ?? '0';
        if (this.safeZoneIdentity !== null && safeZoneIdentity !== this.safeZoneIdentity) throw new Error('同一基线序列的安全区状态不一致');
        this.safeZoneIdentity = safeZoneIdentity;
        if (chunk.navigationState) {
            const navigation = chunk.navigationState;
            const identity = JSON.stringify([navigation.inputSequence ?? 0, navigation.status ?? 0, navigation.revision?.toString() ?? '0',
                navigation.destination?.x ?? null, navigation.destination?.y ?? null]);
            if (this.navigationIdentity !== null && identity !== this.navigationIdentity) throw new Error('同一基线序列的导航状态不一致');
            this.navigationIdentity = identity;
        }
        this.chunks.set(chunk.chunkIndex, chunk);
        if (this.chunks.size !== this.expected) return null;
        const entities: any[] = [];
        const handles = new Set<number>();
        let skillModeState: any = null;
        let navigationState: any = null;
        for (let index = 0; index < this.expected; index++) {
            skillModeState ??= this.chunks.get(index).skillModeState;
            navigationState ??= this.chunks.get(index).navigationState;
            for (const entity of this.chunks.get(index).entities) {
                if (!Number.isInteger(entity.entityHandle) || entity.entityHandle < 1 || handles.has(entity.entityHandle)) throw new Error('基线实体句柄无效或重复');
                handles.add(entity.entityHandle); entities.push(entity);
                if (entities.length > 2048) throw new Error('场景基线实体总数越界');
            }
        }
        return { ...chunk, entities, skillModeState, navigationState, safeZoneId: Long.fromString(this.safeZoneIdentity ?? '0', true) };
    }
}
