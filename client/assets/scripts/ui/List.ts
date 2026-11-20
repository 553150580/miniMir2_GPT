import { _decorator, Button, Component, Enum, instantiate, isValid, Mask, Node, Prefab, ScrollView,
    UITransform, Vec2, warn } from 'cc';
import { layoutVisibleRange, listItemPoint, listLayout, ListLayoutMode, listMaximumOffset,
    listNodeBudget, listOffsetForIndex, preserveListOffset } from './VirtualListWindow';
import type { ListLayout } from './VirtualListWindow';

export { ListLayoutMode } from './VirtualListWindow';

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
    @property({ type: Enum(ListLayoutMode), tooltip: '垂直、水平或按可见宽度换列的网格' }) layoutMode = ListLayoutMode.Vertical;
    @property({ min: 0, tooltip: '条目宽度，0沿用保存模板宽度' }) cellWidth = 0;
    @property({ min: 0, tooltip: '条目高度，0沿用旧行高' }) cellHeight = 0;
    @property({ min: 0, tooltip: '相邻条目的水平间隔' }) spacingX = 0;
    @property({ min: 0, tooltip: '相邻条目的垂直间隔' }) spacingY = 0;
    private items: readonly unknown[] = [];
    private renderer?: ListRenderer;
    private readonly rows = new Map<number, Node>();
    private readonly spare: Node[] = [];
    private readonly indices = new Map<Node, number>();
    private layout?: ListLayout;
    private legacyVertical = false;

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
        const relayout = this.resizeContent(true);
        this.refreshVisible(true, relayout);
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
        this.layout = undefined;
    }

    setItems(items: readonly unknown[], renderer?: ListRenderer, preserveOffset = true): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        this.items = items.slice();
        if (renderer) this.renderer = renderer;
        const relayout = this.resizeContent(preserveOffset);
        this.refreshVisible(true, relayout);
    }

    setRenderer(renderer: ListRenderer): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        this.renderer = renderer;
        this.refreshVisible(true);
    }

    scrollToIndex(index: number, duration = 0): void {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)) return;
        const layout = this.layout;
        if (!layout || !this.items.length || !Number.isFinite(index)) return;
        const target = Math.min(this.items.length - 1, Math.max(0, Math.floor(index)));
        this.moveTo(layout, listOffsetForIndex(layout, target), Number.isFinite(duration) ? Math.max(0, duration) : 0);
        this.refreshVisible(false);
    }

    // ScrollView的即时位移不保证派发滚动事件，恢复后必须同步重建当前可见池。
    restoreScrollOffset(offset: number): void {
        if (!Number.isFinite(offset) || !isValid(this, true) || !isValid(this.node, true)
            || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)
            || !isValid(this.viewport, true) || !isValid(this.scrollView.content, true)) return;
        const layout = this.layout;
        if (!layout) return;
        this.scrollView.stopAutoScroll();
        this.moveTo(layout, offset, 0);
        this.refreshVisible(false);
    }

    private onScroll(): void { this.refreshVisible(false); }
    private onViewportResize(): void { const relayout = this.resizeContent(true); this.refreshVisible(false, relayout); }

    private resizeContent(preserve: boolean): boolean {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)
            || !isValid(this.viewport, true) || !isValid(this.scrollView.content, true)) return false;
        const content = this.scrollView.content.getComponent(UITransform);
        const view = this.viewport.getComponent(UITransform);
        if (!content || !view) return false;
        const previous = this.layout;
        const mode = [ListLayoutMode.Horizontal, ListLayoutMode.Grid].includes(this.layoutMode) ? this.layoutMode : ListLayoutMode.Vertical;
        const legacy = mode === ListLayoutMode.Vertical && this.cellWidth === 0 && this.cellHeight === 0 && this.spacingX === 0 && this.spacingY === 0;
        const template = this.itemTemplate?.data ?? this.templateNode;
        const savedWidth = template?.getComponent(UITransform)?.width ?? view.width;
        const width = this.cellWidth > 0 && Number.isFinite(this.cellWidth) ? this.cellWidth : savedWidth > 0 ? savedWidth : view.width;
        const height = this.cellHeight > 0 && Number.isFinite(this.cellHeight) ? this.cellHeight : this.rowHeight;
        const layout = listLayout(mode, this.items.length, view.width, view.height, width, height, this.spacingX, this.spacingY);
        const contentWidth = legacy ? content.width : layout.contentWidth;
        const unchanged = !!previous && this.legacyVertical === legacy && content.width === contentWidth && content.height === layout.contentHeight
            && previous.viewportWidth === layout.viewportWidth && previous.viewportHeight === layout.viewportHeight
            && previous.mode === mode && previous.columns === layout.columns && previous.cellWidth === layout.cellWidth
            && previous.cellHeight === layout.cellHeight && previous.spacingX === layout.spacingX && previous.spacingY === layout.spacingY;
        this.layout = layout;
        this.legacyVertical = legacy;
        // 同尺寸的数据刷新只重绑内容，不打断手指拖动、惯性或回弹。
        if (preserve && unchanged) return false;
        // 隐藏列表首次绑定时，ScrollView尚未初始化边界，不能把默认偏移当作用户滚动。
        const offset = preserve && previous && previous.viewportWidth > 0 && previous.viewportHeight > 0
            ? preserveListOffset(previous, layout, this.readOffset(previous)) : 0;
        this.scrollView.stopAutoScroll();
        this.scrollView.horizontal = mode === ListLayoutMode.Horizontal;
        this.scrollView.vertical = !this.scrollView.horizontal;
        content.setContentSize(contentWidth, layout.contentHeight);
        this.moveTo(layout, offset, 0);
        return !unchanged;
    }

    private readOffset(layout: ListLayout): number {
        const offset = this.scrollView!.getScrollOffset();
        // Creator横向读取的Content左边界偏移是负数，写入scrollToOffset使用正数。
        return layout.mode === ListLayoutMode.Horizontal ? -offset.x : offset.y;
    }

    private moveTo(layout: ListLayout, offset: number, duration: number): void {
        const target = Math.min(Math.max(0, Number.isFinite(offset) ? offset : 0), listMaximumOffset(layout));
        this.scrollView!.scrollToOffset(layout.mode === ListLayoutMode.Horizontal ? new Vec2(target, 0) : new Vec2(0, target), duration);
    }

    private refreshVisible(force: boolean, relayout = false): void {
        if (!isValid(this, true) || !isValid(this.scrollView, true) || !isValid(this.scrollView.node, true)
            || !isValid(this.viewport, true)) return;
        const content = this.scrollView.content;
        const layout = this.layout;
        if (!layout || !isValid(content, true) || (!this.itemTemplate && !this.templateNode)) return;
        const transform = content.getComponent(UITransform);
        if (!transform) return;
        const range = layoutVisibleRange(layout, this.readOffset(layout), this.overscan);
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
            const needsPlacement = !row || relayout;
            if (!row) {
                let available = this.spare.pop();
                while (available && !isValid(available, true)) available = this.spare.pop();
                available ??= this.createRow() ?? undefined;
                if (!available) return;
                row = available;
                row.setParent(content);
                this.rows.set(index, row);
                this.indices.set(row, index);
            }
            if (needsPlacement) this.placeRow(row, transform, layout, index);
            row.active = true;
            if (needsBind) {
                try {
                    this.renderer?.(row, this.items[index], index);
                    row.emit(List.EVENT_BIND_ITEM, this.items[index], index);
                } catch (error) { warn('列表条目绑定失败', error); }
            }
        }
        const budget = listNodeBudget(layout, this.overscan);
        while (this.rows.size + this.spare.length > budget && this.spare.length) {
            const unused = this.spare.pop();
            if (isValid(unused, true)) unused.destroy();
        }
    }

    private placeRow(row: Node, content: UITransform, layout: ListLayout, index: number): void {
        const point = listItemPoint(layout, index);
        const cell = row.getComponent(UITransform);
        if (this.legacyVertical || !cell) { row.setPosition(point.x, -point.y, 0); return; }
        cell.setContentSize(layout.cellWidth, layout.cellHeight);
        // 条目和Content的锚点来自保存模板，统一从其左上角计算格坐标。
        row.setPosition(-content.width * content.anchorX + point.x + cell.width * cell.anchorX,
            content.height * (1 - content.anchorY) - point.y - cell.height * (1 - cell.anchorY), 0);
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
