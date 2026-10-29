import { GridProjection, ProjectionGeometry } from './GridProjection';
import Long from 'long';
import { progressionMeter, ProgressionState } from './LevelGrowthModel';
// HUD 只投影已同步的数据；金额和数量先用整数处理，避免64位值经Number丢失精度。
export type HudInteger = bigint | number | string | { toString(): string };

export interface HudInput {
    playerHandle: number;
    role: { name?: string; job?: number; level?: number } | null;
    wallet: { gold?: HudInteger; boundGold?: HudInteger; yuanbaoUnits?: HudInteger; boundYuanbaoUnits?: HudInteger } | null;
    inventory: { level?: number; experience?: HudInteger; progression?: ProgressionState | null; items?: Array<{ itemId: number; quantity: HudInteger }> } | null;
    baseline: { entities: Array<{ entityHandle: number; entityKind: number; position: { x: number; y: number }; hp: HudInteger; hpMax: HudInteger; mp: HudInteger; mpMax: HudInteger }>; safeZoneId?: string | bigint | { toString(): string } } | null;
    hosting?: unknown;
    report?: unknown;
    ready: boolean;
    lastError?: string;
    auto: boolean;
    count: number;
    quickSkillLabel?: string;
    gameplay: { rule_version?: number; progression?: { curve_version: string }; jobs?: Array<{ job_id: number; name: string }>; items?: Array<{ item_id: number; kind: string }>; experience?: Array<{ level: number; next_level_exp: HudInteger }> } | null;
    map: ({ name?: string; width: number; height: number; scene_kind?: number } & Partial<ProjectionGeometry>) | null;
}

export function hudInteger(value: HudInteger): bigint {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('HUD数值必须使用安全整数或Long');
    const text = value.toString();
    if (!/^-?\d+$/.test(text)) throw new Error('HUD数值必须为整数');
    return BigInt(text);
}

/** fractionDigits=1 用于元宝最小单位；compact只影响主栏展示，详情仍可显示完整精度。 */
export function hudAmount(value: HudInteger | null | undefined, fractionDigits = 0, compact = false): string {
    if (value == null) return '—';
    if (fractionDigits !== 0 && fractionDigits !== 1) throw new Error('HUD金额只支持整数或十分单位');
    const integer = hudInteger(value), negative = integer < 0n;
    const absolute = negative ? -integer : integer;
    const base = fractionDigits === 1 ? 10n : 1n;
    const unit = compact && absolute >= 100000000n * base ? { scale: 100000000n, suffix: '亿' }
        : compact && absolute >= 10000n * base ? { scale: 10000n, suffix: '万' } : null;
    let result: string;
    if (unit) {
        // 只截取展示位，不改变账本数值；小数尾零省略，防止窄栏溢出。
        const tenths = absolute * 10n / (unit.scale * base);
        result = `${tenths / 10n}${tenths % 10n ? `.${tenths % 10n}` : ''}${unit.suffix}`;
    } else {
        result = (absolute / base).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
        if (fractionDigits === 1) result += `.${absolute % 10n}`;
    }
    return `${negative ? '-' : ''}${result}`;
}

export function hudRatio(value: HudInteger | null | undefined, maximum: HudInteger | null | undefined): number {
    if (value == null || maximum == null) return 0;
    const current = hudInteger(value), total = hudInteger(maximum);
    if (current <= 0n || total <= 0n) return 0;
    if (current >= total) return 1;
    return Number(current * 10000n / total) / 10000;
}

export function hudItemCount(input: HudInput, itemId: number): string {
    if (!input.inventory || !input.gameplay?.items?.some(item => item.item_id === itemId && item.kind === 'potion')) return '—';
    const count = (input.inventory.items ?? []).filter(item => item.itemId === itemId)
        .reduce((sum, item) => sum + hudInteger(item.quantity), 0n);
    return hudAmount(count, 0, true);
}

export function miniMapPoint(position: { x: number; y: number } | null, map: ({ width: number; height: number } & Partial<ProjectionGeometry>) | null,
    width: number, height: number, anchorX = .5, anchorY = .5): { x: number; y: number } | null {
    if (!position || !map || ![position.x, position.y, map.width, map.height, width, height, anchorX, anchorY].every(Number.isFinite)
        || map.width <= 0 || map.height <= 0 || width <= 0 || height <= 0) return null;
    const x = Math.max(0, Math.min(map.width - 1, position.x));
    const y = Math.max(0, Math.min(map.height - 1, position.y));
    if (map.projection === 'isometric_diamond') {
        const projection = new GridProjection(map as ProjectionGeometry), point = projection.project(x, y);
        return { x: (point.x / projection.surfaceWidth + .5 - anchorX) * width,
            y: ((point.y - projection.floorCenterY) / projection.surfaceHeight + .5 - anchorY) * height };
    }
    return { x: ((x + .5) / map.width - anchorX) * width, y: (1 - (y + .5) / map.height - anchorY) * height };
}

export function createHudState(input: HudInput) {
    const player = input.playerHandle > 0 ? input.baseline?.entities.find(entity => entity.entityHandle === input.playerHandle) : undefined;
    const level = input.inventory?.level ?? input.role?.level;
    const next = input.gameplay?.experience?.find(row => row.level === level)?.next_level_exp;
    const experience = input.inventory?.experience;
    const publishedCurve = input.gameplay?.progression?.curve_version;
    const growth = (input.gameplay?.rule_version ?? 0) >= 20 ? progressionMeter(level, Long.isLong(experience) ? experience : undefined,
        typeof publishedCurve === 'string' && input.inventory?.progression?.curveVersion === publishedCurve ? input.inventory.progression : null) : null;
    const dead = player != null && hudInteger(player.hp) <= 0n;
    const town = input.map?.scene_kind === 2;
    const inSafeZone = input.baseline?.safeZoneId != null && hudInteger(input.baseline.safeZoneId) > 0n;
    return {
        roleName: input.role?.name ?? '角色同步中',
        level: level == null ? '等级 —' : `${level}级`,
        job: input.gameplay?.jobs?.find(row => row.job_id === input.role?.job)?.name ?? '职业 —',
        gold: `绑金 ${hudAmount(input.wallet?.boundGold, 0, true)}`,
        yuanbao: `绑元 ${hudAmount(input.wallet?.boundYuanbaoUnits, 1, true)}`,
        walletDetails: `金币 ${hudAmount(input.wallet?.gold)}\n绑定金币 ${hudAmount(input.wallet?.boundGold)}\n元宝 ${hudAmount(input.wallet?.yuanbaoUnits, 1)}\n绑定元宝 ${hudAmount(input.wallet?.boundYuanbaoUnits, 1)}`,
        position: player ? `(${player.position.x},${player.position.y})` : '(—,—)',
        mapName: input.map?.name ?? '地图同步中',
        hpText: `${hudAmount(player?.hp)}/${hudAmount(player?.hpMax)}`,
        mpText: `${hudAmount(player?.mp)}/${hudAmount(player?.mpMax)}`,
        hp: hudRatio(player?.hp, player?.hpMax), mp: hudRatio(player?.mp, player?.mpMax),
        experienceText: growth?.text ?? (next == null ? `经验 ${hudAmount(experience)}` : `${hudAmount(experience)} / ${hudAmount(next)}`),
        experience: growth?.ratio ?? hudRatio(experience, next),
        redPotionCount: hudItemCount(input, 35), bluePotionCount: hudItemCount(input, 36),
        objective: !input.ready ? '场景同步中' : dead ? '角色已倒下 · 点击复活' : town ? `${inSafeZone ? '安全区 · ' : ''}城镇漫步` : `${input.auto ? '自动战斗' : '手动战斗'} · ${input.count}只怪物`,
        auto: input.auto ? '自动' : '手动', count: `${input.count}怪`, attack: dead ? '复活' : '攻击',
        playerPosition: player?.position ?? null,
    };
}
