import Long from 'long';

export interface GrowthEquipmentDefinition {
    kind: string; name: string; required_level?: number; allowed_jobs?: readonly number[];
}
function positiveLevel(value: unknown): value is number { return typeof value === 'number' && Number.isSafeInteger(value) && value > 0 && value <= 0xffffffff; }

// 需求等级只取公开权威定义；查看不受门槛限制，穿戴不能从名称或默认值猜资格。
export function equipmentLevelReason(definition: GrowthEquipmentDefinition | undefined, level: number | undefined, job: number | undefined): string {
    if (!definition || definition.kind !== 'equipment') return '该物品不能装备';
    if (!positiveLevel(definition.required_level)) return '装备需求等级尚未同步';
    if (!positiveLevel(level) || job === undefined) return '角色成长资料尚未同步';
    if (definition.allowed_jobs?.length && !definition.allowed_jobs.includes(job)) return '当前职业不能穿戴';
    return level < definition.required_level ? `需${definition.required_level}级，当前${level}级` : '';
}
export function equipmentLevelLines(definition: GrowthEquipmentDefinition, level: number | undefined, job: number | undefined): string[] {
    if (definition.kind !== 'equipment') return [];
    const reason = equipmentLevelReason(definition, level, job);
    return [positiveLevel(definition.required_level) ? `需求等级 ${definition.required_level}级` : '需求等级：等待权威定义', reason || '已满足职业和等级需求'];
}

export interface ProgressionState {
    formatVersion: number; curveVersion: string; levelCap: number; rebirthCount: number; nextLevelExperience: Long;
    pendingExperience: Long; pendingExperienceLimit: Long; legacyExperienceOverflow: Long; atCap: boolean;
    migrationRequired: boolean; rebirthEnabled: boolean; catalogMaxLevel: number;
}
function unsigned(value: unknown): value is number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 && value <= 0xffffffff; }
function amount(value: Long, name: string, unsignedField = false): bigint {
    if (!Long.isLong(value) || value.isNegative()) throw new Error(name + '必须保持非负Long');
    const result = BigInt(value.toString());
    if (!unsignedField && result > 9223372036854775807n) throw new Error(name + '超出int64范围');
    return result;
}
function exact(value: Long): Long { return Long.fromString(value.toString(), value.unsigned); }
export function observedProgression(value: ProgressionState): ProgressionState {
    if (!value || value.formatVersion !== 1 || typeof value.curveVersion !== 'string' || !value.curveVersion || !positiveLevel(value.levelCap)
        || !unsigned(value.rebirthCount) || !unsigned(value.catalogMaxLevel) || typeof value.atCap !== 'boolean' || typeof value.migrationRequired !== 'boolean'
        || typeof value.rebirthEnabled !== 'boolean') throw new Error('权威成长状态格式无效');
    const pending = amount(value.pendingExperience, '待用经验'), limit = amount(value.pendingExperienceLimit, '经验池上限');
    amount(value.nextLevelExperience, '下级经验'); amount(value.legacyExperienceOverflow, '历史封存经验');
    if (pending > limit && !value.migrationRequired) throw new Error('待用经验超过权威池上限');
    return Object.freeze({ ...value, nextLevelExperience: exact(value.nextLevelExperience), pendingExperience: exact(value.pendingExperience),
        pendingExperienceLimit: exact(value.pendingExperienceLimit), legacyExperienceOverflow: exact(value.legacyExperienceOverflow) });
}
function compact(value: bigint): string {
    const scale = value >= 100000000n ? 100000000n : value >= 10000n ? 10000n : 1n;
    if (scale === 1n) return value.toString();
    const tenths = value * 10n / scale; return `${tenths / 10n}${tenths % 10n ? '.' + tenths % 10n : ''}${scale === 10000n ? '万' : '亿'}`;
}
export function progressionMeter(level: number | undefined, experience: Long | undefined, value: ProgressionState | null | undefined): { text: string; ratio: number } {
    if (!value || !positiveLevel(level) || !experience) return { text: '成长资料同步中', ratio: 0 };
    const state = observedProgression(value), current = amount(experience, '当前经验');
    if (state.migrationRequired) return { text: '成长资料待整理 · 详见角色', ratio: 0 };
    const numerator = state.atCap ? amount(state.pendingExperience, '待用经验') : current;
    const denominator = amount(state.atCap ? state.pendingExperienceLimit : state.nextLevelExperience, '经验门槛');
    if (denominator === 0n || (!state.atCap && level > state.catalogMaxLevel)) return { text: '经验门槛待同步', ratio: 0 };
    return { text: `${state.atCap ? '已达上限 · 待用 ' : ''}${compact(numerator)} / ${compact(denominator)}`,
        ratio: numerator >= denominator ? 1 : Number(numerator * 10000n / denominator) / 10000 };
}
export function progressionSummary(level: number | undefined, value: ProgressionState | null | undefined): string {
    if (!value) return '成长状态同步中';
    return `等级 ${level ?? '—'} / 当前上限 ${value.levelCap}${value.migrationRequired ? ' · 待整理' : value.atCap ? ' · 已达上限' : ''}\n轻触此处查看成长详情`;
}
export function progressionDetailLines(level: number | undefined, experience: Long | undefined, value: ProgressionState | null | undefined): string[] {
    if (!value || !experience) return ['成长状态尚未同步，不推测经验比例'];
    const state = observedProgression(value);
    return [`当前等级 ${level ?? '—'} · 当前上限 ${state.levelCap}`, `已完成转生 ${state.rebirthCount} 次`,
        `当前等级经验 ${amount(experience, '当前经验')}`, state.nextLevelExperience.isZero() ? '下一级门槛：目录未提供' : `下一级经验 ${state.nextLevelExperience}`,
        state.migrationRequired && state.pendingExperienceLimit.isZero() ? `待用历史余额 ${state.pendingExperience} · 当前池上限待核` : `待用经验 ${state.pendingExperience} / ${state.pendingExperienceLimit}`, `历史经验封存 ${state.legacyExperienceOverflow}`,
        state.migrationRequired ? '旧成长资料待服务端整理；保持原等级，不推定已转生' : state.atCap ? '达到当前上限仍可正常战斗与掉落' : '经验按权威目录逐级消费',
        amount(state.pendingExperience, '待用经验') >= amount(state.pendingExperienceLimit, '经验池上限') && !state.pendingExperienceLimit.isZero() ? '待用经验池已满，新增超额经验不再接收' : '超出经验池上限的部分不累计',
        state.rebirthEnabled ? '转生资格以服务端规则为准，本界面不提供转生操作' : '转生尚未开放；不提供假费用或入口',
        `成长曲线 ${state.curveVersion}`, '当前数值为开发候选，不代表自然1至50周期或平衡通过'];
}
export interface ExperienceNotice { readonly role: string; readonly revision: string; readonly source: string; readonly raw: string; readonly accepted: string;
    readonly pooled: string; readonly rejected: string; readonly before: number; readonly after: number; readonly migrated: boolean; readonly legacyBefore: number; readonly legacyAfter: number; readonly poolFull: boolean; }
export class ExperienceGrantModel {
    private role = ''; private highWater = 0n; private firstInventory = true; notice: ExperienceNotice | null = null;
    attach(role: string): void { if (role !== this.role) { this.role = role; this.highWater = 0n; } this.firstInventory = true; this.notice = null; }
    synchronize(revision: Long): void { if (this.firstInventory) { this.firstInventory = false; const next = amount(revision, '角色修订', true); if (next > this.highWater) this.highWater = next; } }
    detach(): void { this.firstInventory = true; this.notice = null; }
    accept(value: any, role: string, rule: Long, curve: string): boolean {
        if (!value || this.firstInventory || role !== this.role) return false;
        try {
            if (!(value.role?.roleId instanceof Uint8Array) || value.role.roleId.length !== 16) return false;
            const incoming = `${amount(value.role.originServerId, '入口服', true)}:${Array.from(value.role.roleId as Uint8Array, byte => byte.toString(16).padStart(2, '0')).join('')}`;
            const revision = amount(value.roleRevision, '角色修订', true), source = amount(value.sourceSeq, '来源序号', true);
            if (incoming !== role || revision <= this.highWater || source === 0n || source > revision || !value.ruleVersion.equals(rule) || value.curveVersion !== curve) return false;
            const state = observedProgression(value.state); if (state.curveVersion !== curve) return false;
            const raw = amount(value.rawExperience, '原经验'), accepted = amount(value.acceptedExperience, '接收经验'), pooled = amount(value.pooledExperience, '入池经验'), rejected = amount(value.rejectedExperience, '未接收经验');
            if (raw !== accepted + rejected || pooled > accepted || !positiveLevel(value.levelBefore) || !positiveLevel(value.levelAfter) || value.levelAfter < value.levelBefore
                || typeof value.migrated !== 'boolean' || !unsigned(value.legacyLevelBefore) || !unsigned(value.legacyLevelAfter)) return false;
            this.highWater = revision;
            this.notice = Object.freeze({ role, revision: revision.toString(), source: source.toString(), raw: raw.toString(), accepted: accepted.toString(), pooled: pooled.toString(), rejected: rejected.toString(),
                before: value.levelBefore, after: value.levelAfter, migrated: value.migrated, legacyBefore: value.legacyLevelBefore, legacyAfter: value.legacyLevelAfter,
                poolFull: state.atCap && !state.pendingExperienceLimit.isZero() && state.pendingExperience.equals(state.pendingExperienceLimit) });
            return true;
        } catch { return false; }
    }
}
