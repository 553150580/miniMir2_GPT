import { _decorator, Button, Component, instantiate, isValid, Mask, Node, Prefab, ScrollView,
    UITransform, Vec2, warn } from 'cc';
import { visibleRange } from './VirtualListWindow';

const { ccclass, property } = _decorator;
export type ListRenderer = (node: Node, data: unknown, index: number) => void;

@ccclass('List')
export class List extends Component {
    static readonly EVENT_ITEM_CLICK = 'list-item-click';
    static readonly EVENT_BIND_ITEM = 'list-bind-item';

    @property(ScrollView) scrollView: ScrollView | null = null;
    @property(Prefab) itemTemplate: Prefab | null = null;
    @property(Node) templateNode: Node | null = null;
    @property(Node) viewport: Node | null = null;
    @property rowHeight = 64;
    @property overscan = 2;
    private items: readonly unknown[] = [];
    private renderer?: ListRenderer;
    private readonly rows = new Map<number, Node>();
    private readonly spare: Node[] = [];
    private readonly indices = new Map<Node, number>();
    private measuredViewport = 0;

    get itemCount(): number { return this.items.length; }
    get visibleNodeCount(): number { return this.rows.size; }

    protected onLoad(): void {
        this.scrollView ??= this.getComponent(ScrollView);
        this.viewport ??= this.scrollView?.content?.parent ?? null;
        this.templateNode ??= this.node.getChildByPath('ItemTemplate');
        if (this.templateNode) this.templateNode.active = false;
        if (!this.scrollView?.content || !this.viewport?.getComponent(Mask)) {
            warn('List 需要已保存的 ScrollView、Content、View/Mask');
        }
        if (!this.itemTemplate && !this.templateNode) warn('List 需要编辑器保存的条目 Prefab 或 ItemTemplate');
        if (!Number.isFinite(this.rowHeight) || this.rowHeight <= 0) this.rowHeight = 64;
        this.overscan = Number.isFinite(this.overscan) ? Math.max(0, Math.floor(this.overscan)) : 2;
    }

    protected onEnable(): void {
        if (isValid(this.scrollView, true) && isValid(this.scrollView.node, true)) {
            this.scrollView.node.on(ScrollView.EventType.SCROLLING, this.onScroll, this);
            this.scrollView.node.on(ScrollView.EventType.SCROLL_ENDED, this.onScroll, this);
        }
        if (isValid(this.viewport, true)) this.viewport.on(Node.EventType.SIZE_CHANGED, this.onViewportResize, this);
        this.resizeContent(true);
        this.refreshVisible(true);
    }

    protected onDisable(): void {
        if (isValid(this.scrollView, true) && isValid(this.scrollView.node, true)) {
            this.scrollView.node.off(ScrollView.EventType.SCROLLING, this.onScroll, this);
            this.scrollView.node.off(ScrollView.EventType.SCROLL_ENDED, this.onScroll, this);
        }
        if (isValid(this.viewport, true)) this.viewport.off(Node.EventType.SIZE_CHANGED, this.onViewportResize, this);
    }

    protected onDestroy(): void {
        // 父层递归销毁时，子条目可能先于本组件销毁；跳过已销毁或待销毁的引用。
        for (const row of Array.from(this.rows.values()).concat(this.spare)) if (isValid(row, true)) row.destroy();
        this.rows.clear();
        this.spare.length = 0;
        this.indices.clear();
        this.items = [];
        this.renderer = undefined;
    }

    setItems(items: readonly unknown[], renderer?: ListRenderer, preserveOffset = true): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        this.items = items.slice();
        if (renderer) this.renderer = renderer;
        this.resizeContent(preserveOffset);
        this.refreshVisible(true);
    }

    setRenderer(renderer: ListRenderer): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        this.renderer = renderer;
        this.refreshVisible(true);
    }

    scrollToIndex(index: number, duration = 0): void {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)) return;
        if (!this.items.length || !Number.isFinite(index)) return;
        const target = Math.min(this.items.length - 1, Math.max(0, Math.floor(index)));
        const offset = Math.min(target * this.rowHeight, this.scrollView?.getMaxScrollOffset().y ?? 0);
        this.scrollView?.scrollToOffset(new Vec2(0, offset), Number.isFinite(duration) ? Math.max(0, duration) : 0);
        this.refreshVisible(false);
    }

    private onScroll(): void { this.refreshVisible(false); }
    private onViewportResize(): void { this.resizeContent(true); this.refreshVisible(false); }

    private resizeContent(preserve: boolean): void {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)
            || !isValid(this.viewport, true) || !isValid(this.scrollView.content, true)) return;
        const content = this.scrollView.content.getComponent(UITransform);
        const view = this.viewport.getComponent(UITransform);
        if (!content || !view || !this.scrollView) return;
        const height = Math.max(view.height, this.items.length * this.rowHeight);
        const initialized = this.measuredViewport > 0;
        const unchanged = content.height === height && this.measuredViewport === view.height;
        this.measuredViewport = view.height;
        // 同尺寸的数据刷新只重绑内容，不打断手指拖动、惯性或回弹。
        if (preserve && unchanged) return;
        // 隐藏列表首次绑定时，ScrollView尚未初始化边界，不能把默认偏移当作用户滚动。
        const offset = preserve && initialized ? this.scrollView.getScrollOffset().y : 0;
        this.scrollView.stopAutoScroll();
        content.setContentSize(content.width, height);
        this.scrollView.scrollToOffset(new Vec2(0,
            Math.min(Math.max(0, offset), Math.max(0, content.height - view.height))), 0);
    }

    private refreshVisible(force: boolean): void {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)
            || !isValid(this.viewport, true)) return;
        const content = this.scrollView.content;
        const height = this.viewport.getComponent(UITransform)?.height ?? 0;
        if (!isValid(content, true) || (!this.itemTemplate && !this.templateNode)) return;
        const range = visibleRange(this.items.length, this.scrollView?.getScrollOffset().y ?? 0,
            height, this.rowHeight, this.overscan);
        for (const [index, row] of this.rows) {
            if (!isValid(row, true)) { this.rows.delete(index); this.indices.delete(row); continue; }
            if (index >= range.start && index < range.end) continue;
            row.active = false;
            this.rows.delete(index);
            this.indices.delete(row);
            this.spare.push(row);
        }
        for (let index = range.start; index < range.end; ++index) {
            let row = this.rows.get(index);
            const needsBind = !row || force;
            if (!row) {
                let available = this.spare.pop();
                while (available && !isValid(available, true)) available = this.spare.pop();
                available ??= this.createRow() ?? undefined;
                if (!available) return;
                row = available;
                row.setParent(content);
                row.setPosition(0, -index * this.rowHeight, 0);
                this.rows.set(index, row);
                this.indices.set(row, index);
            }
            row.active = true;
            if (needsBind) {
                try {
                    this.renderer?.(row, this.items[index], index);
                    row.emit(List.EVENT_BIND_ITEM, this.items[index], index);
                } catch (error) { warn('列表条目绑定失败', error); }
            }
        }
        const budget = Math.ceil(height / this.rowHeight) + Math.max(0, this.overscan) * 2 + 2;
        while (this.rows.size + this.spare.length > budget && this.spare.length) {
            const unused = this.spare.pop();
            if (isValid(unused, true)) unused.destroy();
        }
    }

    private createRow(): Node | null {
        const row = isValid(this.itemTemplate, true) ? instantiate(this.itemTemplate)
            : isValid(this.templateNode, true) ? instantiate(this.templateNode) : null;
        if (!row) return null;
        if (row.getComponent(Button)) row.on(Button.EventType.CLICK, () => {
            const index = this.indices.get(row);
            if (index !== undefined && isValid(this, true) && isValid(this.node, true)) {
                this.node.emit(List.EVENT_ITEM_CLICK, index, this.items[index], row);
            }
        }, this);
        return row;
    }
}
