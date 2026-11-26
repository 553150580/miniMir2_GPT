import Long from 'long';

export interface RoleCardProduct {
    readonly productId: number; // 唯一配置商品编号，客户端只原样回传。
    readonly kind: number; // 服务端区分特权月卡与托管月卡的类型。
    readonly days: number; // 一次购买覆盖的天数。
    readonly priceYuanbaoUnits: Long; // 非绑定元宝定点价格，单位为十分之一元宝。
}

export interface RoleCardQuote {
    readonly quoteId: Uint8Array; // 本次购买必须提交的原报价编号。
    readonly product: RoleCardProduct; // 服务端当时确认的商品和价格。
    readonly ruleVersion: Long; // 报价锁定的玩法规则版本。
    readonly configHash: string; // 报价采用的配置摘要。
    readonly expiresAtUnixMs: Long; // 报价受理截止时间。
    readonly previousUntilUnixMs: Long; // 确认报价时此卡的原有效期。
}

export interface RoleCardState {
    readonly sceneEpoch: Long; // 签发目录或报价时的角色场景代次。
    readonly products: readonly RoleCardProduct[]; // 当前可选元宝购买的月卡。
    readonly quote?: RoleCardQuote | null; // 指定商品查询返回的原报价。
    readonly privilegeUntilUnixMs: Long; // 特权卡有效期，未购买为零。
    readonly hostingUntilUnixMs: Long; // 托管卡有效期，未购买为零。
    readonly operatingDay: string; // 服务端采用的上海运营日。
    readonly dailyPrivilegeUnits: Long; // 当前规则配置的绑定元宝日领奖额。
    readonly privilegeClaimable: boolean; // 本角色当天能否领取特权奖励。
    readonly errorCode: number; // 原请求的业务状态。
    readonly roleRevision: Long; // 服务端返回该状态时的角色资产修订。
}

export interface RoleCardBuyRequest { readonly quoteId: Uint8Array; readonly productId: number; }
export interface RoleCardDailyRequest { readonly operatingDay: string; }

export interface RoleCardPurchaseReceipt {
    readonly quote: RoleCardQuote; // 原购卡报价，重放必须一致。
    readonly effectiveAtUnixMs: Long; // 原成交生效时间。
    readonly newUntilUnixMs: Long; // 确认后的卡有效期。
    readonly yuanbaoBalanceUnits: Long; // 原成交后的非绑定元宝余额。
}

export interface RoleCardDailyReceipt {
    readonly operatingDay: string; // 实际领取的原上海运营日。
    readonly grantedUnits: Long; // 原领取到账的绑定元宝数。
    readonly boundYuanbaoBalanceUnits: Long; // 原领取后的绑定元宝余额。
    readonly acceptedAtUnixMs: Long; // 服务端首次确认领取的时间。
}

// 读取原始协议的精确字段，拒绝对金额和时点做 Number 隐式转换。
function integer(value: unknown, unsigned = false): value is Long {
    return Long.isLong(value) && value.unsigned === unsigned && !value.isNegative();
}

function positive(value: unknown, unsigned = false): value is Long {
    return integer(value, unsigned) && !value.isZero();
}

function quoteIdentity(value: unknown): value is Uint8Array {
    return value instanceof Uint8Array && value.length === 16 && value.some(byte => byte !== 0);
}

function product(value: unknown): value is RoleCardProduct {
    if (!value || typeof value !== 'object') return false;
    const item = value as Partial<RoleCardProduct>;
    return Number.isInteger(item.productId) && item.productId! > 0 && item.productId! <= 0xffffffff &&
        (item.kind === 1 || item.kind === 2) && item.days === 30 && positive(item.priceYuanbaoUnits);
}

function quote(value: unknown): value is RoleCardQuote {
    if (!value || typeof value !== 'object') return false;
    const item = value as Partial<RoleCardQuote>;
    return quoteIdentity(item.quoteId) && product(item.product) && positive(item.ruleVersion, true) &&
        typeof item.configHash === 'string' && /^[0-9a-f]{64}$/.test(item.configHash) &&
        positive(item.expiresAtUnixMs) && integer(item.previousUntilUnixMs);
}

function operatingDay(value: unknown): value is string {
    return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value);
}

// validateRoleCardState 确认当前场景对应的目录，并核实报价与目录中的商品完全一致。
export function validateRoleCardState(value: unknown, sceneEpoch: Long, productId: number): RoleCardState {
    const result = value as Partial<RoleCardState> | null;
    if (!result || !positive(result.sceneEpoch, true) || !result.sceneEpoch.equals(sceneEpoch) ||
        !positive(result.roleRevision, true) || !integer(result.privilegeUntilUnixMs) || !integer(result.hostingUntilUnixMs) ||
        !operatingDay(result.operatingDay) || !integer(result.dailyPrivilegeUnits) || typeof result.privilegeClaimable !== 'boolean' ||
        !Array.isArray(result.products) || result.products.length > 2 || !result.products.every(product) ||
        new Set(result.products.map(item => item.productId)).size !== result.products.length ||
        new Set(result.products.map(item => item.kind)).size !== result.products.length) throw new Error('角色月卡状态未同步，请刷新');
    if (productId) {
        const selected = result.products.find(item => item.productId === productId);
        const priced = result.quote;
        if (!selected || !quote(priced) || priced.product.productId !== productId || priced.product.kind !== selected.kind ||
            priced.product.days !== selected.days || !priced.product.priceYuanbaoUnits.equals(selected.priceYuanbaoUnits)) {
            throw new Error('角色月卡报价与当前目录不一致，请重新报价');
        }
    } else if (result.quote) throw new Error('角色月卡目录收到意外报价，请刷新');
    return result as RoleCardState;
}

// validateRoleCardPurchaseReceipt 在移除本地未决操作前核对原商品和原报价，重连重放沿用首次余额。
export function validateRoleCardPurchaseReceipt(value: unknown, request: RoleCardBuyRequest): RoleCardPurchaseReceipt {
    const result = value as Partial<RoleCardPurchaseReceipt> | null;
    if (!result || !quote(result.quote) || result.quote.product.productId !== request.productId ||
        !quoteIdentity(request.quoteId) || !result.quote.quoteId.every((byte, index) => byte === request.quoteId[index]) ||
        !positive(result.effectiveAtUnixMs) || !positive(result.newUntilUnixMs) || !integer(result.yuanbaoBalanceUnits) ||
        !result.newUntilUnixMs.greaterThan(result.effectiveAtUnixMs) || !result.newUntilUnixMs.greaterThan(result.quote.previousUntilUnixMs)) {
        throw new Error('购卡回执未确认原报价，请重新连接后沿原操作核对');
    }
    return result as RoleCardPurchaseReceipt;
}

// validateRoleCardDailyReceipt 在移除本地未决操作前核对实际领取的是原运营日。
export function validateRoleCardDailyReceipt(value: unknown, request: RoleCardDailyRequest): RoleCardDailyReceipt {
    const result = value as Partial<RoleCardDailyReceipt> | null;
    if (!result || !operatingDay(result.operatingDay) || result.operatingDay !== request.operatingDay ||
        !positive(result.acceptedAtUnixMs) || !positive(result.grantedUnits) || !integer(result.boundYuanbaoBalanceUnits) ||
        result.boundYuanbaoBalanceUnits.lessThan(result.grantedUnits)) {
        throw new Error('特权日领回执未确认原运营日，请重新连接后沿原操作核对');
    }
    return result as RoleCardDailyReceipt;
}
