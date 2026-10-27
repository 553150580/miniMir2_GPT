import { _decorator, Button, Component, director, EditBox, game, Game, isValid, Label, Node, profiler, ProgressBar, sys, Toggle } from 'cc';
import Long from 'long';
import { CreatorSceneBackend } from '../core/scene/CreatorSceneBackend';
import { SceneLoader } from '../core/scene/SceneLoader';
import { SceneManager } from '../core/scene/SceneManager';
import { UIMgr } from '../ui/UIMgr';
import { PopupView } from '../ui/PopupView';
import { List } from '../ui/List';
import { NavigationIntent } from '../input/NavigationIntent';
import { TouchJoystick } from '../input/TouchJoystick';
import { MapTouchNavigator } from '../input/MapTouchNavigator';
import { newOperationId } from './GameConnection';
import { operation, roleIdentity, session } from './GameSession';
import { PendingOperations } from './PendingOperations';
import { GameWorldView } from './GameWorldView';
import { WorldAssets } from './WorldAssets';
import { GameHUD } from './GameHUD';
import { GameMessages } from './GameMessages';

const { ccclass, property } = _decorator;
const names = ['男战士', '女法师', '男道士'];

@ccclass('AppRoot')
export class AppRoot extends Component {
    @property sceneKind = 'Boot';
    private readonly buttons: Array<[Node, () => void]> = [];
    private readonly cleanups: Array<() => void> = [];
    private nav = new NavigationIntent();
    private world?: GameWorldView;
    private hud?: GameHUD;
    private messages?: GameMessages;
    private ui?: UIMgr;
    private popup?: PopupView;
    private selectedRole = -1;
    private selectedJob = 0;
    private selectedItem: any = null;
    private inputSequence = 0;
    private auto = true;
    private count = 1;
    private disposed = false;
    private economicBusy = false;
    private unresolved = false;
    private moveTimer = 0;
    private lastBaseline = '';
    private lastInventory: any = null;
    private joystick?: TouchJoystick;
    private uiActionBusy = false;
    private reconciliationStarted = false;
    private releaseInput = (): void => { if (this.joystick) this.joystick.release(); else this.nav.releaseJoystick(); };

    async start(): Promise<void> {
        profiler.hideStats();
        session.scenes ??= new SceneManager(new SceneLoader(new CreatorSceneBackend(director)), ['Boot', 'Login', 'RoleSelect', 'Game'], this.sceneKind);
        const unsubscribe = session.scenes.subscribe(state => {
            const overlay = this.node.getChildByPath('Loading'); if (!overlay) return;
            overlay.active = state.phase === 'preloading' || state.phase === 'loading';
            const label = overlay.getChildByPath('Status')?.getComponent(Label);
            if (label) label.string = state.phase === 'loading' ? '正在启动场景' : state.progress?.ratio == null ? '准备资源中' : `已加载 ${state.progress.completed}/${state.progress.total}`;
            const bar = overlay.getChildByPath('Progress')?.getComponent(ProgressBar); if (bar) bar.progress = state.progress?.ratio ?? 0;
        });
        this.cleanups.push(unsubscribe);
        try {
            if (this.sceneKind === 'Boot') { this.button('Continue', () => this.changeScene('Login')); await this.changeScene('Login'); }
            else if (this.sceneKind === 'Login') this.loginScene();
            else if (this.sceneKind === 'RoleSelect') await this.roleScene();
            else if (this.sceneKind === 'Game') await this.gameScene();
        } catch (error) { this.error(error); }
    }

    update(dt: number): void {
        this.world?.update(dt, session.role?.job ?? 0, !!session.inventory?.equipped?.some((entry: any) => entry.slot === 0));
        if (!session.ready || !this.world) return;
        this.moveTimer += dt;
        if (this.moveTimer < .15) return;
        this.moveTimer = 0;
        const direction = this.nav.state.direction;
        if (direction.x || direction.y) {
            const x = direction.x, y = -direction.y;
            const facing = [[0,-1],[1,-1],[1,0],[1,1],[0,1],[-1,1],[-1,0],[-1,-1]].findIndex(v => v[0] === x && v[1] === y);
            session.connection.send(20006, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, direction: facing });
        }
    }

    onDestroy(): void {
        this.disposed = true;
        this.world?.dispose();
        for (const [node, handler] of this.buttons) if (isValid(node, true)) node.off(Button.EventType.CLICK, handler, this);
        for (const cleanup of this.cleanups) cleanup();
        game.off(Game.EVENT_HIDE, this.releaseInput, this);
        if (typeof window !== 'undefined') window.removeEventListener('blur', this.releaseInput);
    }

    private loginScene(): void {
        const login = async (register: boolean): Promise<void> => {
            if (!/^[A-Za-z0-9_]{6,20}$/.test(this.edit('Account').string.trim()) || !/^[A-Za-z0-9_]{6,20}$/.test(this.edit('Password').string)) throw new Error('账号和密码需6～20位字母、数字或下划线');
            this.status('正在认证');
            const result = await session.connection.rpc(register ? 10009 : 10001, { account: this.edit('Account').string.trim(), password: this.edit('Password').string });
            const account = Array.from(result.accountId as Uint8Array, value => value.toString(16).padStart(2, '0')).join('');
            session.pending = new PendingOperations(sys.localStorage, `minimir.pending.v1.${account}.1`);
            session.connection.token = result.accountToken; session.lastError = '';
            this.edit('Password').string = '';
            await this.changeScene('RoleSelect');
        };
        this.button('Login', () => login(false)); this.button('Register', () => login(true));
    }

    private async roleScene(): Promise<void> {
        if (!session.connection.token) { await this.changeScene('Login'); return; }
        for (let i = 0; i < 5; i++) this.button(`Role${i}`, () => { this.selectedRole = i; this.refreshRoles(); });
        for (let i = 0; i < 3; i++) this.button(`JobTabs/Job${i}`, () => { this.selectedJob = i; this.status(`创建职业：${names[i]}`); });
        this.button('Logout', async () => { session.connection.token = ''; await this.changeScene('Login'); });
        this.button('Create', async () => {
            const free = [1,2,3,4,5].find(slot => !session.roles.some(role => role.slot === slot));
            if (free === undefined) throw new Error('五个角色槽已满');
            await session.connection.rpc(10005, { originServerId: Long.UONE, slot: free, job: this.selectedJob, name: this.edit('Name').string.trim(), opId: newOperationId() });
            await this.fetchRoles(); this.selectedRole = session.roles.findIndex(role => role.slot === free); this.refreshRoles();
        });
        this.button('Enter', async () => {
            const role = session.roles[this.selectedRole]; if (!role) throw new Error('先选择一个角色');
            session.preparedWorld?.dispose(); session.preparedWorld = null;
            const assets = await WorldAssets.prepare(role.job, (done, total) => {
                if (!this.disposed) this.status(`准备动作资源 ${done}/${total}`);
            });
            if (this.disposed) { assets.dispose(); return; }
            session.preparedWorld = assets;
            try {
                // 动作资源准备完成才取短票，避免网络加载占用入场基线的确认期限。
                this.status('正在获取入场票据');
                const ticket = await session.connection.rpc(10007, { role: role.key });
                session.role = role; session.ready = false; session.entry = null; session.baseline = null;
                await session.connection.connect(ticket.ticket);
                await session.scenes!.switchTo('Game');
            } catch (error) {
                session.connection.close();
                if (session.preparedWorld === assets) { assets.dispose(); session.preparedWorld = null; }
                throw error;
            }
        });
        await this.fetchRoles();
    }

    private async fetchRoles(): Promise<void> {
        session.roles = (await session.connection.rpc(10003, { originServerId: Long.UONE })).roles;
        if (this.selectedRole < 0 && session.roles.length) this.selectedRole = 0;
        this.refreshRoles();
    }

    private refreshRoles(): void {
        for (let i = 0; i < 5; i++) {
            const role = session.roles[i];
            this.text(`Role${i}/Label`, role ? `${i === this.selectedRole ? '● ' : ''}${role.name}　${names[role.job]}　${role.level}级` : '空角色槽');
        }
    }

    private async gameScene(): Promise<void> {
        if (!session.entry || !session.role) { await this.changeScene('Login'); return; }
        this.ui = this.find('UIMgr').getComponent(UIMgr)!; this.popup = this.find('PopupView').getComponent(PopupView)!;
        const safe = this.node.getChildByName('Safe')!;
        this.hud = safe.getComponent(GameHUD)!;
        this.messages = safe.getComponent(GameMessages)!;
        if (!this.hud || !this.messages) throw new Error('场景尚未保存HUD与消息组件');
        const selectedMessage = (value: { title: string; message: string }): void => { this.popup!.show('Details', value); };
        safe.on(GameMessages.EVENT_SELECTED, selectedMessage, this);
        this.cleanups.push(() => { if (isValid(safe, true)) safe.off(GameMessages.EVENT_SELECTED, selectedMessage, this); });
        this.button('BottomHUD/Footer/ChangeRole', () => this.leave(false));
        const viewport = this.find('MapViewport'); const world = viewport.getChildByName('World')!;
        this.world = new GameWorldView(viewport, world, world.getChildByName('Entities')!, session.role.name, session.role.job);
        const prepared = session.preparedWorld;
        if (!prepared) throw new Error('入场前尚未准备动作资源，请返回选角重试');
        session.preparedWorld = null;
        this.hud.setMapFrame(prepared.background);
        this.world.prepare(prepared, entity => this.popup!.show('Details', { title: '实体详情', message: `类型：${entity.entityKind}\n位置：${entity.position.x}, ${entity.position.y}\n生命：${entity.hp.toString()} / ${entity.hpMax.toString()}` }));
        if (this.disposed) return;
        viewport.getComponent(MapTouchNavigator)!.bind(this.nav, point => this.world!.screenToGrid(point));
        this.joystick = this.find('MapViewport/Joystick').getComponent(TouchJoystick)!;
        this.joystick.bind(this.nav);
        game.on(Game.EVENT_HIDE, this.releaseInput, this);
        if (typeof window !== 'undefined') window.addEventListener('blur', this.releaseInput);
        this.cleanups.push(this.nav.subscribe((state, reason) => {
            if (!session.ready) return;
            if (reason === 'ground' && state.target) session.connection.send(20006, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, destination: state.target });
            else if (reason === 'release') session.connection.send(20006, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, releaseJoystick: true });
        }));
        this.ui.node.on(UIMgr.EVENT_CHANGED, this.releaseInput, this); this.popup.node.on(PopupView.EVENT_CHANGED, this.releaseInput, this);
        this.cleanups.push(() => { if (this.ui && isValid(this.ui.node, true)) this.ui.node.off(UIMgr.EVENT_CHANGED, this.releaseInput, this); if (this.popup && isValid(this.popup.node, true)) this.popup.node.off(PopupView.EVENT_CHANGED, this.releaseInput, this); });
        this.bindGameButtons();
        const refresh = (): void => this.refreshGame(); session.update.add(refresh); this.cleanups.push(() => session.update.delete(refresh));
        this.refreshGame();
        // 地图与可编辑场景已真实准备好后才确认当前连接的完整基线。
        if (!session.baseline) await new Promise<void>((resolve, reject) => {
            const timer = setTimeout(() => { session.update.delete(check); reject(new Error('等待场景基线超时')); }, 10000);
            const check = (): void => { if (session.baseline) { clearTimeout(timer); session.update.delete(check); resolve(); } }; session.update.add(check);
        });
        if (this.disposed) return;
        session.connection.send(4, { sceneEpoch: session.entry.sceneEpoch, baselineId: session.entry.baselineId });
        this.status('已确认地图基线，等待服务端就绪');
    }

    private bindGameButtons(): void {
        for (const panel of ['Role','Inventory','Hosting','Report','Messages','Settings']) {
            this.button(`UIMgr/Panels/Functions/Pages/Page0/${panel}`, () => { this.ui!.open(panel); this.refreshGame(); });
        }
        for (const [path, panel] of [['BottomHUD/QuickNav/Inventory','Inventory'],['BottomHUD/QuickNav/Role','Role'],['BottomHUD/QuickNav/Hosting','Hosting'],['BottomHUD/QuickNav/Report','Report'],['BottomHUD/Footer/Functions','Functions'],['BottomHUD/Skills/Settings','Settings'],['BottomHUD/Footer/Inventory','Inventory'],['BottomHUD/Footer/Map','Role'],['TopHUD/Avatar','Role'],['BottomHUD/Chat','Messages']] as const) this.button(path, () => { this.ui!.open(panel); this.refreshGame(); });
        const yuanbao = (value: Long): string => `${value.div(10)}.${value.mod(10)}`;
        this.button('TopHUD/WalletButton', () => this.popup!.show('Details', { title: '四钱包', message: session.wallet ? `金币 ${session.wallet.gold}\n绑定金币 ${session.wallet.boundGold}\n元宝 ${yuanbao(session.wallet.yuanbaoUnits)}\n绑定元宝 ${yuanbao(session.wallet.boundYuanbaoUnits)}` : '等待余额同步' }));
        this.button('MapViewport/Actions/Auto', () => { this.auto = this.find('MapViewport/Actions/Auto').getComponent(Toggle)!.isChecked; return this.autoState(); });
        this.button('MapViewport/Actions/Count', () => { this.count = this.count === 1 ? 4 : this.count === 4 ? 8 : 1; return this.autoState(); });
        for (const path of ['MapViewport/Actions/Attack','BottomHUD/Skills/BasicAttack']) this.button(path, async () => {
            if (!session.ready) throw new Error('场景尚未就绪');
            const player = session.baseline?.entities.find((entity: any) => entity.entityKind === 1);
            if (player?.hp.isZero()) {
                if (!session.death || !(session.death.allowedActions & 1)) throw new Error('当前场景暂未开放复活');
                if (await this.popup!.confirm('Confirm', '返回出生点复活？', '角色已倒下')) { await this.economic(30002, { deathId: session.death.deathId, action: 1 }); session.death = null; }
                return;
            }
            const target = session.baseline?.entities.find((entity: any) => entity.entityKind === 3 && !entity.hp.isZero());
            if (target) session.connection.send(30003, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, skillId: 0, targetHandle: target.entityHandle, targetGrid: target.position });
        });
        for (const [path, title, message] of [
            ['BottomHUD/Skills/RedPotion','金创药','背包数量显示在快捷栏。药品使用暂未开放。'],
            ['BottomHUD/Skills/BluePotion','魔法药','背包数量显示在快捷栏。药品使用暂未开放。'],
            ['BottomHUD/Skills/SkillsPanel','技能','当前可以手动普攻或开启自动战斗。职业技能暂未开放。'],
            ['BottomHUD/Skills/PetPanel','宠物','宠物会跟随角色并参与战斗。宠物管理暂未开放。'],
            ['BottomHUD/QuickNav/Boss','BOSS挑战','BOSS挑战暂未开放。'],
            ['BottomHUD/QuickNav/Quest','任务','任务系统暂未开放。'],
            ['MapViewport/Mission','战斗状态','点击空地寻路，推动摇杆接管方向。自动战斗与离线托管分别设置。'],
        ] as const) this.button(path, () => this.popup!.show('Help', { title, message }));
        this.button('UIMgr/Panels/Settings/Help', () => this.popup!.show('Help', { title: '操作帮助', message: '左下摇杆控制八方向，轻触空地寻路。打开面板和聊天保留地面目标，主动方向接管才终止原寻路。点击半透明背景返回。' }));
        this.button('UIMgr/Panels/Hosting/Leave', () => this.leave(true));
        this.button('UIMgr/Panels/Hosting/Claim', () => this.economic(40006, { benefitKind: 0 }));
        for (const panel of ['Inventory','Role']) {
            if (panel === 'Role') { this.find('UIMgr/Panels/Role/Equip').active = false; this.find('UIMgr/Panels/Role/Recycle').active = false; continue; }
            this.button(`UIMgr/Panels/${panel}/Equip`, () => {
                if (!this.selectedItem) throw new Error('先选择物品');
                const definition = this.world!.gameplay.items.find((item: any) => item.item_id === this.selectedItem.itemId);
                if (!definition || definition.equip_slot == null) throw new Error('该物品不能装备');
                return this.economic(40001, { itemInstanceId: this.selectedItem.instanceId, slot: definition.equip_slot });
            });
            this.button(`UIMgr/Panels/${panel}/Recycle`, async () => {
                if (!this.selectedItem) throw new Error('先选择物品');
                const definition = this.world!.gameplay.items.find((item: any) => item.item_id === this.selectedItem.itemId);
                if (await this.popup!.confirm('Confirm', `分解${definition?.name ?? '所选装备'}？分解所得继承装备绑定状态。`)) await this.economic(40002, { itemInstanceIds: [this.selectedItem.instanceId] });
            });
        }
    }

    private refreshGame(): void {
        if (this.disposed || !this.world) return;
        const baseline = session.baseline;
        if (session.hosting) this.auto = session.hosting.autoBattleEnabled;
        if (baseline) this.count = baseline.entities.filter((entity: any) => entity.entityKind === 3).length;
        if (baseline && this.lastBaseline !== baseline.stateSequence.toString()) {
            this.lastBaseline = baseline.stateSequence.toString(); this.world.apply(baseline);
            const player = baseline.entities.find((entity: any) => entity.entityKind === 1);
            if (player) {
                this.nav.completeGroundTarget(player.position);
            }
        }
        const presentation = { role: session.role, wallet: session.wallet, inventory: session.inventory, baseline,
            hosting: session.hosting, report: session.report, ready: session.ready, lastError: session.lastError,
            auto: this.auto, count: this.count, gameplay: this.world.gameplay, map: this.world.map };
        this.hud?.refresh(presentation);
        this.messages?.refresh({ ...presentation, roleIdentity: roleIdentity(), entry: session.entry });
        if (this.lastInventory !== session.inventory && this.selectedItem && !session.inventory?.items?.some((item: any) => item.instanceId.toString() === this.selectedItem.instanceId.toString())) this.selectedItem = null;
        if (this.lastInventory !== session.inventory) for (const panel of ['Inventory','Role']) {
            const list = this.find(`UIMgr/Panels/${panel}/List`).getComponent(List)!;
            const items = panel === 'Inventory' ? session.inventory?.items ?? [] : (session.inventory?.equipped ?? []).map((entry: any) => entry.item);
            list.setItems(items, (node, item: any) => { const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId); node.getChildByName('Text')!.getComponent(Label)!.string = `${definition?.name ?? item.itemId} ×${item.quantity} ${item.bound ? '绑定' : '非绑定'}`; }, true);
            if (!(list as any).__boundSelection) { (list as any).__boundSelection = true; list.node.on('list-item-click', (_index: number, item: any) => { this.selectedItem = item; const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId); this.popup!.show('Details', { title: definition?.name ?? '物品详情', message: `数量 ${item.quantity}\n${item.bound ? '绑定物品' : '非绑定物品'}` }); }); }
        }
        this.lastInventory = session.inventory;
        for (const panel of ['Inventory','Role']) {
            const selected = this.world.gameplay.items.find((row: any) => row.item_id === this.selectedItem?.itemId);
            this.text(`UIMgr/Panels/${panel}/Message`, this.selectedItem ? `当前选择：${selected?.name ?? '物品'}` : '点击条目查看详情并选择物品');
        }
        this.text('UIMgr/Panels/Hosting/Message', `剩余托管 ${session.hosting?.remainHostingSec ?? 0} 秒\n在线自动战斗不扣托管时间`);
        const report = session.report;
        this.text('UIMgr/Panels/Report/Message', report ? `托管 ${report.usedSeconds} 秒，击杀 ${report.kills}\n经验 ${report.experience}\n收益已按事件时状态确认` : '暂无已提交托管战报');
        this.status(session.lastError || (session.ready ? '' : '正在同步场景'));
        if (session.ready && !this.reconciliationStarted) {
            this.reconciliationStarted = true;
            try { if (session.pending?.get(roleIdentity())) void this.submitPending().catch(error => this.error(error)); }
            catch (error) { this.unresolved = true; this.error(error); }
        }
    }

    private async autoState(): Promise<void> {
        try {
            if (!session.ready) throw new Error('场景尚未就绪');
            await session.connection.request(30004, { sceneEpoch: session.entry.sceneEpoch, enabled: this.auto, monsterCount: this.count });
        } finally {
            // Toggle会先翻转自身；拒绝或断线时也立即恢复服务端确认的状态。
            this.refreshGame();
        }
    }

    private async economic(id: number, value: any): Promise<void> {
        if (this.economicBusy || this.unresolved) throw new Error('上一笔操作正在核对，请等待回执');
        if (!session.pending || session.pending.get(roleIdentity())) throw new Error('上一笔操作需要沿原身份核对');
        const payload = { ...value, operation: operation() };
        try { session.pending.set(roleIdentity(), { id, value: payload }); }
        catch (error) { session.command = payload.operation.commandSeq; throw error; }
        await this.submitPending();
    }

    private async submitPending(): Promise<void> {
        const role = roleIdentity();
        const ledger = session.pending;
        const pending = ledger?.get(role);
        if (!pending || this.economicBusy) return;
        this.economicBusy = true;
        let result: any;
        try {
            result = await session.connection.request(pending.id, pending.value);
        } catch (error) { this.unresolved = true; throw error; }
        finally { this.economicBusy = false; }
        const expected = (pending.value.operation as { opId: Uint8Array }).opId;
        if (!(result.opId instanceof Uint8Array) || result.opId.length !== expected.length || !expected.every((value, index) => value === result.opId[index])) {
            this.unresolved = true;
            throw new Error('操作回执身份不匹配，保留原请求等待核对');
        }
        if (result.pending) { this.unresolved = true; throw new Error('操作仍在处理中，请重新认证查询结果'); }
        ledger!.clear(role); this.unresolved = false;
        if (result.errorCode) throw new Error(`操作未完成：${result.errorCode}`);
        this.status('操作已由服务端确认');
    }

    private async leave(hosting: boolean): Promise<void> {
        if (hosting && !session.hosting?.remainHostingSec) throw new Error('先领取免费托管时长');
        this.nav.exitGame(); session.connection.close(); session.ready = false;
        if (!hosting) await this.changeScene('RoleSelect'); else { session.connection.token = ''; await this.changeScene('Login'); }
    }

    private async changeScene(name: string): Promise<void> { try { await session.scenes!.switchTo(name); } catch (error) { this.error(error); } }
    private find(path: string): Node { const value = this.node.getChildByPath(`Safe/${path}`); if (!value) throw new Error(`场景缺少已保存引用：${path}`); return value; }
    private edit(path: string): EditBox { return this.find(path).getComponent(EditBox)!; }
    private text(path: string, value: string): void { const label = this.find(path).getComponent(Label); if (label) label.string = value; }
    private status(message: string): void { const label = this.node.getChildByPath('Safe/Status')?.getComponent(Label); if (label) label.string = message; }
    private error(error: unknown): void { const message = error instanceof Error ? error.message : String(error); session.lastError = message; this.status(message); }
    private button(path: string, action: () => unknown): void {
        const node = this.find(path); const handler = (): void => {
            if (this.uiActionBusy) {
                if (node.getComponent(Toggle)) this.refreshGame();
                return;
            }
            try {
                session.lastError = '';
                const result = action();
                if (result instanceof Promise) { this.uiActionBusy = true; result.catch(error => this.error(error)).finally(() => { this.uiActionBusy = false; }); }
            } catch (error) { this.error(error); }
        };
        node.on(Button.EventType.CLICK, handler, this); this.buttons.push([node, handler]);
    }
}
