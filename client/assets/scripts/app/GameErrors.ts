import { CodeItemUseBlocked, CodeItemUseCooldown } from '../generated/protocol_ids';

const FULL_BAG_AUTO_MESSAGE = '背包已满，整理后再开启自动战斗';

// 同一份协议错误供请求 Promise 和会话订阅使用，不能生成两种用户提示。
export function protocolErrorMessage(messageId: number, errorCode: number): string {
    if (messageId === 30004 && errorCode === 2103) return FULL_BAG_AUTO_MESSAGE;
    if (errorCode === CodeItemUseBlocked) return '角色倒下或当前状态不能使用药品';
    if (errorCode === CodeItemUseCooldown) return '药品使用过快，请稍后再试';
    return `操作失败，错误码${errorCode}`;
}

export function normalizeGameErrorMessage(message: string): string {
    const text = message.trim();
    // 兼容当前会话中的旧错误文案；现有协议的 2103 仅用于满包拒绝自动战斗。
    if (text === '服务端拒绝：2103' || text === '操作失败，错误码2103') return FULL_BAG_AUTO_MESSAGE;
    return text;
}
