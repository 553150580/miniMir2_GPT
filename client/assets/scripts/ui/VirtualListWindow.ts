export interface VisibleRange { readonly start: number; readonly end: number; }
export enum ListLayoutMode { Vertical, Horizontal, Grid }

export interface ListLayout {
    readonly mode: ListLayoutMode;
    readonly count: number;
    readonly columns: number;
    readonly lines: number;
    readonly viewportWidth: number;
    readonly viewportHeight: number;
    readonly cellWidth: number;
    readonly cellHeight: number;
    readonly spacingX: number;
    readonly spacingY: number;
    readonly contentWidth: number;
    readonly contentHeight: number;
}

const nonNegative = (value: number): number => Number.isFinite(value) ? Math.max(0, value) : 0;
const itemCount = (value: number): number => Math.floor(nonNegative(value));
const extent = (lines: number, size: number, gap: number): number => lines > 0 ? lines * size + (lines - 1) * gap : 0;

function axisRange(count: number, offset: number, viewport: number, size: number, gap: number, overscan: number): VisibleRange {
    if (count === 0 || viewport === 0) return { start: 0, end: 0 };
    const extra = itemCount(overscan), stride = size + gap;
    const top = Math.min(Math.max(0, extent(count, size, gap) - viewport), nonNegative(offset));
    // 遮罩起点停在间隔内时，上一格已经完全离开可见区。
    const first = Math.floor((top - size) / stride) + 1;
    return { start: Math.max(0, first - extra), end: Math.min(count, Math.ceil((top + viewport) / stride) + extra) };
}

// 只计算可见索引；条目节点由 List 从编辑器模板复用。
export function visibleRange(count: number, offset: number, viewport: number,
    rowHeight: number, overscan = 2): VisibleRange {
    if (!Number.isFinite(rowHeight) || rowHeight <= 0) throw new Error('列表条目高度必须大于 0');
    return axisRange(itemCount(count), offset, nonNegative(viewport), rowHeight, 0, overscan);
}

export function listLayout(mode: ListLayoutMode, count: number, viewWidth: number, viewHeight: number,
    width: number, height: number, gapX: number, gapY: number): ListLayout {
    const length = itemCount(count);
    const viewportWidth = nonNegative(viewWidth), viewportHeight = nonNegative(viewHeight);
    const cellWidth = Math.max(1, nonNegative(width)), cellHeight = Math.max(1, nonNegative(height));
    const spacingX = nonNegative(gapX), spacingY = nonNegative(gapY);
    const horizontal = mode === ListLayoutMode.Horizontal;
    const columns = mode === ListLayoutMode.Grid ? Math.max(1, Math.floor((viewportWidth + spacingX) / (cellWidth + spacingX))) : 1;
    const lines = Math.ceil(length / columns);
    return { mode, count: length, columns, lines, viewportWidth, viewportHeight, cellWidth, cellHeight, spacingX, spacingY,
        contentWidth: Math.max(viewportWidth, horizontal ? extent(lines, cellWidth, spacingX) : cellWidth),
        contentHeight: Math.max(viewportHeight, horizontal ? cellHeight : extent(lines, cellHeight, spacingY)) };
}

export function layoutVisibleRange(layout: ListLayout, offset: number, overscan: number): VisibleRange {
    if (layout.viewportWidth === 0 || layout.viewportHeight === 0) return { start: 0, end: 0 };
    const range = layout.mode === ListLayoutMode.Horizontal
        ? axisRange(layout.lines, offset, layout.viewportWidth, layout.cellWidth, layout.spacingX, overscan)
        : axisRange(layout.lines, offset, layout.viewportHeight, layout.cellHeight, layout.spacingY, overscan);
    return { start: range.start * layout.columns, end: Math.min(layout.count, range.end * layout.columns) };
}

export function listItemPoint(layout: ListLayout, index: number): { x: number; y: number } {
    if (layout.mode === ListLayoutMode.Horizontal) return { x: index * (layout.cellWidth + layout.spacingX), y: 0 };
    return { x: (index % layout.columns) * (layout.cellWidth + layout.spacingX), y: Math.floor(index / layout.columns) * (layout.cellHeight + layout.spacingY) };
}

export function listOffsetForIndex(layout: ListLayout, index: number): number {
    const point = listItemPoint(layout, index);
    return layout.mode === ListLayoutMode.Horizontal ? point.x : point.y;
}

export function listMaximumOffset(layout: ListLayout): number {
    return layout.mode === ListLayoutMode.Horizontal ? layout.contentWidth - layout.viewportWidth : layout.contentHeight - layout.viewportHeight;
}

export function preserveListOffset(previous: ListLayout, next: ListLayout, offset: number): number {
    if (previous.mode === next.mode && previous.columns === next.columns && listOffsetForIndex(previous, previous.columns) === listOffsetForIndex(next, next.columns)) return offset;
    const stride = listOffsetForIndex(previous, previous.columns);
    const top = Math.min(listMaximumOffset(previous), nonNegative(offset));
    const first = Math.floor(top / stride) * previous.columns;
    const remainder = Math.min(top % stride, listOffsetForIndex(next, next.columns));
    return listOffsetForIndex(next, first) + remainder;
}

export function listNodeBudget(layout: ListLayout, overscan: number): number {
    const horizontal = layout.mode === ListLayoutMode.Horizontal;
    const viewport = horizontal ? layout.viewportWidth : layout.viewportHeight;
    const stride = listOffsetForIndex(layout, layout.columns);
    return Math.min(layout.count, (Math.ceil(viewport / stride) + 2 * itemCount(overscan) + 2) * layout.columns);
}
