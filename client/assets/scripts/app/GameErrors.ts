import { CodeItemUseBlocked, CodeItemUseCooldown, CodeSkillCooldown, CodeSkillNotLearned, CodeSkillResource, CodeSkillStateBlocked, CodeSkillTarget,
    CodeSkillPreviewStale, CodeSkillMaterials, CodeSkillUpgradeBlocked, CodeSkillBookSource, CodeSkillPurchaseBlocked,
    CodeSkillMaterialRequired, CodeSkillMaterialChanged, CodeLineForbidden, CodeEquipmentBagFull, CodeEquipmentChanged, CodeEquipmentBlocked } from '../generated/protocol_ids';

const SCENE_FORBIDDEN_MESSAGE = '当前场景不允许此操作';

// 同一份协议错误供请求 Promise 和会话订阅使用，不能生成两种用户提示。
export function protocolErrorMessage(messageId: number, errorCode: number): string {
    if (errorCode === CodeLineForbidden) return messageId === 30004 ? '当前场景不允许调整自动战斗' : SCENE_FORBIDDEN_MESSAGE;
    if (errorCode === CodeEquipmentBagFull) return '背包空间不足，装备保持原位';
    if (errorCode === CodeEquipmentChanged) return '原装备或所在部位已变化，请重新选择';
    if (errorCode === CodeEquipmentBlocked) return '当前职业、等级或装备状态不允许穿脱';
    if (errorCode === CodeItemUseBlocked) return '角色倒下或当前状态不能使用药品';
    if (errorCode === CodeItemUseCooldown) return '药品使用过快，请稍后再试';
    if (errorCode === CodeSkillNotLearned) return '尚未学会该技能';
    if (errorCode === CodeSkillStateBlocked) return '角色倒下或当前状态不能施放技能';
    if (errorCode === CodeSkillCooldown) return '技能尚未冷却，请稍后再试';
    if (errorCode === CodeSkillResource) return '魔法不足，不能施放技能';
    if (errorCode === CodeSkillTarget) return '目标已失效、被阻挡或超出技能射程';
    if (errorCode === CodeSkillPreviewStale) return '报价已过期或资产发生变化，请重新预览';
    if (errorCode === CodeSkillMaterials) return '材料不足或原实例数量、版本不匹配，请重新选择';
    if (errorCode === CodeSkillUpgradeBlocked) return '当前等级或技能状态不能升级';
    if (errorCode === CodeSkillBookSource) return '此来源或品质的技能书不能分解';
    if (errorCode === CodeSkillPurchaseBlocked) return '余额、背包或购买条件不满足';
    if (errorCode === CodeSkillMaterialRequired) return '请先装入对应施法护符，并确认剩余数量充足';
    if (errorCode === CodeSkillMaterialChanged) return '原护符已变化，请重新选择后施法或装卸';
    return `操作失败，错误码${errorCode}`;
}

export function normalizeGameErrorMessage(message: string): string {
    const text = message.trim();
    // 旧通用错误没有原消息或地图身份，不能反推为满包。
    if (text === '服务端拒绝：2103' || text === '操作失败，错误码2103') return SCENE_FORBIDDEN_MESSAGE;
    return text;
}
