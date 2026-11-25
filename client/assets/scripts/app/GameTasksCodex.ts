import { Button, Color, isValid, Label, Node, Sprite, SpriteFrame } from 'cc';
import type Long from 'long';
import { List } from '../ui/List';

interface RewardItem {
    itemId: number;
    quantity: Long;
}

interface TaskRow {
    kind: number;
    id: number;
    name: string;
    progress: Long;
    target: Long;
    mapId: Long;
    monsterId: number;
    boundGold: Long;
    boundYuanbaoUnits: Long;
    rewardItems: RewardItem[];
    available: boolean;
    claimed: boolean;
    claimable: boolean;
}

interface TaskState {
    errorCode: number;
    day: string;
    roleRevision: Long;
    activity: number;
    dailyClaimed: number;
    dailyClaimLimit: number;
    rows: TaskRow[];
}

export interface TaskClaim {
    kind: number;
    id: number;
    day: string;
}

const categories = [
    { kind: 1, name: '主线' }, { kind: 2, name: '每日' }, { kind: 3, name: '成就' },
    { kind: 4, name: '图鉴' }, { kind: 5, name: '活跃' },
];
type Category = typeof categories[number];
type ListRow = TaskRow | { heading: string };

// 绑定保存场景中的列表和领奖按钮。目标进度、运营日及奖励全部使用服务端回包。
export class GameTasksCodex {
    private readonly categoryList: List;
    private readonly recordList: List;
    private readonly summary: Label;
    private readonly detail: Label;
    private readonly claimButton: Button;
    private readonly preview: Sprite;
    private previewFrame: SpriteFrame | null = null;
    private previewRequest?: { scope: string; monsterId: number };
    private category = categories[0];
    private selected = 0;
    private state?: TaskState;
    private loadedScope = '';
    private nextProgressRead = 0;
    private busy = false;
    private disposed = false;

    constructor(private readonly panel: Node, private readonly scope: () => string,
        private readonly query: () => Promise<TaskState>, private readonly award: (value: TaskClaim) => Promise<void>,
        private readonly mapName: (id: string) => string | undefined, private readonly itemName: (id: number) => string | undefined,
        private readonly monsterFrame: (id: number) => SpriteFrame | null,
        private readonly report: (error: unknown) => void, private readonly loadMonsterFrame: (id: number) => Promise<SpriteFrame | null>) {
        this.categoryList = panel.getChildByName('CategoryList')!.getComponent(List)!;
        this.recordList = panel.getChildByName('RecordList')!.getComponent(List)!;
        this.summary = panel.getChildByName('Summary')!.getComponent(Label)!;
        this.detail = panel.getChildByName('Detail')!.getComponent(Label)!;
        this.claimButton = panel.getChildByName('Claim')!.getComponent(Button)!;
        this.preview = panel.getChildByName('CodexPreview')!.getComponent(Sprite)!;
        this.categoryList.node.on(List.EVENT_ITEM_CLICK, this.chooseCategory, this);
        this.recordList.node.on(List.EVENT_ITEM_CLICK, this.chooseRecord, this);
        this.categoryList.setItems(categories, this.renderCategory);
        this.showSelection();
    }

    async reload(): Promise<void> {
        const scope = this.scope();
        if (!scope || this.busy || this.disposed) return;
        if (scope !== this.loadedScope) {
            this.state = undefined;
            this.selected = 0;
        }
        this.busy = true;
        this.nextProgressRead = Date.now() + 5000;
        this.showSelection();
        try {
            const result = await this.query();
            if (this.disposed || scope !== this.scope()) return;
            if (result.errorCode) throw new Error('暂时无法读取任务进度');
            this.state = result;
            this.loadedScope = scope;
            this.showRows();
        } finally {
            this.busy = false;
            if (!this.disposed) this.showSelection();
        }
    }

    observe(revision: Long): void {
        if (!this.panel.activeInHierarchy) {
            this.previewRequest = undefined;
            this.updatePreview(null, false);
            return;
        }
        if (this.state && !this.busy && this.scope()
            && (this.loadedScope !== this.scope() || !this.state.roleRevision.equals(revision) || Date.now() >= this.nextProgressRead)) {
            void this.reload().catch(this.report);
        }
    }

    async claim(): Promise<void> {
        const row = this.row;
        if (this.busy || !row?.claimable || this.loadedScope !== this.scope()) return;
        this.busy = true;
        this.showSelection();
        try {
            await this.award({ kind: row.kind, id: row.id, day: row.kind === 2 || row.kind === 5 ? this.state!.day : '' });
        } finally {
            this.busy = false;
            if (!this.disposed) this.showSelection();
        }
        await this.reload();
    }

    dispose(): void {
        this.disposed = true;
        this.previewRequest = undefined;
        this.updatePreview(null, false);
        if (isValid(this.categoryList.node, true)) this.categoryList.node.off(List.EVENT_ITEM_CLICK, this.chooseCategory, this);
        if (isValid(this.recordList.node, true)) this.recordList.node.off(List.EVENT_ITEM_CLICK, this.chooseRecord, this);
    }

    private get row(): TaskRow | undefined {
        return this.state?.rows.find(row => row.kind === this.category.kind && row.id === this.selected);
    }

    private chooseCategory = (_index: number, category: Category): void => {
        if (category === this.category) return;
        this.category = category;
        this.selected = 0;
        this.categoryList.setRenderer(this.renderCategory);
        this.showRows(false);
    };

    private chooseRecord = (_index: number, row: ListRow): void => {
        if ('heading' in row) return;
        if (row.kind !== this.category.kind) return;
        this.selected = row.id;
        this.recordList.setRenderer(this.renderRecord);
        this.showSelection();
    };

    private renderCategory = (node: Node, value: unknown): void => {
        const category = value as Category;
        node.getChildByName('Text')!.getComponent(Label)!.string = (category === this.category ? '● ' : '') + category.name;
    };

    private renderRecord = (node: Node, value: unknown): void => {
        const row = value as ListRow;
        const label = node.getChildByName('Text')!.getComponent(Label)!;
        node.getComponent(Button)!.interactable = !('heading' in row);
        if ('heading' in row) {
            label.string = row.heading;
            return;
        }
        const status = row.claimed ? '已领' : row.claimable ? '可领' : row.available ? row.progress + '/' + row.target : '未解锁';
        label.string = `${row.id === this.selected ? '● ' : ''}${row.name}  ${status}`;
    };

    private showRows(keepOffset = true): void {
        const rows = this.state?.rows.filter(row => row.kind === this.category.kind) ?? [];
        if (!rows.some(row => row.id === this.selected)) this.selected = rows[0]?.id ?? 0;
        if (this.category.kind !== 4) {
            this.recordList.setItems(rows, this.renderRecord, keepOffset);
        } else {
            const byMap = new Map<string, TaskRow[]>();
            for (const row of rows) {
                const map = row.mapId.toString();
                const group = byMap.get(map) ?? [];
                if (!byMap.has(map)) byMap.set(map, group);
                group.push(row);
            }
            const grouped: ListRow[] = [];
            for (const [map, group] of Array.from(byMap)) grouped.push({ heading: this.mapName(map) ?? '地图 ' + map }, ...group);
            this.recordList.setItems(grouped, this.renderRecord, keepOffset);
        }
        this.showSelection();
    }

    private showSelection(): void {
        const row = this.row;
        const showMonster = row?.kind === 4 && this.panel.activeInHierarchy && this.scope() === this.loadedScope;
        this.showMonsterPreview(showMonster ? row : undefined);
        this.claimButton.interactable = !!row?.claimable && !this.busy && this.loadedScope === this.scope();
        this.summary.string = this.state && (this.category.kind === 2 || this.category.kind === 5)
            ? `活跃 ${this.state.activity}/100 · 已领 ${this.state.dailyClaimed}/${this.state.dailyClaimLimit}`
            : this.busy ? '正在同步进度…' : '';
        if (!row) {
            this.detail.string = this.busy ? '' : this.state ? '此分类暂无开放的目标' : '请刷新任务进度';
            return;
        }
        const rewards: string[] = [];
        if (!row.boundGold.isZero()) rewards.push(`绑定金币 ${row.boundGold}`);
        if (!row.boundYuanbaoUnits.isZero()) {
            const value = row.boundYuanbaoUnits;
            rewards.push(`绑定元宝 ${value.div(10)}.${value.mod(10)}`);
        }
        for (const item of row.rewardItems) rewards.push(`${this.itemName(item.itemId) ?? '物品 ' + item.itemId} × ${item.quantity}`);
        const map = this.mapName(row.mapId.toString());
        const reward = rewards.length ? '\n奖励  ' + rewards.join('、') : '';
        this.detail.string = `${map ? map + ' · ' : ''}${row.name}  ${row.progress}/${row.target}${reward}`;
    }

    private showMonsterPreview(row?: TaskRow): void {
        if (!row) {
            this.previewRequest = undefined;
            this.updatePreview(null, false);
            return;
        }
        const scope = this.loadedScope;
        const firstFrame = this.monsterFrame(row.monsterId);
        if (firstFrame) {
            this.previewRequest = undefined;
            this.updatePreview(firstFrame, row.progress.isZero());
            return;
        }
        if (this.previewRequest?.scope === scope && this.previewRequest.monsterId === row.monsterId) {
            this.updatePreview(this.previewFrame, row.progress.isZero());
            return;
        }
        const request = { scope, monsterId: row.monsterId };
        this.previewRequest = request;
        this.updatePreview(null, false);
        void this.loadMonsterFrame(row.monsterId).then(frame => {
            try {
                const current = this.row;
                if (this.previewRequest === request && !this.disposed && this.panel.activeInHierarchy && this.scope() === scope
                    && current?.kind === 4 && current.monsterId === request.monsterId) this.updatePreview(frame, current.progress.isZero());
            } finally { frame?.decRef(); }
        }).catch(error => {
            if (this.previewRequest === request && !this.disposed && this.panel.activeInHierarchy && this.scope() === scope) this.report(error);
        });
    }

    private updatePreview(frame: SpriteFrame | null, silhouette: boolean): void {
        // 图鉴独立持有当前一帧，旧地图清理时不会提前释放仍被预览使用的贴图。
        const previous = this.previewFrame;
        if (previous !== frame) {
            frame?.addRef();
            this.previewFrame = frame;
            if (isValid(this.preview, true)) {
                this.preview.spriteFrame = frame;
                this.preview.node.active = Boolean(frame);
            }
            previous?.decRef();
        }
        if (frame && isValid(this.preview, true)) this.preview.color = silhouette ? new Color(18, 25, 30, 255) : Color.WHITE;
    }
}
