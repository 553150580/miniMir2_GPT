import Long from 'long';
import { BaselineAssembler } from './BaselineAssembler';

// 单次退出RPC只收取其原scope、同一stateSequence的完整响应；不借周期快照补齐取消确认。
export class NavigationCancelAssembler {
    private readonly assembler = new BaselineAssembler();
    private sequence: string | null = null;
    private readonly indexes = new Set<number>();
    private complete = false;
    constructor(private readonly scene: Long, private readonly baseline: Long, private readonly mapId?: Long) {
        if (!Long.isLong(scene) || !Long.isLong(baseline) || scene.isZero() || baseline.isZero() || !scene.unsigned && scene.isNegative() || !baseline.unsigned && baseline.isNegative()) throw new Error('导航取消缺少完整基线身份');
    }
    accept(chunk: any): any | null {
        if (this.complete) throw new Error('导航取消已完成，不能重复组装');
        if (!Long.isLong(chunk?.sceneEpoch) || !Long.isLong(chunk?.baselineId) || !chunk.sceneEpoch.equals(this.scene) || !chunk.baselineId.equals(this.baseline)) throw new Error('导航取消响应属于旧场景或基线');
        if (!Long.isLong(chunk.stateSequence) || chunk.stateSequence.isZero() || !chunk.stateSequence.unsigned && chunk.stateSequence.isNegative()) throw new Error('导航取消响应序列无效');
        const sequence = chunk.stateSequence.toString();
        if (this.sequence !== null && this.sequence !== sequence) throw new Error('导航取消分块序列不一致');
        if (!Number.isInteger(chunk.chunkIndex) || this.indexes.has(chunk.chunkIndex) || this.indexes.size >= 64) throw new Error('导航取消分块重复或超过上限');
        if (!chunk.navigationState || ![0, 4].includes(chunk.navigationState.status)) throw new Error('远端导航取消尚未确认');
        this.sequence = sequence;
        const value = this.assembler.accept(chunk, this.scene, this.baseline, this.mapId);
        this.indexes.add(chunk.chunkIndex);
        if (!value) return null;
        this.complete = true;
        return { ...value, assembledNavigationCancel: true };
    }
}
