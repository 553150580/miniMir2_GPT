import { CodeItemUseBlocked, CodeItemUseCooldown, CodeSkillCooldown, CodeSkillNotLearned, CodeSkillResource, CodeSkillStateBlocked, CodeSkillTarget,
    CodeSkillPreviewStale, CodeSkillMaterials, CodeSkillUpgradeBlocked, CodeSkillBookSource, CodeSkillPurchaseBlocked,
    CodeSkillMaterialRequired, CodeSkillMaterialChanged, CodeLineForbidden, CodeEquipmentBagFull, CodeEquipmentChanged, CodeEquipmentBlocked,
    CodeWarehouseFull, CodeWarehouseChanged, CodeWarehouseBlocked, CodeWarehousePrivilegeRequired,
    CodeVendorUnavailable, CodeVendorQuoteStale, CodeVendorFunds, CodeVendorItemBlocked,
    CodeTaskCodexUnavailable, CodeTaskCodexDayChanged,
    CodeRoleSlotOrNameOccupied, CodeRouteChanged, CodeServerBusy, CodeUnauthenticated } from '../generated/protocol_ids';

const SCENE_FORBIDDEN_MESSAGE = '当前场景不允许此操作';
export const ACCOUNT_AUTHENTICATION_MESSAGE = '登录凭证已失效，请重新登录。';

export function isRoleAccountRejection(messageId: number, errorCode: number): boolean {
    return errorCode === CodeUnauthenticated && (messageId === 10004 || messageId === 10006 || messageId === 10008 || messageId === 10012);
}

// 同一份协议错误供请求 Promise 和会话订阅使用，不能生成两种用户提示。
export function protocolErrorMessage(messageId: number, errorCode: number): string {
    if (isRoleAccountRejection(messageId, errorCode)) return ACCOUNT_AUTHENTICATION_MESSAGE;
    if (errorCode === CodeUnauthenticated) {
        if (messageId === 10002) return '账号或密码不符，请重新输入。';
        if (messageId === 20002) return '入场票据已失效，请重试进入，或重新登录。';
        return '当前连接认证已失效，请重新连接。';
    }
    if (errorCode === CodeRouteChanged && messageId === 20002) return '游戏线路已更新，请重试进入，或重新登录。';
    if (errorCode === CodeServerBusy) return '服务暂时繁忙，请稍后重试。';
    if (errorCode === CodeRoleSlotOrNameOccupied) return '角色槽位或名称已被占用，请查看角色列表或更换名称后重试';
    if (errorCode === CodeLineForbidden) return messageId === 30004 ? '当前场景不允许调整自动战斗' : SCENE_FORBIDDEN_MESSAGE;
    if (errorCode === CodeWarehouseFull) return '背包或仓库没有空位，原物品保持原处';
    if (errorCode === CodeWarehouseChanged) return '原仓库、物品或版本已变化，请重新查询';
    if (errorCode === CodeWarehouseBlocked) return '当前物品状态或仓库条件不允许此操作';
    if (errorCode === CodeWarehousePrivilegeRequired) return '本角色特权已失效，不能查看或存取共享仓库';
    if (errorCode === CodeVendorUnavailable) return '请靠近当前城镇的对应商人';
    if (errorCode === CodeVendorQuoteStale) return '商店报价已失效，请重新报价';
    if (errorCode === CodeVendorFunds) return '当前钱包余额不足';
    if (errorCode === CodeVendorItemBlocked) return '物品或数量不满足当前商店服务条件';
    if (errorCode === CodeTaskCodexUnavailable) return '当前奖励不可领取，请查看任务进度';
    if (errorCode === CodeTaskCodexDayChanged) return '运营日已切换，请刷新任务';
    if (errorCode === CodeEquipmentBagFull && messageId === 40030) return '背包已满，奖励仍在待领取';
    if (errorCode === CodeEquipmentBagFull && messageId === 20012) return '背包或待领取队列已满，请查看个人首领面板';
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
