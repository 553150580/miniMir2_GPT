import { _decorator, Component, EditBox, isValid, Label, Node, ScrollView } from 'cc';
import { List } from '../ui/List';
import { UIMgr } from '../ui/UIMgr';
import { MessageSnapshot, SystemMessage, SystemMessageLog } from './SystemMessageLog';

const { ccclass } = _decorator;

// 绑定编辑器保存的消息区、列表和默认EditBox；不创建固定节点或发送世界聊天。
@ccclass('GameMessages')
export class GameMessages extends Component {
    static readonly EVENT_SELECTED = 'system-message-selected';
    private readonly log = new SystemMessageLog();
    private lines: Array<Label | null> = [];
    private panel: Node | null = null;
    private list: List | null = null;
    private scroll: ScrollView | null = null;
    private search: EditBox | null = null;
    private summary: Label | null = null;
    private manager: UIMgr | null = null;
    private pending = true;
    private queryChanged = false;
    private touching = false;
    private rendered: readonly SystemMessage[] = [];
    private renderedOnce = false;
    private currentRole = '';
    private flushing = false;

    protected onLoad(): void {
        this.lines = ['BottomHUD/Chat/Line0', 'BottomHUD/Chat/Line1'].map(path => this.node.getChildByPath(path)?.getComponent(Label) ?? null);
        this.panel = this.node.getChildByPath('UIMgr/Panels/Messages');
        this.list = this.panel?.getChildByPath('List')?.getComponent(List) ?? null;
        this.scroll = this.list?.getComponent(ScrollView) ?? null;
        this.search = this.panel?.getChildByPath('Search')?.getComponent(EditBox) ?? null;
        this.summary = this.panel?.getChildByPath('Summary')?.getComponent(Label) ?? null;
        this.manager = this.node.getChildByPath('UIMgr')?.getComponent(UIMgr) ?? null;
    }

    protected onEnable(): void {
        this.search?.node.on(EditBox.EventType.TEXT_CHANGED, this.onSearch, this);
        this.list?.node.on(List.EVENT_ITEM_CLICK, this.onSelected, this);
        this.list?.node.on(Node.EventType.TOUCH_START, this.onTouchStart, this, true);
        this.list?.node.on(Node.EventType.TOUCH_END, this.onTouchEnd, this, true);
        this.list?.node.on(Node.EventType.TOUCH_CANCEL, this.onTouchEnd, this, true);
        this.scroll?.node.on(ScrollView.EventType.SCROLL_ENDED, this.flush, this);
        this.manager?.node.on(UIMgr.EVENT_CHANGED, this.onPanelsChanged, this);
    }

    protected onDisable(): void {
        if (isValid(this.search, true) && isValid(this.search.node, true)) this.search.node.off(EditBox.EventType.TEXT_CHANGED, this.onSearch, this);
        if (isValid(this.list, true) && isValid(this.list.node, true)) {
            this.list.node.off(List.EVENT_ITEM_CLICK, this.onSelected, this);
            this.list.node.off(Node.EventType.TOUCH_START, this.onTouchStart, this, true);
            this.list.node.off(Node.EventType.TOUCH_END, this.onTouchEnd, this, true);
            this.list.node.off(Node.EventType.TOUCH_CANCEL, this.onTouchEnd, this, true);
        }
        if (isValid(this.scroll, true) && isValid(this.scroll.node, true)) this.scroll.node.off(ScrollView.EventType.SCROLL_ENDED, this.flush, this);
        if (isValid(this.manager, true) && isValid(this.manager.node, true)) this.manager.node.off(UIMgr.EVENT_CHANGED, this.onPanelsChanged, this);
        this.unscheduleAllCallbacks(); this.touching = false;
    }

    refresh(snapshot: MessageSnapshot): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        if (snapshot.roleIdentity !== this.currentRole) {
            this.currentRole = snapshot.roleIdentity; this.renderedOnce = false; this.rendered = [];
            if (this.search) this.search.string = '';
            this.queryChanged = false;
        }
        if (!this.log.observe(snapshot)) return;
        this.presentRecent();
    }

    navigationUnreachable(identity: string): void {
        if (!isValid(this, true) || !isValid(this.node, true) || !this.log.navigationUnreachable(identity)) return;
        this.presentRecent();
    }

    private presentRecent(): void {
        const latest = this.log.messages.slice(-2);
        for (let index = 0; index < this.lines.length; index++) {
            const line = this.lines[index];
            if (line) line.string = latest[index] ? `[${latest[index].title}] ${latest[index].preview ?? latest[index].message}` : '轻触查看系统消息';
        }
        this.pending = true; this.flush();
    }

    private onSearch(): void { this.queryChanged = true; this.pending = true; this.flush(); }
    private onTouchStart(): void { this.touching = true; }
    private onTouchEnd(): void { this.touching = false; this.scheduleOnce(this.flush, 0); }
    private onPanelsChanged(): void {
        // 关闭消息面板时清除手指状态；再次打开才绑定积累的消息，不重建面板。
        if (!this.panel?.activeInHierarchy) { this.touching = false; return; }
        this.flush();
    }

    private onSelected(_index: number, item: SystemMessage): void {
        if (item && isValid(this.node, true)) this.node.emit(GameMessages.EVENT_SELECTED, { title: item.title, message: item.message });
    }

    private flush(): void {
        if (this.flushing || !this.pending || !isValid(this.list, true) || !isValid(this.scroll, true) || !this.panel?.activeInHierarchy) return;
        // List扩容会更新Content并停止惯性；手指拖动和回弹期间先缓存，结束后再应用。
        if (this.touching || this.scroll.isScrolling() || this.scroll.isAutoScrolling()) return;
        this.flushing = true;
        try {
            const filtered = this.log.filter(this.search?.string ?? '');
            const previousOffset = this.scroll.getScrollOffset().y;
            const rowHeight = this.list.rowHeight;
            const oldIndex = Math.max(0, Math.floor(previousOffset / rowHeight));
            const oldAnchor = this.rendered[oldIndex]?.id;
            const nearBottom = this.scroll.getMaxScrollOffset().y - previousOffset <= rowHeight;
            this.list.setItems(filtered, (node, data) => {
                const item = data as SystemMessage;
                const label = node.getChildByName('Text')?.getComponent(Label);
                if (label) label.string = `${item.title}\n${item.preview ?? item.message}`;
            }, true);
            if (this.queryChanged) this.list.restoreScrollOffset(0);
            else if (!this.renderedOnce || nearBottom) this.list.scrollToIndex(filtered.length - 1);
            else if (oldAnchor !== undefined) {
                const index = filtered.findIndex(item => item.id === oldAnchor);
                if (index >= 0) this.list.restoreScrollOffset(index * rowHeight + previousOffset % rowHeight);
            }
            this.rendered = Array.from(filtered); this.renderedOnce = true;
            this.queryChanged = false; this.pending = false;
            if (this.summary) this.summary.string = this.search?.string.trim()
                ? `找到 ${filtered.length} 条 · 本次会话共 ${this.log.messages.length} 条`
                : `本次会话 ${this.log.messages.length} 条 · 保留最近 ${this.log.capacity} 条`;
        } finally { this.flushing = false; }
    }
}
