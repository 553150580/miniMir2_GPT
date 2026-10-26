export interface VisibleRange { readonly start: number; readonly end: number; }

// 只计算可见索引；条目节点由 List 从编辑器模板复用。
export function visibleRange(count: number, offset: number, viewport: number,
    rowHeight: number, overscan = 2): VisibleRange {
    if (!Number.isFinite(rowHeight) || rowHeight <= 0) throw new Error('列表条目高度必须大于 0');
    const length = Number.isFinite(count) ? Math.max(0, Math.floor(count)) : 0;
    const height = Number.isFinite(viewport) ? Math.max(0, viewport) : 0;
    if (!length || !height) return { start: 0, end: 0 };
    const extra = Number.isFinite(overscan) ? Math.max(0, Math.floor(overscan)) : 0;
    const maxOffset = Math.max(0, length * rowHeight - height);
    const top = Math.min(maxOffset, Number.isFinite(offset) ? Math.max(0, offset) : 0);
    return {
        start: Math.max(0, Math.floor(top / rowHeight) - extra),
        end: Math.min(length, Math.ceil((top + height) / rowHeight) + extra),
    };
}
