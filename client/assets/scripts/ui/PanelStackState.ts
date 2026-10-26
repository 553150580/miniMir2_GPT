// 栈只保存已有面板引用；关闭不销毁节点、不重置面板数据或滚动位置。
export class PanelStackState<T> {
    private entries: T[] = [];

    get top(): T | null { return this.entries[this.entries.length - 1] ?? null; }
    get size(): number { return this.entries.length; }
    get items(): readonly T[] { return this.entries.slice(); }

    open(panel: T): void {
        this.entries = this.entries.filter(entry => entry !== panel);
        this.entries.push(panel);
    }

    back(): T | null { return this.entries.pop() ?? null; }
    clear(): void { this.entries.length = 0; }
}
