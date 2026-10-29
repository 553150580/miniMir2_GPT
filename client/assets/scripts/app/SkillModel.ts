import { PotionInteger, PotionItem } from './PotionModel';
import { OwnedPetState, ownedPetCanRecall } from './OwnedPetModel';

export interface SkillDefinition {
    id: number; name: string; job_id: number; book_item_id: number; learn_level: number; initial_level: number;
    kind: 'fireball' | 'healing' | 'fencing' | 'spirit_sword' | 'great_fireball' | 'thunderbolt' | 'firebang' | 'mass_healing'
        | 'slaying' | 'thrusting' | 'half_moon' | 'flaming_sword' | 'hell_fire' | 'lightning' | 'thunder_storm' | 'ice_storm'
        | 'magic_shield' | 'soul_shield' | 'blessed_armour' | 'repulsion' | 'shoulder_dash' | 'teleport' | 'summon_pet'; active: boolean; range: number;
    displacement?: { distance_base_by_level: readonly number[]; distance_random_bound: number; step_display_ms: number };
    activation_kind?: 'passive' | 'passive_proc' | 'cast' | 'toggle' | 'charge';
    target_kind?: 'passive' | 'enemy_entity' | 'enemy_grid' | 'friendly_entity' | 'friendly_grid' | 'friendly_self' | 'attack_mode' | 'attack_charge' | 'enemy_direction' | 'enemy_self';
    buff_id?: number; material_item_id?: number; material_quantity?: number;
    area_radius?: number; require_line_of_sight?: boolean; accuracy_by_level?: readonly number[]; max_attack_bonus_by_level?: readonly number[];
    secondary_direction_offsets?: readonly number[]; secondary_distance_cells?: number; line_cells?: number; line_step_ms?: number; charge_window_ms?: number;
    base_cost: number; level_cost: number; cast_ms: number; impact_base_ms: number; projectile_ms_per_cell: number; accuracy_per_level: number;
}
export interface SkillInventory {
    revision: PotionInteger; level: number; items: readonly PotionItem[];
    learnedSkills?: readonly { skillId: number; level: number }[];
}
export interface SkillSnapshot {
    ready: boolean; roleIdentity: string; job: number; sceneEpoch: PotionInteger; ruleVersion: PotionInteger;
    inventory: SkillInventory | null; definitions: readonly SkillDefinition[]; modes?: SkillModeState | null; modesReceivedAt?: number;
    castingSupply?: { name: string; quantity: string } | null;
    castingSupplyEnabled?: boolean;
    playerStatuses?: readonly { teleportManaPenaltyPercent?: number }[];
    mapNoTeleport?: boolean;
    mapNoPets?: boolean;
    ownedPet?: OwnedPetState | null;
    statusDefinitions?: readonly { id: number; duration_rule?: { kind?: string; duration_ms?: number; stat_multiplier: number; seconds_by_level: readonly number[] };
        attribute_effects: { value_by_level: readonly number[]; target_level_divisor?: number; target_level_add?: number } }[];
}
export interface SkillModeEntry { skillId: number; enabled: boolean; ready: boolean; remainingMs: number; cooldownRemainingMs: number; }
export interface SkillModeState { sceneEpoch: PotionInteger; revision: PotionInteger; playerLife: PotionInteger; serverTimeUnixMs: PotionInteger; states: readonly SkillModeEntry[]; }
export function acceptsSkillModeState(current: SkillModeState | null, incoming: SkillModeState, scene: PotionInteger): boolean {
    try {
        if (!incoming || incoming.sceneEpoch.toString() !== scene.toString() || !Array.isArray(incoming.states) || incoming.states.length > 64
            || integer(incoming.playerLife) < 0n || integer(incoming.serverTimeUnixMs) < 1n) return false;
        if (current?.sceneEpoch.toString() === scene.toString() && integer(incoming.revision) <= integer(current.revision)) return false;
        integer(incoming.revision);
        const ids = new Set<number>();
        for (const state of incoming.states) {
            if (!Number.isSafeInteger(state.skillId) || state.skillId < 1 || ids.has(state.skillId) || typeof state.enabled !== 'boolean' || typeof state.ready !== 'boolean'
                || ![state.remainingMs, state.cooldownRemainingMs].every(value => Number.isSafeInteger(value) && value >= 0 && value <= 0xffffffff)) return false;
            ids.add(state.skillId);
        }
        return true;
    } catch { return false; }
}
export interface SkillRow {
    definition: SkillDefinition; learned: boolean; level: number; canLearn: boolean; cost: number; status: string;
}
export interface SkillEntity {
    entityHandle: number; entityKind: number; hp: PotionInteger; mp?: PotionInteger; isPetCorpse?: boolean;
    position: { x: number; y: number }; direction: number;
}
export interface SkillLearnPreview {
    readonly skillId: number; readonly roleIdentity: string; readonly sceneEpoch: string; readonly ruleVersion: string;
    readonly revision: string; readonly instanceHex: string; readonly itemVersion: string; readonly quantity: string;
    readonly message: string;
}

function integer(value: PotionInteger): bigint {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('技能状态数值必须保持整数精度');
    const text = value.toString(); if (!/^(?:0|[1-9]\d*)$/.test(text)) throw new Error('技能状态数值无效');
    return BigInt(text);
}
function hex(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('技能书实例标识无效');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
export function skillCost(definition: SkillDefinition, level: number, teleportPenaltyPercent = 0): number {
    const result = definition.base_cost + definition.level_cost * level;
    if (!Number.isSafeInteger(level) || level < 0 || !Number.isSafeInteger(result) || result < 0) throw new Error('技能消耗配置无效');
    if (definition.kind !== 'teleport') return result;
    if (!Number.isSafeInteger(teleportPenaltyPercent) || teleportPenaltyPercent < 0 || teleportPenaltyPercent > 0xffffffff) throw new Error('传送耗蓝状态无效');
    const cost = BigInt(result) + BigInt(result) * BigInt(teleportPenaltyPercent) / 100n;
    if (cost > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('传送耗蓝超出精确范围');
    return Number(cost);
}
export function skillAccuracy(row: SkillRow): number {
    if (!row.learned) return 0;
    const result = row.definition.accuracy_by_level ? row.definition.accuracy_by_level[row.level]
        : row.definition.kind === 'spirit_sword' ? NaN : row.level * row.definition.accuracy_per_level;
    if (!Number.isSafeInteger(result) || result < 0) throw new Error('技能准确加成配置无效');
    return result;
}
export function skillRows(snapshot: SkillSnapshot): SkillRow[] {
    const teleportPenalty = Math.max(0, ...(snapshot.playerStatuses ?? []).map(status => status.teleportManaPenaltyPercent ?? 0));
    return snapshot.definitions.filter(definition => definition.job_id === snapshot.job).map(definition => {
        const learned = snapshot.inventory?.learnedSkills?.find(skill => skill.skillId === definition.id);
        const hasBook = snapshot.inventory?.items.some(item => item.itemId === definition.book_item_id && integer(item.quantity) > 0n) ?? false;
        const eligible = (snapshot.inventory?.level ?? 0) >= definition.learn_level;
        return { definition, learned: learned !== undefined, level: learned?.level ?? definition.initial_level,
            cost: skillCost(definition, learned?.level ?? definition.initial_level, teleportPenalty),
            canLearn: snapshot.ready && !learned && eligible && hasBook,
            status: learned ? learned.level === 0 ? '已学 · 初学（等级0）' : `已学 · 等级${learned.level}`
                : !eligible ? `${definition.learn_level}级可学习` : hasBook ? '拥有技能书 · 可学习' : '缺少对应技能书' };
    }).sort((a, b) => a.definition.id - b.definition.id);
}
export function primarySkill(snapshot: SkillSnapshot, preferredId = 0): SkillRow | null {
    const rows = skillRows(snapshot);
    return rows.find(row => row.definition.id === preferredId && row.definition.active && row.learned)
        ?? rows.find(row => row.definition.active && row.learned) ?? rows.find(row => row.definition.active) ?? null;
}
export function skillTargetKind(definition: SkillDefinition): NonNullable<SkillDefinition['target_kind']> {
    if (definition.target_kind) return definition.target_kind;
    if (definition.kind === 'healing') return 'friendly_entity';
    if (definition.kind === 'fencing' || definition.kind === 'spirit_sword') return 'passive';
    if (definition.kind === 'fireball') return 'enemy_entity';
    throw new Error('技能缺少明确的目标规则');
}
export function isGroundSkill(definition: SkillDefinition): boolean { return ['enemy_grid', 'friendly_grid'].includes(skillTargetKind(definition)); }
export function skillActivation(definition: SkillDefinition): NonNullable<SkillDefinition['activation_kind']> {
    return definition.activation_kind ?? (definition.active ? 'cast' : 'passive');
}
export function isDirectionalSkill(definition: SkillDefinition): boolean { return skillTargetKind(definition) === 'enemy_direction'; }
export function isAreaEffect(definition: SkillDefinition): boolean { return isGroundSkill(definition) || ['enemy_direction', 'enemy_self'].includes(skillTargetKind(definition)); }
export function skillMode(snapshot: SkillSnapshot, skillId: number): SkillModeEntry | undefined {
    return snapshot.modes?.sceneEpoch.toString() === snapshot.sceneEpoch.toString() ? snapshot.modes.states.find(value => value.skillId === skillId) : undefined;
}
export function skillModeStatus(snapshot: SkillSnapshot, definition: SkillDefinition, now = Date.now()): string {
    const activation = skillActivation(definition), state = skillMode(snapshot, definition.id);
    if (activation !== 'toggle' && activation !== 'charge' && activation !== 'passive_proc') return '';
    if (!state) return '等待战斗状态';
    if (activation === 'toggle') return state.enabled ? '已开启' : '已关闭';
    const elapsed = Math.max(0, now - (snapshot.modesReceivedAt ?? now));
    if (state.ready) return state.remainingMs > 0 ? `已就绪 · 剩余${Math.max(0, Math.ceil((state.remainingMs - elapsed) / 1000))}秒` : '下一击已就绪';
    const cooldown = Math.max(0, Math.ceil((state.cooldownRemainingMs - elapsed) / 1000));
    return cooldown ? `冷却 ${cooldown}秒` : activation === 'charge' ? '可充能' : '攻击时有机会准备下一击';
}
export function isSummonRecall(snapshot: SkillSnapshot, row: SkillRow): boolean {
    return row.definition.kind === 'summon_pet' && ownedPetCanRecall(snapshot.ownedPet, row.definition.id);
}
export function needsCastingSupply(snapshot: SkillSnapshot, row: SkillRow): boolean {
    return !!row.definition.material_quantity && !isSummonRecall(snapshot, row);
}
export function skillActionLabel(snapshot: SkillSnapshot, row: SkillRow): string {
    if (row.definition.kind === 'summon_pet' && snapshot.mapNoPets) return '此图禁召';
    if (isSummonRecall(snapshot, row)) return '召回';
    if (needsCastingSupply(snapshot, row) && snapshot.castingSupplyEnabled === false) return '护符未开放';
    const activation = skillActivation(row.definition), state = skillMode(snapshot, row.definition.id);
    if (activation === 'toggle') return state?.enabled ? '关闭' : '开启';
    if (activation === 'charge') return state?.ready ? '已就绪' : '充能';
    return !row.definition.active ? '被动技能' : '施放';
}
export function skillPassiveText(row: SkillRow): string {
    const bonus = row.definition.kind === 'slaying' ? row.learned ? row.definition.max_attack_bonus_by_level?.[row.level] : 0 : undefined;
    return `当前准确 +${skillAccuracy(row)}${bonus !== undefined ? ` · 攻击上限 +${bonus}` : ''}`;
}
// 快捷槽原字号与单行宽度固定，完整名称仍由技能列表及瞄准提示展示。
export function skillQuickLabel(definition: SkillDefinition): string {
    return ({ summon_pet: '召唤', teleport: '瞬移', firebang: '爆裂', mass_healing: '群疗', slaying: '攻杀', thrusting: '刺杀', half_moon: '半月', flaming_sword: '烈火', hell_fire: '地狱火', lightning: '疾光', thunder_storm: '雷光', magic_shield: '魔法盾', soul_shield: '幽灵盾', blessed_armour: '战甲' } as Record<string, string>)[definition.kind] ?? definition.name;
}
export function skillQuickStateLabel(snapshot: SkillSnapshot, row: SkillRow): string {
    if (row.definition.kind === 'summon_pet' && snapshot.mapNoPets) return '禁召';
    if (needsCastingSupply(snapshot, row) && snapshot.castingSupplyEnabled === false) return '未开放';
    const label = skillQuickLabel(row.definition), activation = skillActivation(row.definition), state = skillMode(snapshot, row.definition.id);
    return activation === 'toggle' && state ? `${label}${state.enabled ? '开' : '关'}` : activation === 'charge' && state?.ready ? `${label}备` : label;
}
export function skillDescriptionLines(row: SkillRow, snapshot: SkillSnapshot): string[] {
    const definition = row.definition, lines = [row.status, `魔法消耗 ${row.cost}`];
    if (definition.displacement) {
        const rules=definition.displacement, low=rules.distance_base_by_level[row.level], high=low+rules.distance_random_bound-1;
        if (definition.kind==='repulsion') lines.push('尝试推开周围等级较低的敌人', `本阶推开距离 ${low}–${high}格`, '不会对怪物造成伤害');
        else lines.push('轻触地图选择冲撞方向', `本阶冲撞距离 ${low}–${high}格`, '只推动首格的低等级敌人，结束时造成一次伤害', '冲撞受阻仍消耗魔法与冷却');
    }
    const status = snapshot.statusDefinitions?.find(value => value.id === definition.buff_id), reduction = status?.attribute_effects.value_by_level[row.level];
    if (definition.kind === 'magic_shield') lines.push('仅对自己施放', reduction === undefined ? '获得伤害减免，实际数值见当前状态' : `当前等级伤害减免 ${reduction}%`, '持续时间受魔法属性与技能等级影响',
        '受到实际伤害会缩短持续时间', '已有魔法盾时重施仍消耗魔法，但不刷新', '角色倒下后魔法盾解除');
    else if (definition.kind === 'soul_shield' || definition.kind === 'blessed_armour') lines.push(`轻触地面选择中心，友方范围半径${definition.area_radius ?? 0}格`,
        definition.kind === 'soul_shield' ? '提高受益者的魔法防御上限' : '提高受益者的物理防御上限', '加成取决于受益者等级，实际值见状态',
        '持续时间受施法者道术与技能等级影响', '同类状态重施刷新持续时间', '友方倒下时仍可受益，持续时间继续推进', '无主宠物尸体不属于友方');
    else if (!definition.active) lines.push(skillPassiveText(row));
    else if (!definition.displacement && definition.kind !== 'teleport' && definition.kind !== 'summon_pet') lines.push(definition.target_kind === 'enemy_self' ? `自身周围${definition.area_radius ?? 0}格` : isDirectionalSkill(definition) ? `前方${definition.line_cells ?? 0}格` : `射程 ${definition.range}格`);
    if (definition.kind === 'summon_pet') {
        lines.push(isSummonRecall(snapshot, row) ? '当前可召回原宠，不消耗护符' : '新召唤由服务器选择出生位置', '召回仍消耗魔法并受动作与冷却限制');
        if (snapshot.mapNoPets) lines.push('当前地图禁止召唤');
    }
    if (definition.kind === 'teleport') {
        lines.push('无需选择目标位置，随机移动至绑定地图内', '施法生效时检查当前地图，失败不返还已消耗魔法');
        if (snapshot.mapNoTeleport) lines.push('当前地图禁止传送');
        if (status?.duration_rule?.kind === 'fixed_ms') lines.push(`成功后获得${Math.ceil((status.duration_rule.duration_ms ?? 0) / 1000)}秒传送耗蓝状态`);
        if (reduction !== undefined) lines.push(`状态期间仅传送技能的魔法消耗增加${reduction}%`, '再次成功刷新持续时间');
    }
    if (status?.attribute_effects.target_level_divisor) lines.push(`防御上限加成＝受益者等级÷${status.attribute_effects.target_level_divisor}取整＋${status.attribute_effects.target_level_add ?? 0}`);
    if ((definition.kind === 'soul_shield' || definition.kind === 'blessed_armour') && status?.duration_rule?.stat_multiplier)
        lines.push(`本阶持续时间＝施放时道术值×${status.duration_rule.stat_multiplier}＋${status.duration_rule.seconds_by_level[row.level]}秒`);
    if (definition.material_quantity) lines.push(`${definition.kind === 'summon_pet' ? '新召唤' : '每次'}消耗已装入护符 ${definition.material_quantity}份`, snapshot.castingSupplyEnabled === false ? '施法护符暂未开放' : snapshot.castingSupply
        ? `当前护符：${snapshot.castingSupply.name} · 余${snapshot.castingSupply.quantity}` : '尚未装入施法护符，请在背包中选择装入');
    lines.push(`${definition.learn_level}级可使用对应技能书学习`, '升级需选择书本与书页，先查看报价');
    return lines;
}
export function createSkillLearnPreview(snapshot: SkillSnapshot, skillId: number): SkillLearnPreview {
    if (!snapshot.ready || !snapshot.inventory || !snapshot.roleIdentity) throw new Error('角色与背包尚未同步');
    const row = skillRows(snapshot).find(value => value.definition.id === skillId);
    if (!row) throw new Error('当前职业不能学习此技能');
    if (row.learned) throw new Error('已经学会此技能');
    if (snapshot.inventory.level < row.definition.learn_level) throw new Error(`需要达到${row.definition.learn_level}级`);
    const book = snapshot.inventory.items.find(item => item.itemId === row.definition.book_item_id && integer(item.quantity) > 0n);
    if (!book) throw new Error('背包中没有对应技能书');
    return Object.freeze({ skillId, roleIdentity: snapshot.roleIdentity, sceneEpoch: integer(snapshot.sceneEpoch).toString(),
        ruleVersion: integer(snapshot.ruleVersion).toString(), revision: integer(snapshot.inventory.revision).toString(),
        instanceHex: hex(book.instanceId), itemVersion: integer(book.version).toString(), quantity: integer(book.quantity).toString(),
        message: `学习${row.definition.name}\n消耗1本对应技能书\n${book.bound ? '绑定' : '非绑定'}技能书，所选数量${integer(book.quantity)}\n学会后为初学（等级0）\n确认学习？` });
}
export function validateSkillLearnPreview(preview: SkillLearnPreview, snapshot: SkillSnapshot): void {
    const changed = '技能或背包状态已变化，请重新预览';
    if (!snapshot.ready || !snapshot.inventory || snapshot.roleIdentity !== preview.roleIdentity
        || integer(snapshot.sceneEpoch).toString() !== preview.sceneEpoch || integer(snapshot.ruleVersion).toString() !== preview.ruleVersion
        || integer(snapshot.inventory.revision).toString() !== preview.revision) throw new Error(changed);
    const row = skillRows(snapshot).find(value => value.definition.id === preview.skillId);
    const book = snapshot.inventory.items.find(item => hex(item.instanceId) === preview.instanceHex);
    if (!row || row.learned || snapshot.inventory.level < row.definition.learn_level || !book || book.itemId !== row.definition.book_item_id
        || integer(book.quantity).toString() !== preview.quantity || integer(book.version).toString() !== preview.itemVersion) throw new Error(changed);
}
export function skillBookInstance(preview: SkillLearnPreview): Uint8Array {
    return Uint8Array.from({ length: 16 }, (_value, index) => parseInt(preview.instanceHex.slice(index * 2, index * 2 + 2), 16));
}
export function gridDirection(origin: { x: number; y: number }, target: { x: number; y: number }, fallback = 4, count = 8): number {
    const dx = target.x - origin.x, dy = target.y - origin.y;
    if (!dx && !dy) return fallback;
    return (Math.round(Math.atan2(dx, -dy) * count / (2 * Math.PI)) + count) % count;
}
export function selectSkillTarget(row: SkillRow, entities: readonly SkillEntity[], selectedHandle: number | null, playerHandle: number): SkillEntity {
    const player = validateCaster(row, entities, playerHandle), targetKind = skillTargetKind(row.definition);
    if (targetKind !== 'enemy_entity' && targetKind !== 'friendly_entity') throw new Error('该技能需要选择地面位置');
    const friendly = targetKind === 'friendly_entity';
    const distance = (target: SkillEntity): number => Math.max(Math.abs(target.position.x - player.position.x), Math.abs(target.position.y - player.position.y));
    const legal = (target: SkillEntity): boolean => !target.isPetCorpse && integer(target.hp) > 0n
        && (friendly ? target.entityHandle === playerHandle || target.entityKind === 2 : target.entityKind === 3);
    const selected = entities.find(entity => entity.entityHandle === selectedHandle);
    if (selected && (selected.isPetCorpse || integer(selected.hp) === 0n) && (friendly
        ? selected.entityHandle === playerHandle || selected.entityKind === 2 : selected.entityKind === 3)) throw new Error('所选目标已倒下，请重新选择');
    let target = selected && legal(selected) ? selected : undefined;
    if (!target && friendly) target = player;
    if (!target) target = entities.filter(legal).slice().sort((a, b) => distance(a) - distance(b) || a.entityHandle - b.entityHandle)[0];
    if (!target) throw new Error('没有可施放的目标');
    if (distance(target) > row.definition.range) throw new Error('目标超出技能射程，请靠近后施放');
    return target;
}
export function validateGroundTarget(row: SkillRow, entities: readonly SkillEntity[], grid: { x: number; y: number }, bounds: { width: number; height: number }, playerHandle: number): { x: number; y: number } {
    const player = validateCaster(row, entities, playerHandle);
    if (!isGroundSkill(row.definition)) throw new Error('该技能需要选择实体目标');
    // 范围中心允许阻挡格；命中格逐格是否可作用由服务端裁定，不复用移动可行走条件。
    if (![bounds.width, bounds.height].every(value => Number.isSafeInteger(value) && value > 0)
        || ![grid.x, grid.y].every(value => Number.isSafeInteger(value) && value >= 0)
        || grid.x >= bounds.width || grid.y >= bounds.height) throw new Error('地面目标无效或超出地图边界');
    if (Math.max(Math.abs(grid.x - player.position.x), Math.abs(grid.y - player.position.y)) > row.definition.range) throw new Error('目标超出技能射程，请重新选择地面');
    return { x: grid.x, y: grid.y };
}
export function validateDirectionTarget(row: SkillRow, entities: readonly SkillEntity[], grid: { x: number; y: number }, bounds: { width: number; height: number }, playerHandle: number): { x: number; y: number; direction: number } {
    const player = validateCaster(row, entities, playerHandle);
    if (!isDirectionalSkill(row.definition)) throw new Error('该技能无需方向瞄准');
    if (![grid.x, grid.y].every(value => Number.isSafeInteger(value) && value >= 0) || grid.x >= bounds.width || grid.y >= bounds.height) throw new Error('方向目标超出地图边界');
    if (grid.x === player.position.x && grid.y === player.position.y) throw new Error('请轻触角色周围选择方向');
    return { x: grid.x, y: grid.y, direction: gridDirection(player.position, grid, player.direction) };
}
export function validateCaster(row: SkillRow, entities: readonly SkillEntity[], playerHandle: number, checkCost = true): SkillEntity {
    if (!row.learned) throw new Error('尚未学会该技能');
    if (!row.definition.active) throw new Error('该技能为被动技能，无需主动施放');
    const player = entities.find(entity => entity.entityHandle === playerHandle);
    if (!player || integer(player.hp) === 0n) throw new Error('角色已倒下，不能施放技能');
    if (checkCost && integer(player.mp ?? 0) < BigInt(row.cost)) throw new Error('魔法不足');
    return player;
}
