import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { traceInput } from './InputTrace';
import { adjustPurchaseQuantity, assetId, assetInteger, canRecycleBook, captureSkillAssetContext, MaterialChoice, materialSelectionMatches,
    ShopOffer, SkillAssetContext, SkillAssetItem, SkillAssetQuote, SkillAssetsSnapshot, SkillMaterialSelection, SkillUpgradeRule,
    skillQuoteLines, sourceName, upgradeRequirements, yuanbaoText } from './SkillAssetsModel';

export interface PreparedSkillAsset {
    context: SkillAssetContext;
    request: { kind: number; offerId: number; quantity: bigint; skillId: number; materials: MaterialChoice[] };
}
interface RowBinding { item: SkillAssetItem; plus: () => void; minus: () => void; }

// 普通资产面板和覆盖报价列表均只绑定保存的节点及模板，不创建固定布局。
export class GameSkillAssets {
    private readonly panel: Node;
    private readonly offersList: List;
    private readonly materialsList: List;
    private readonly quoteList: List;
    private readonly selections = new SkillMaterialSelection();
    private readonly rows = new Map<Node, RowBinding>();
    private input: SkillAssetsSnapshot | null = null;
    private kind = 1;
    private skillId = 0;
    private offerId = 0;
    private purchaseQuantity = 1n;
    private key = '';
    private notice = '';
    private busy = false;
    private resetListOffset = true;

    constructor(safe: Node, private readonly popup: PopupView, private readonly report: (error: unknown) => void) {
        const panel = safe.getChildByPath('UIMgr/Panels/SkillAssets');
        const quote = safe.getChildByPath('PopupView/Panels/SkillQuote/List')?.getComponent(List);
        if (!panel || !quote) throw new Error('技能资产与报价面板尚未保存');
        this.panel = panel; this.quoteList = quote;
        this.offersList = this.saved('OffersList').getComponent(List)!;
        this.materialsList = this.saved('MaterialsList').getComponent(List)!;
        this.offersList.node.on(List.EVENT_ITEM_CLICK, this.selectOffer, this);
        this.materialsList.node.on(List.EVENT_ITEM_CLICK, this.itemDetails, this);
    }

    open(kind: number, skillId = 0): void {
        this.kind = kind; this.skillId = skillId; this.offerId = 0; this.purchaseQuantity = 1n; this.selections.clear(); this.notice = ''; this.key = '';
        this.resetListOffset = true;
        if (this.input) this.refresh(this.input);
    }
    lock(value: boolean): void { this.busy = value; this.key = ''; if (this.input) this.refresh(this.input); }
    reset(): void { if (this.busy) return; this.selections.clear(); this.offerId = 0; this.purchaseQuantity = 1n; this.notice = ''; this.key = ''; if (this.input) this.refresh(this.input); }
    adjustPurchase(delta: number): void {
        if (this.busy) return;
        const offer = this.input?.offers.find(row => row.offer_id === this.offerId); if (!offer) throw new Error('先选择要购买的商品');
        this.purchaseQuantity = adjustPurchaseQuantity(this.purchaseQuantity, delta, offer.max_purchase_quantity);
        this.key = ''; if (this.input) this.refresh(this.input);
    }
    private upgrade(input: SkillAssetsSnapshot): SkillUpgradeRule | undefined {
        const learned = input.inventory?.learnedSkills?.find(row => row.skillId === this.skillId);
        return learned ? input.upgrades.find(row => row.skill_id === this.skillId && row.level === learned.level + 1) : undefined;
    }
    private items(input: SkillAssetsSnapshot): SkillAssetItem[] {
        const required = this.kind === 2 ? this.upgrade(input) : undefined;
        const ids = required ? upgradeRequirements(required) : new Map<number, bigint>();
        return (input.inventory?.items ?? []).filter(item => assetInteger(item.quantity) > 0n && (this.kind === 2 ? ids.has(item.itemId)
            : input.definitions.find(row => row.item_id === item.itemId)?.kind === 'book'));
    }
    refresh(input: SkillAssetsSnapshot): void {
        this.input = input;
        if (this.selections.reconcile(input.inventory?.items ?? [])) { this.notice = '原材料已变化，请重新选择'; this.key = ''; }
        const key = JSON.stringify({ kind: this.kind, skill: this.skillId, offer: this.offerId, purchase: this.purchaseQuantity.toString(), revision: input.inventory?.revision.toString(),
            wallet: input.wallet?.revision.toString(), ready: input.ready, busy: this.busy, notice: this.notice,
            choices: this.items(input).map(item => [assetId(item.instanceId), this.selections.quantity(item).toString()]) });
        if (key === this.key) return; this.key = key;
        this.saved('OffersList').active = this.kind === 1; this.saved('MaterialsList').active = this.kind !== 1;
        this.saved('PurchaseQuantity').active = this.kind === 1;
        this.text('Title', this.kind === 1 ? '技能书店' : this.kind === 2 ? '技能升级' : '技能书分解');
        const preserveOffset = !this.resetListOffset;
        if (this.kind === 1) {
            this.offersList.setItems(input.offers, (node, data) => {
                const offer = data as ShopOffer;
                const price = offer.currency_kind === 'gold' ? `${offer.price_units}金币` : `${yuanbaoText(offer.price_units)}${offer.currency_kind === 'ingot' ? '元宝' : '绑定元宝'}`;
                node.getChildByName('Text')!.getComponent(Label)!.string = `${this.offerId === offer.offer_id ? '● ' : ''}${offer.name} ×${offer.quantity}\n${price} · 产物绑定\n${offer.required_role_level}级可购买`;
            }, preserveOffset);
            const offer = input.offers.find(row => row.offer_id === this.offerId);
            this.text('Message', offer ? `${offer.name} · 购买${this.purchaseQuantity}份\n${offer.currency_kind === 'gold' ? '先扣绑定金币，不足再扣金币' : offer.currency_kind === 'ingot' ? '仅扣元宝，不换用绑定元宝' : '仅扣绑定元宝，不换用元宝'}` : '点击商品选择购买\nNPC技能书不可分解为书页');
            this.text('PurchaseQuantity/Quantity', this.purchaseQuantity.toString());
            this.saved('PurchaseQuantity/Minus').getComponent(Button)!.interactable = !!offer && !this.busy && this.purchaseQuantity > 1n;
            this.saved('PurchaseQuantity/Plus').getComponent(Button)!.interactable = !!offer && !this.busy && this.purchaseQuantity < BigInt(offer.max_purchase_quantity);
        } else {
            this.materialsList.setItems(this.items(input), (node, data) => this.renderMaterial(node, data as SkillAssetItem), preserveOffset);
            const learned = input.inventory?.learnedSkills?.find(row => row.skillId === this.skillId);
            const rule = this.upgrade(input), skill = input.skills.find(row => row.id === this.skillId);
            const costs = rule ? Array.from(upgradeRequirements(rule)).map(([id, quantity]) => `${input.definitions.find(row => row.item_id === id)?.name ?? id}×${quantity}`).join('、') : '';
            this.text('Message', this.kind === 3 ? '只拆明确白名单来源与品质的书\n使用＋/−选择原实例数量' : !learned ? '尚未学会此技能'
                : !rule ? `${skill?.name ?? '技能'} 等级${learned.level}\n当前没有开放的下一阶`
                : `${skill?.name ?? '技能'} ${learned.level} → ${rule.level} · 需${rule.required_role_level}级\n${costs}\n使用＋/−明确选择书本与书页`);
        }
        this.resetListOffset = false;
        const synced = input.ready && !!input.inventory && !!input.wallet && input.inventory.revision.toString() === input.wallet.revision.toString();
        let eligible = false;
        if (this.kind === 1) { const offer = input.offers.find(row => row.offer_id === this.offerId); eligible = !!offer && offer.max_purchase_quantity >= 1 && (input.inventory?.level ?? 0) >= offer.required_role_level; }
        else { const choices = this.selections.selected(input.inventory?.items ?? []), rule = this.upgrade(input);
            eligible = this.kind === 3 ? choices.length > 0 : !!rule && (input.inventory?.level ?? 0) >= rule.required_role_level && materialSelectionMatches(rule, choices, input.inventory?.items ?? []); }
        this.saved('Preview').getComponent(Button)!.interactable = synced && eligible && !this.busy;
        this.saved('Reset').getComponent(Button)!.interactable = !this.busy;
        this.text('Hint', this.notice || '报价后核对材料、绑定与钱包扣款');
        for (const [node] of this.rows) if (!isValid(node, true)) this.rows.delete(node);
    }
    prepare(): PreparedSkillAsset {
        if (!this.input) throw new Error('资产尚未同步');
        const choices = this.kind === 1 ? [] : this.selections.selected(this.input.inventory?.items ?? []);
        return { context: captureSkillAssetContext(this.input, choices), request: { kind: this.kind, offerId: this.kind === 1 ? this.offerId : 0,
            quantity: this.kind === 1 ? this.purchaseQuantity : 0n, skillId: this.kind === 2 ? this.skillId : 0, materials: choices } };
    }
    showQuote(quote: SkillAssetQuote): Promise<boolean> {
        const input = this.input!;
        this.quoteList.setItems(skillQuoteLines(quote, input.definitions), (node, data) => { node.getChildByName('Text')!.getComponent(Label)!.string = String(data); }, false);
        return this.popup.confirm('SkillQuote', '请滚动查看全部报价明细，再确认提交。', quote.kind === 1 ? '购买确认' : quote.kind === 2 ? '升级确认' : '拆书确认');
    }
    private renderMaterial(node: Node, item: SkillAssetItem): void {
        let binding = this.rows.get(node);
        if (!binding) {
            const adjust = (delta: number): void => {
                traceInput({ event: 'button', path: `SkillAssets/Materials/${delta > 0 ? 'Plus' : 'Minus'}`, blocked: this.busy });
                if (this.busy) return;
                try { this.selections.adjust(this.rows.get(node)!.item, delta); this.key = ''; this.refresh(this.input!); } catch (error) { this.report(error); }
            };
            binding = { item, plus: () => adjust(1), minus: () => adjust(-1) }; this.rows.set(node, binding);
            node.getChildByName('Plus')!.on(Button.EventType.CLICK, binding.plus, this); node.getChildByName('Minus')!.on(Button.EventType.CLICK, binding.minus, this);
        }
        binding.item = item;
        const count = this.selections.quantity(item), eligible = this.kind !== 3 || canRecycleBook(item, this.input!.recycle);
        const definition = this.input!.definitions.find(row => row.item_id === item.itemId);
        node.getChildByName('Text')!.getComponent(Label)!.string = `${definition?.name ?? item.itemId} ×${item.quantity}\n${item.bound ? '绑定' : '非绑定'} · ${sourceName(item.sourceKind)}\n${eligible ? `实例${assetId(item.instanceId).slice(-6)} · 品质${item.quality ?? 0}` : '此来源或品质不可拆书'}`;
        node.getChildByName('Quantity')!.getComponent(Label)!.string = count.toString();
        node.getChildByName('Plus')!.getComponent(Button)!.interactable = eligible && !this.busy && count < assetInteger(item.quantity);
        node.getChildByName('Minus')!.getComponent(Button)!.interactable = !this.busy && count > 0n;
    }
    private selectOffer(_index: number, offer: ShopOffer): void { if (this.busy) return; this.offerId = offer.offer_id; this.purchaseQuantity = 1n; this.key = ''; if (this.input) this.refresh(this.input); }
    private itemDetails(_index: number, item: SkillAssetItem): void {
        if (this.busy) return;
        this.popup.show('Details', { title: this.input?.definitions.find(row => row.item_id === item.itemId)?.name ?? '材料详情',
            message: `数量 ${item.quantity}\n${item.bound ? '绑定' : '非绑定'} · ${sourceName(item.sourceKind)}\n品质 ${item.quality ?? 0}\n实例 ${assetId(item.instanceId).slice(-6)}\n使用＋/−选择本实例消耗数量` });
    }
    dispose(): void {
        if (isValid(this.offersList.node, true)) this.offersList.node.off(List.EVENT_ITEM_CLICK, this.selectOffer, this);
        if (isValid(this.materialsList.node, true)) this.materialsList.node.off(List.EVENT_ITEM_CLICK, this.itemDetails, this);
        for (const [node, row] of this.rows) if (isValid(node, true)) {
            node.getChildByName('Plus')?.off(Button.EventType.CLICK, row.plus, this); node.getChildByName('Minus')?.off(Button.EventType.CLICK, row.minus, this);
        }
        this.rows.clear(); this.selections.clear(); this.input = null;
    }
    private saved(path: string): Node { const value = this.panel.getChildByPath(path); if (!value) throw new Error(`技能资产缺少保存节点：${path}`); return value; }
    private text(path: string, value: string): void { const label = this.saved(path).getComponent(Label); if (label) label.string = value; }
}
