import { _decorator, Button, Component, director, EditBox, game, Game, isValid, Label, Node, profiler, ProgressBar, sys, Toggle, UITransform } from 'cc';
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
import { newOperationId, ProtocolResponseError } from './GameConnection';
import { currentEntryIdentity, isCurrentEntry, operation, roleIdentity, session } from './GameSession';
import { PendingOperations } from './PendingOperations';
import { GameWorldView } from './GameWorldView';
import { WorldAssets } from './WorldAssets';
import { GameHUD } from './GameHUD';
import { GameMessages } from './GameMessages';
import { createPotionPreview, potionInstanceId, PotionSnapshot, validatePotionPreview } from './PotionModel';
import { protocolErrorMessage } from './GameErrors';
import { GameSkills } from './GameSkills';
import { createSkillLearnPreview, gridDirection, isDirectionalSkill, isSummonRecall, needsCastingSupply, isGroundSkill, primarySkill, selectSkillTarget, skillActivation, skillMode, skillQuickStateLabel, skillRows, SkillRow, SkillSnapshot, skillBookInstance, validateCaster, validateDirectionTarget, validateGroundTarget, validateSkillLearnPreview } from './SkillModel';
import type { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import { traceInput } from './InputTrace';
import { GameSkillAssets } from './GameSkillAssets';
import { canRecycleBook, pageMergeGroup, preparePageMerge, PreparedSkillAsset, SkillAssetQuote, SkillAssetsSnapshot, sourceName, validatePageMergeQuote, validateQuotedMaterials, validateSkillAssetContext } from './SkillAssetsModel';
import { GameCombatStatus } from './GameCombatStatus';
import { captureSupply, CastingSupplySnapshot, selectedCastingSupply, SupplySelection, validateSupply } from './CastingSupplyModel';
import { MapAttachmentCoordinator, StaleMapAttachmentError } from './MapAttachmentCoordinator';
import { attachmentKey } from './MapTravelModel';
import type { AttachmentIdentity, TravelContext } from './MapTravelModel';
import { MapViewportPreparation } from './MapViewportPreparation';
import { roleBindingText } from './RoleBindingModel';
import { petAppearDuration } from './PetBornModel';
import { GameAutoRecycle } from './GameAutoRecycle';
import { AutoRecycleSnapshot, autoRecycleConfirmation, autoRecycleReportLines, createAutoRecyclePreview, manualRecycleBlockReason, recycleItemKey, validateAutoRecyclePreview } from './AutoRecycleModel';
import { equipmentLevelLines, equipmentLevelReason, progressionDetailLines, progressionSummary } from './LevelGrowthModel';

const { ccclass, property } = _decorator;
const names = ['男战士', '女法师', '男道士'];

@ccclass('AppRoot')
export class AppRoot extends Component {
    @property sceneKind = 'Boot';
    private readonly buttons: Array<[Node, string, () => void]> = [];
    private readonly cleanups: Array<() => void> = [];
    private readonly mapWaiters = new Set<() => void>();
    private nav = new NavigationIntent();
    private world?: GameWorldView;
    private hud?: GameHUD;
    private messages?: GameMessages;
    private skills?: GameSkills;
    private skillAssets?: GameSkillAssets;
    private autoRecycle?: GameAutoRecycle;
    private combatStatus?: GameCombatStatus;
    private ui?: UIMgr;
    private popup?: PopupView;
    private selectedRole = -1;
    private selectedJob = 0;
    private selectedItem: any = null;
    private selectedTarget: number | null = null;
    private aim: { skillId: number; role: string; scene: string; rule: string; supply?: SupplySelection } | null = null;
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
    private attachment?: MapAttachmentCoordinator<WorldAssets, GameWorldView>;
    private attached?: AttachmentIdentity;
    private attachmentTarget = '';
    private attachmentTask?: Promise<GameWorldView>;
    private attaching = false;
    private mapLoading = false;
    private navigationUnsubscribe?: () => void;
    private inputBarrier = false;
    private inputEnabled: { map: boolean; joystick: boolean } | null = null;
    private preparedTravel?: { id: string; assets: WorldAssets; viewport: MapViewportPreparation };
    private travelSource?: TravelContext;
    private travelDestination = 1;
    private travelUiSequence = 0;
    private enterAttempt = 0;
    private leaving = false;
    private releaseInput = (): void => { if (this.joystick) this.joystick.release(); else this.nav.releaseJoystick(); };

    async start(): Promise<void> {
        profiler.hideStats();
        session.scenes ??= new SceneManager(new SceneLoader(new CreatorSceneBackend(director)), ['Boot', 'Login', 'RoleSelect', 'Game'], this.sceneKind);
        const unsubscribe = session.scenes.subscribe(state => {
            const overlay = this.node.getChildByPath('Loading'); if (!overlay) return;
            overlay.active = this.mapLoading || state.phase === 'preloading' || state.phase === 'loading';
            if (this.mapLoading) return;
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
        this.combatStatus?.update();
        if (!this.canSendInput()) return;
        this.moveTimer += dt;
        if (this.moveTimer < .15) return;
        this.moveTimer = 0;
        const direction = this.nav.state.direction;
        if (direction.x || direction.y) {
            const facing = this.world!.logicalDirection(direction.x, direction.y);
            if (facing !== null) session.connection.send(20006, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, direction: facing });
        }
    }

    onDestroy(): void {
        this.disposed = true;
        for (const cancel of Array.from(this.mapWaiters)) cancel();
        this.enterAttempt++; this.attachment?.dispose(); this.world?.dispose(); this.releasePreparedTravel(); this.navigationUnsubscribe?.();
        for (const [node, eventType, handler] of this.buttons) if (isValid(node, true)) node.off(eventType, handler, this);
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
            const attempt = ++this.enterAttempt, token = session.connection.token;
            session.preparedWorld?.dispose(); session.preparedWorld = null;
            const assets = await WorldAssets.prepare(role.job, (done, total) => {
                if (!this.disposed) this.status(`准备动作资源 ${done}/${total}`);
            });
            if (this.disposed || this.enterAttempt !== attempt || session.connection.token !== token) { assets.dispose(); return; }
            session.preparedWorld = assets;
            try {
                // 动作资源准备完成才取短票，避免网络加载占用入场基线的确认期限。
                this.status('正在获取入场票据');
                const ticket = await session.connection.rpc(10007, { role: role.key });
                if (this.disposed || this.enterAttempt !== attempt || session.connection.token !== token) throw new StaleMapAttachmentError('旧选角取票已经失效');
                session.role = role; session.ready = false; session.entry = null; session.baseline = null;
                await session.connection.connect(ticket.ticket);
                if (this.disposed || this.enterAttempt !== attempt || session.role !== role || !currentEntryIdentity()) throw new StaleMapAttachmentError('旧选角入场已经失效');
                await session.scenes!.switchTo('Game');
            } catch (error) {
                if (!this.disposed && this.enterAttempt === attempt) session.connection.close();
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
        this.attachment = new MapAttachmentCoordinator<WorldAssets, GameWorldView>();
        this.cleanups.push(() => this.combatStatus?.dispose());
        this.skills = new GameSkills(safe, this.popup, (title, lines) => this.combatStatus!.showDetails(title, lines));
        this.cleanups.push(() => this.skills?.dispose());
        this.skillAssets = new GameSkillAssets(safe, this.popup, error => this.error(error));
        this.cleanups.push(() => this.skillAssets?.dispose());
        this.autoRecycle = new GameAutoRecycle(this.find('UIMgr/Panels/Settings'));
        const cast = (event: CastEvent): void => { if (this.eventMatchesWorld(event.sceneEpoch)) this.world?.acceptCast(event); };
        session.casts.add(cast); this.cleanups.push(() => session.casts.delete(cast));
        const castStep = (event: CastStepEvent): void => { if (this.eventMatchesWorld(event.sceneEpoch)) this.world?.acceptCastStep(event); };
        session.castSteps.add(castStep); this.cleanups.push(() => session.castSteps.delete(castStep));
        const attack = (event: AttackEvent): void => { if (this.eventMatchesWorld(event.sceneEpoch)) this.world?.acceptAttack(event); };
        session.attacks.add(attack); this.cleanups.push(() => session.attacks.delete(attack));
        const impact = (event: ImpactEvent): void => { if (this.eventMatchesWorld(event.sceneEpoch)) this.world?.acceptImpact(event); };
        session.impacts.add(impact); this.cleanups.push(() => session.impacts.delete(impact));
        const displacement = (event: Parameters<GameWorldView['acceptDisplacement']>[0]): void => { if (this.eventMatchesWorld(event.sceneEpoch)) this.world?.acceptDisplacement(event); };
        session.displacements.add(displacement); this.cleanups.push(() => session.displacements.delete(displacement));
        if (this.disposed) return;
        this.joystick = this.find('MapViewport/Joystick').getComponent(TouchJoystick)!;
        this.setInputBarrier(true);
        game.on(Game.EVENT_HIDE, this.releaseInput, this);
        if (typeof window !== 'undefined') window.addEventListener('blur', this.releaseInput);
        this.ui.node.on(UIMgr.EVENT_CHANGED, this.releaseInput, this); this.popup.node.on(PopupView.EVENT_CHANGED, this.releaseInput, this);
        this.cleanups.push(() => { if (this.ui && isValid(this.ui.node, true)) this.ui.node.off(UIMgr.EVENT_CHANGED, this.releaseInput, this); if (this.popup && isValid(this.popup.node, true)) this.popup.node.off(PopupView.EVENT_CHANGED, this.releaseInput, this); });
        this.bindGameButtons();
        const refresh = (): void => this.handleSessionUpdate(); session.update.add(refresh); this.cleanups.push(() => session.update.delete(refresh));
        await this.attachCurrentEntry();
    }

    private bindGameButtons(): void {
        for (const name of ['AutoRecycleEnabled', 'RecycleProtectBound', 'RecycleProtectUnbound', ...[0, 1, 2, 3, 4].map(id => 'RecycleQuality' + id)])
            this.button('UIMgr/Panels/Settings/' + name, () => { this.autoRecycle!.markDirty(); this.refreshGame(); }, Toggle.EventType.TOGGLE);
        for (const name of ['RecycleMaxLevel', 'RecycleProtectedTypes']) this.button('UIMgr/Panels/Settings/' + name,
            () => { this.autoRecycle!.markDirty(); }, EditBox.EventType.TEXT_CHANGED);
        this.button('UIMgr/Panels/Settings/RecycleReset', () => { this.autoRecycle!.reset(); this.refreshGame(); });
        this.button('UIMgr/Panels/Settings/RecycleSave', () => this.saveAutoRecycle(false));
        this.button('UIMgr/Panels/Settings/RecycleApply', () => this.saveAutoRecycle(true));
        this.button('UIMgr/Panels/Inventory/ItemLock', () => this.setItemLock());
        this.button('UIMgr/Panels/Role/Message', () => {
            const identity = currentEntryIdentity(); if (!identity || !session.role) return;
            this.combatStatus?.showLiveDetails('角色成长', () => !this.disposed && isCurrentEntry(identity)
                ? progressionDetailLines(session.inventory?.level, session.inventory?.experience, session.inventory?.progression) : null);
        }, Node.EventType.TOUCH_END);
        for (const panel of ['Role','Inventory','Hosting','Report','Messages','Settings']) {
            this.button(`UIMgr/Panels/Functions/Pages/Page0/${panel}`, () => { this.ui!.open(panel); this.refreshGame(); });
        }
        for (const [path, panel] of [['BottomHUD/QuickNav/Inventory','Inventory'],['BottomHUD/QuickNav/Role','Role'],['BottomHUD/QuickNav/Hosting','Hosting'],['BottomHUD/QuickNav/Report','Report'],['BottomHUD/Footer/Functions','Functions'],['BottomHUD/Skills/Settings','Settings'],['BottomHUD/Footer/Inventory','Inventory'],['TopHUD/Avatar','Role'],['BottomHUD/Chat','Messages']] as const) this.button(path, () => { this.ui!.open(panel); this.refreshGame(); });
        this.button('BottomHUD/Footer/Map', () => this.travelMap());
        const yuanbao = (value: Long): string => `${value.div(10)}.${value.mod(10)}`;
        this.button('TopHUD/WalletButton', () => this.popup!.show('Details', { title: '四钱包', message: session.wallet ? `金币 ${session.wallet.gold}\n绑定金币 ${session.wallet.boundGold}\n元宝 ${yuanbao(session.wallet.yuanbaoUnits)}\n绑定元宝 ${yuanbao(session.wallet.boundYuanbaoUnits)}` : '等待余额同步' }));
        this.button('MapViewport/Actions/Auto', () => { this.auto = this.find('MapViewport/Actions/Auto').getComponent(Toggle)!.isChecked; return this.autoState(); }, Toggle.EventType.TOGGLE);
        this.button('MapViewport/Actions/Count', () => { this.count = this.count === 1 ? 4 : this.count === 4 ? 8 : 1; return this.autoState(); });
        this.button('BottomHUD/Skills/RedPotion', () => this.usePotion(35));
        this.button('BottomHUD/Skills/BluePotion', () => this.usePotion(36));
        this.button('MapViewport/Actions/Attack', () => this.basicAttack());
        this.button('BottomHUD/Skills/BasicAttack', () => {
            if (session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle)?.hp.isZero()) return this.basicAttack();
            const skill = this.quickSkill();
            if (!skill) return this.basicAttack();
            if (!skill.learned) { this.ui!.open('Skills'); this.refreshGame(); return; }
            return this.castSkill(skill);
        });
        this.button('BottomHUD/Skills/SkillsPanel', () => { this.ui!.open('Skills'); this.refreshGame(); });
        this.button('UIMgr/Panels/Skills/Learn', () => this.learnSkill());
        this.button('UIMgr/Panels/Skills/Use', () => {
            const skill = this.skills?.selected; if (!skill) throw new Error('先选择技能');
            return this.castSkill(skill);
        });
        this.button('UIMgr/Panels/Skills/ModeToggle', () => {
            const skill = this.skills?.selected; if (!skill) throw new Error('先选择技能');
            const enabled = this.find('UIMgr/Panels/Skills/ModeToggle').getComponent(Toggle)!.isChecked;
            try { return this.castSkill(skill, enabled); }
            finally { this.refreshGame(); }
        }, Toggle.EventType.TOGGLE);
        this.button('UIMgr/Panels/Skills/BindQuick', () => {
            const selected = this.skills?.selected;
            if (!selected?.learned || !selected.definition.active) throw new Error('请选择已学会的主动技能');
            session.quickSkills.set(roleIdentity(), selected.definition.id); this.refreshGame();
        });
        this.button('MapViewport/Aiming/Cancel', () => this.cancelAim());
        this.button('MapViewport/BuffBar/Overview', () => this.combatStatus!.openOverview());
        this.button('UIMgr/Panels/Role/Equip', () => this.setCastingSupply(true));
        for (const [path, kind] of [['Shop', 1], ['Upgrade', 2], ['Salvage', 3]] as const) this.button(`UIMgr/Panels/Skills/${path}`, () => {
            const selected = this.skills?.selected;
            if (kind === 2 && !selected?.learned) throw new Error('先学会并选择要升级的技能');
            this.skillAssets!.open(kind, kind === 2 ? selected!.definition.id : 0); this.ui!.open('SkillAssets'); this.refreshGame();
        });
        this.button('UIMgr/Panels/SkillAssets/Reset', () => this.skillAssets!.reset());
        this.button('UIMgr/Panels/SkillAssets/Preview', () => this.previewSkillAsset());
        this.button('UIMgr/Panels/SkillAssets/PurchaseQuantity/Minus', () => this.skillAssets!.adjustPurchase(-1));
        this.button('UIMgr/Panels/SkillAssets/PurchaseQuantity/Plus', () => this.skillAssets!.adjustPurchase(1));
        for (const [path, title, message] of [
            ['BottomHUD/QuickNav/Boss','BOSS挑战','BOSS挑战暂未开放。'],
            ['BottomHUD/QuickNav/Quest','任务','任务系统暂未开放。'],
            ['MapViewport/Mission','战斗状态','点击空地寻路，推动摇杆接管方向。自动战斗与离线托管分别设置。'],
        ] as const) this.button(path, () => this.popup!.show('Help', { title, message }));
        this.button('BottomHUD/Skills/PetPanel', () => this.combatStatus?.showPet());
        this.button('UIMgr/Panels/Settings/Help', () => this.popup!.show('Help', { title: '操作帮助', message: '左下摇杆控制八方向，轻触空地寻路。打开面板和聊天保留地面目标，主动方向接管才终止原寻路。点击半透明背景返回。'
            + (session.role?.job === 0 ? '\n刺杀、半月需手动开启；烈火需手动充能。自动战斗仅使用已开启的模式和已就绪的烈火。' : '') }));
        this.button('UIMgr/Panels/Hosting/Leave', () => this.leave(true));
        this.button('UIMgr/Panels/Hosting/Claim', () => this.economic(40006, { benefitKind: 0 }));
        for (const panel of ['Inventory','Role']) {
            if (panel === 'Role') { this.find('UIMgr/Panels/Role/Equip').active = false; this.find('UIMgr/Panels/Role/Recycle').active = false; continue; }
            this.button(`UIMgr/Panels/${panel}/Equip`, () => {
                if (!this.selectedItem) throw new Error('先选择物品');
                const definition = this.world!.gameplay.items.find((item: any) => item.item_id === this.selectedItem.itemId);
                if (definition?.kind === 'casting_supply') return this.setCastingSupply(false);
                if (!definition || definition.kind !== 'equipment' || definition.equip_slot == null || definition.equip_slot === 100) throw new Error('该物品不能装备');
                const reason = equipmentLevelReason(definition, session.inventory?.level, session.role?.job); if (reason) throw new Error(reason);
                return this.economic(40001, { itemInstanceId: this.selectedItem.instanceId, slot: definition.equip_slot });
            });
            this.button(`UIMgr/Panels/${panel}/Recycle`, async () => {
                if (!this.selectedItem) throw new Error('先选择物品');
                const definition = this.world!.gameplay.items.find((item: any) => item.item_id === this.selectedItem.itemId);
                if (definition?.kind === 'book') { this.skillAssets!.open(3); this.ui!.open('SkillAssets'); this.refreshGame(); return; }
                if (definition?.kind !== 'equipment') throw new Error('该物品不能分解');
                const protectedReason = manualRecycleBlockReason(this.selectedItem); if (protectedReason) throw new Error(protectedReason);
                const captured = this.selectedItem, revision = session.inventory.revision.toString(), scope = currentEntryIdentity();
                if (await this.popup!.confirm('Confirm', `分解${definition?.name ?? '所选装备'}？分解所得继承装备绑定状态。`)) {
                    if (this.disposed) return;
                    const current = session.inventory?.items.find((row: any) => recycleItemKey(row.instanceId) === recycleItemKey(captured.instanceId));
                    if (!scope || !isCurrentEntry(scope) || session.inventory?.revision.toString() !== revision || !current || current.version.toString() !== captured.version.toString()
                        || manualRecycleBlockReason(current)) throw new Error('装备或保护状态已变化，请重新确认');
                    await this.economic(40002, { itemInstanceIds: [new Uint8Array(current.instanceId)] });
                }
            });
        }
    }

    private handleSessionUpdate(): void {
        if (this.disposed) return;
        if (!session.travel.active) { this.releasePreparedTravel(); this.travelSource = undefined; }
        const identity = currentEntryIdentity();
        if (!identity || !isCurrentEntry(identity)) {
            this.attachment?.invalidate(); this.attachmentTarget = ''; this.attachmentTask = undefined; this.attaching = false;
            this.setInputBarrier(true); this.releasePreparedTravel(); this.status(session.lastError || '连接已变化，请重新进入'); return;
        }
        if (!this.attached || attachmentKey(this.attached) !== attachmentKey(identity)) {
            void this.attachCurrentEntry().catch(error => { if (!(error instanceof StaleMapAttachmentError)) this.error(error); }); return;
        }
        this.setInputBarrier(this.attaching || !session.ready || session.travel.blocksInput || session.teleport.blocksInput);
        this.refreshGame();
    }

    private attachCurrentEntry(): Promise<GameWorldView> {
        const identity = currentEntryIdentity(), entry = session.entry, role = session.role;
        if (!identity || !entry || !role || !isCurrentEntry(identity) || !this.attachment) return Promise.reject(new StaleMapAttachmentError('入场身份已经变化'));
        const key = attachmentKey(identity);
        if (this.attachmentTarget === key && this.attachmentTask) return this.attachmentTask;
        this.attachmentTarget = key; this.attaching = true; this.setInputBarrier(true); this.showMapLoading(true, '准备目标地图');
        let prime: MapViewportPreparation | undefined;
        const task = this.attachment.attach(identity, {
            current: () => !this.disposed && session.entry === entry && isCurrentEntry(identity),
            load: async () => {
                let assets: WorldAssets | undefined;
                try {
                    if (this.preparedTravel?.id === identity.travelId && this.preparedTravel.assets.mapId === identity.map.toString()) {
                        assets = this.preparedTravel.assets; prime = this.preparedTravel.viewport; this.preparedTravel = undefined;
                    } else {
                        const warm = session.preparedWorld;
                        if (warm?.mapId === identity.map.toString()) { assets = warm; session.preparedWorld = null; }
                        else {
                            assets = await WorldAssets.prepare(role.job, (done, total) => {
                                if (!this.disposed && isCurrentEntry(identity)) this.showMapLoading(true, `准备地图资源 ${done}/${total}`, done, total);
                            }, entry.roomId);
                            if (warm && session.preparedWorld === warm) { session.preparedWorld = null; warm.dispose(); }
                        }
                    }
                    if (this.disposed || session.entry !== entry || !isCurrentEntry(identity)) throw new StaleMapAttachmentError('旧地图资源准备已经失效');
                    if (assets.mapId !== identity.map.toString() || String(assets.gameplay.rule_version) !== entry.resourceVersion.toString()) throw new Error('地图资源或规则版本与真实入场身份不一致');
                    await this.waitBaseline(identity); return assets;
                } catch (error) { prime?.release(); prime = undefined; assets?.dispose(); throw error; }
            },
            mount: assets => {
                session.progressionCurve = typeof assets.gameplay.progression?.curve_version === 'string' ? assets.gameplay.progression.curve_version : null;
                this.combatStatus?.dispose(); this.combatStatus = undefined;
                this.selectedTarget = null; this.aim = null; this.lastBaseline = ''; this.lastInventory = null; this.reconciliationStarted = false;
                const viewport = this.find('MapViewport'), world = viewport.getChildByName('World')!;
                const view = new GameWorldView(viewport, world, world.getChildByName('Entities')!, role.name, role.job, entry.sceneEpoch, entry.playerHandle);
                this.world = view; this.attached = identity;
                view.bindPresentation(identity, () => !this.disposed && !!session.role && this.world === view && isCurrentEntry(identity),
                    () => { if (session.role && isCurrentEntry(identity)) session.petBorn.discardPending(); });
                this.hud!.setMapFrame(assets.background, assets);
                view.prepare(assets, entity => {
                    if (!this.isWorldCurrent() || this.world !== view || !this.canSendInput()) return;
                    if (this.consumeGround(entity.position)) return;
                    this.selectedTarget = entity.entityHandle;
                    this.combatStatus?.showEntity(entity.entityHandle, entity.life.toString());
                });
                const combatStatus = new GameCombatStatus(this.node.getChildByName('Safe')!, this.ui!, this.popup!, assets.statusBadgePrefab, entry.playerHandle, assets.gameplay.statuses, { sceneEpoch: entry.sceneEpoch, baselineId: entry.baselineId, mapId: entry.roomId });
                this.combatStatus = combatStatus;
                const releaseStatus = assets.onBeforeInvalidation(reason => {
                    if (reason !== 'dispose') return;
                    combatStatus.dispose(); releaseStatus();
                    if (this.combatStatus === combatStatus) this.combatStatus = undefined;
                });
                this.bindNavigation(identity); return view;
            },
            prepare: async view => {
                const baseline = await this.waitBaseline(identity);
                view.apply(baseline); this.lastBaseline = baseline.stateSequence.toString(); this.nav.observeAuthority(baseline.navigationState);
                const sequence = baseline.navigationState?.inputSequence;
                if (Number.isInteger(sequence) && sequence >= 0 && sequence <= 0xffffffff) this.inputSequence = Math.max(this.inputSequence, sequence);
                const player = baseline.entities.find((value: any) => value.entityHandle === entry.playerHandle);
                if (!player) throw new Error('当前地图基线缺少玩家身份');
                await view.prepareViewport(player.position);
                if (!view.isMapReady) throw new Error('当前地图首视口尚未准备完成');
                prime?.release(); prime = undefined;
            },
            acknowledge: view => {
                if (!isCurrentEntry(identity) || session.entry !== entry || this.world !== view || !view.isMapReady
                    || session.baseline?.sceneEpoch.toString() !== identity.scene.toString() || session.baseline?.baselineId.toString() !== identity.baseline.toString()) throw new StaleMapAttachmentError('旧地图准备不得确认新场景');
                if (identity.travelId && !session.travel.acknowledge(identity)) throw new StaleMapAttachmentError('旅行附着确认代次已经变化');
                if (identity.serverTransitionId && !session.teleport.acknowledge(identity)) throw new StaleMapAttachmentError('瞬移附着确认代次已经变化');
                session.connection.send(4, { sceneEpoch: entry.sceneEpoch, baselineId: entry.baselineId });
            },
        });
        this.attachmentTask = task;
        void task.then(() => {
            if (!this.disposed && this.attachmentTarget === key && isCurrentEntry(identity)) {
                this.attaching = false; this.showMapLoading(false); this.handleSessionUpdate();
            }
        }, error => {
            prime?.release(); prime = undefined;
            if (!this.disposed && this.attachmentTarget === key && isCurrentEntry(identity)) {
                this.attaching = false; this.showMapLoading(false); this.setInputBarrier(true); this.error(error);
            }
        });
        return task;
    }

    private waitBaseline(identity: AttachmentIdentity): Promise<any> {
        const current = (): boolean => !this.disposed && isCurrentEntry(identity);
        const valid = (): boolean => !!session.baseline && session.baseline.sceneEpoch.toString() === identity.scene.toString() && session.baseline.baselineId.toString() === identity.baseline.toString();
        if (!current()) return Promise.reject(new StaleMapAttachmentError('等待基线的入场身份已失效'));
        if (valid()) return Promise.resolve(session.baseline);
        return new Promise((resolve, reject) => {
            let finished = false;
            const finish = (error?: Error): void => { if (finished) return; finished = true; clearTimeout(timer); session.update.delete(check); this.mapWaiters.delete(cancel); if (error) reject(error); else resolve(session.baseline); };
            const cancel = (): void => finish(new StaleMapAttachmentError('地图基线等待已取消'));
            const check = (): void => { if (!current()) finish(new StaleMapAttachmentError('旧基线等待已经失效')); else if (valid()) finish(); };
            const timer = setTimeout(() => finish(new Error('等待场景基线超时')), 10000); session.update.add(check); this.mapWaiters.add(cancel);
        });
    }

    private bindNavigation(identity: AttachmentIdentity): void {
        this.releaseInput(); this.navigationUnsubscribe?.(); this.nav = new NavigationIntent(); this.nav.setInputBlocked(true);
        const viewport = this.find('MapViewport');
        viewport.getComponent(MapTouchNavigator)!.bind(this.nav, point => this.canSendInput() ? this.world!.screenToGrid(point, !this.aim) : null,
            grid => this.canSendInput() && this.consumeGround(grid), grid => { if (this.canSendInput() && this.aim) this.world!.showAim(grid); });
        this.joystick!.bind(this.nav);
        this.navigationUnsubscribe = this.nav.subscribe((state, reason) => {
            if (!this.canSendInput() || !isCurrentEntry(identity)) return;
            if (reason === 'unreachable') {
                const navigation = session.baseline?.navigationState;
                this.messages?.navigationUnreachable(`${identity.role}:${identity.binding}:${identity.scene}:${navigation?.revision}:${navigation?.inputSequence}`);
            } else if (reason === 'ground' && state.target) {
                const inputSequence = ++this.inputSequence; this.nav.bindGroundInput(inputSequence);
                session.connection.send(20006, { sceneEpoch: identity.scene, inputSequence, destination: state.target });
            } else if (reason === 'release') session.connection.send(20006, { sceneEpoch: identity.scene, inputSequence: ++this.inputSequence, releaseJoystick: true });
        });
    }
    private isWorldCurrent(): boolean { return !!this.attached && !!this.world && isCurrentEntry(this.attached) && this.world.sceneEpoch.toString() === this.attached.scene.toString(); }
    private canSendInput(): boolean { return !this.disposed && !this.leaving && !this.attaching && !this.inputBarrier && session.ready && !session.travel?.blocksInput && !session.teleport?.blocksInput && this.isWorldCurrent(); }
    private eventMatchesWorld(scene: { toString(): string }): boolean { return this.isWorldCurrent() && this.attached!.scene.toString() === scene.toString(); }
    private setInputBarrier(blocked: boolean): void {
        this.nav.setInputBlocked(blocked || this.leaving);
        if (this.inputBarrier === blocked) return;
        const map = this.find('MapViewport').getComponent(MapTouchNavigator)!, joystick = this.joystick;
        if (!joystick) return;
        if (blocked) { this.releaseInput(); this.inputEnabled = { map: map.enabled, joystick: joystick.enabled }; map.enabled = false; joystick.enabled = false; }
        else { map.enabled = this.inputEnabled?.map ?? true; joystick.enabled = this.inputEnabled?.joystick ?? true; this.inputEnabled = null; }
        this.inputBarrier = blocked;
    }
    private showMapLoading(visible: boolean, text = '', completed?: number, total?: number): void {
        if (this.disposed) return; this.mapLoading = visible;
        const node = this.node.getChildByName('Loading'); if (!node) return; node.active = visible;
        if (text) { const label = node.getChildByName('Status')?.getComponent(Label); if (label) label.string = text; }
        if (completed !== undefined && total !== undefined && total > 0) { const bar = node.getChildByName('Progress')?.getComponent(ProgressBar); if (bar) bar.progress = completed / total; }
    }
    private travelContext(): TravelContext {
        const identity = currentEntryIdentity(), entry = session.entry;
        const player = session.baseline?.entities.find((value: any) => value.entityHandle === entry?.playerHandle);
        if (!identity || !entry || !player || !isCurrentEntry(identity)) throw new Error('旅行缺少当前角色与场景身份');
        return { connection: identity.connection, binding: identity.binding, role: identity.role, scene: identity.scene, baseline: identity.baseline, life: player.life, rule: entry.resourceVersion };
    }
    private validateTravel(): void {
        this.ensureNotLeaving();
        if (!session.ready || !this.isWorldCurrent() || this.attaching) throw new Error('当前地图尚未就绪');
        const player = session.baseline?.entities.find((value: any) => value.entityHandle === session.entry.playerHandle);
        if (!player || player.hp.isZero()) throw new Error('角色存活时才能往返城镇');
        if (this.economicBusy || this.unresolved || session.pending?.get(roleIdentity())) throw new Error('上一笔资产操作须先沿原身份核对');
    }
    private async travelMap(): Promise<void> {
        if (session.travel.phase === 'uncertain' && this.travelSource) { await this.reconcileTravel(this.travelSource); return; }
        this.validateTravel();
        if (this.world!.gameplay.town_travel?.enabled !== true) throw new Error('城镇往返尚未开放');
        if (session.travel.active) throw new Error('上一旅行计划尚未确定，请等待原计划状态或重新连接');
        const source = this.travelContext(), entry = session.entry, id = Array.from(newOperationId(), byte => byte.toString(16).padStart(2, '0')).join('');
        const destination = entry.sceneKind === 2 ? 2 : 1;
        const scope = { id, generation: session.travel.begin(source, id, destination), ui: ++this.travelUiSequence };
        const current = (): boolean => !this.disposed && this.travelUiSequence === scope.ui && session.travel.id === scope.id && session.travel.generation === scope.generation;
        let prepared: typeof this.preparedTravel;
        this.travelSource = source; this.travelDestination = destination;
        this.releaseInput(); this.showMapLoading(true, '申请目标地图计划');
        try {
            const plan = await session.connection.request(20007, { sceneEpoch: entry.sceneEpoch, travelId: id, destination });
            if (!current()) throw new StaleMapAttachmentError('旧旅行计划回调已经失效');
            if (plan.phase !== undefined) throw new Error('服务端已终止或处理本次旅行计划');
            session.travel.acceptPlan(plan, this.travelContext(), Date.now()); this.validateTravel();
            const assets = await WorldAssets.prepare(session.role.job, (done, total) => {
                if (current() && session.travel.active) this.showMapLoading(true, `准备目标资源 ${done}/${total}`, done, total);
            }, plan.targetMapId);
            const prime = new MapViewportPreparation(assets);
            let valid = false;
            try { valid = current() && session.travel.active && session.travel.matches(this.travelContext()); } catch { /* 原身份已丢失，仍须归还刚完成的资源。 */ }
            if (!valid) { assets.dispose(); throw new StaleMapAttachmentError('旧旅行资源准备已经失效'); }
            prepared = { id, assets, viewport: prime }; this.preparedTravel = prepared;
            if ((assets.sourceMap ? 2 : 1) !== plan.targetSceneKind) throw new Error('旅行目标地图类型与资源不匹配');
            const view = this.find('MapViewport').getComponent(UITransform)!, scale = this.world!.world.scale;
            await prime.prepare(plan.position, view.width / scale.x, view.height / scale.y);
            if (!current()) throw new StaleMapAttachmentError('旧旅行视口准备已经失效');
            this.validateTravel(); session.travel.prepared(this.travelContext(), Date.now()); this.showMapLoading(false);
            const accepted = await this.popup!.confirm('Confirm', destination === 1 ? '目标城镇资源已准备。确认前往城镇？原个人战斗现场将由服务端保留。' : '目标个人地图资源已准备。确认返回原战斗现场？', '地图往返');
            if (!current()) throw new StaleMapAttachmentError('旧旅行确认已经失效');
            if (!accepted) { await this.cancelTravel(id, source); return; }
            this.validateTravel(); this.releaseInput(); session.travel.commit(this.travelContext(), Date.now()); this.setInputBarrier(true);
            this.showMapLoading(true, '等待服务端收束并移交地图');
            await session.connection.request(20009, { sceneEpoch: entry.sceneEpoch, travelId: id, cancel: false });
            if (!current()) throw new StaleMapAttachmentError('旧旅行提交回调已经失效');
            await this.waitTravel(id, source);
        } catch (error) {
            if (!current()) throw new StaleMapAttachmentError('旧旅行回调已经失效');
            if (session.travel.active) {
                if (session.travel.phase === 'planning' && error instanceof ProtocolResponseError) { session.travel.reset(); scope.id = session.travel.id; scope.generation = session.travel.generation; }
                else if (session.travel.blocksInput) session.travel.uncertain();
                else { try { await this.cancelTravel(id, source); } catch { if (current()) session.travel.uncertain(); } }
            }
            if (!current()) throw new StaleMapAttachmentError('旧旅行取消回调已经失效');
            throw error;
        } finally {
            if (prepared && (!current() || !session.travel.blocksInput)) this.releasePreparedTravel(prepared);
            if (current()) { this.showMapLoading(false); this.setInputBarrier(this.attaching || !session.ready || session.travel.blocksInput || session.teleport.blocksInput); this.refreshGame(); }
        }
    }
    private async cancelTravel(id: string, source: TravelContext): Promise<void> {
        if (session.travel.id !== id || !session.travel.active) return;
        if (!session.travel.sameConnection(source) || session.connection.generation !== source.connection || session.connection.binding.toString() !== source.binding.toString()
            || session.entry?.sceneEpoch.toString() !== source.scene.toString() || roleIdentity() !== source.role) throw new StaleMapAttachmentError('旧旅行不得升级授权取消');
        session.travel.cancelling();
        const result = await session.connection.request(20009, { sceneEpoch: source.scene, travelId: id, cancel: true });
        if (result.travelId !== id || ![3, 4].includes(result.phase)) throw new Error('旅行取消尚未由服务端确定');
    }
    private async reconcileTravel(source: TravelContext): Promise<void> {
        const id = session.travel.id;
        const scope = { id, generation: session.travel.generation, ui: ++this.travelUiSequence }, prepared = this.preparedTravel;
        const current = (): boolean => !this.disposed && this.travelUiSequence === scope.ui && session.travel.id === scope.id && session.travel.generation === scope.generation;
        if (!session.travel.sameConnection(source) || session.connection.generation !== source.connection || session.connection.binding.toString() !== source.binding.toString()) throw new StaleMapAttachmentError('旧旅行只能在原连接核对');
        this.showMapLoading(true, '沿原旅行身份核对结果');
        try {
            if (!session.travel.blocksInput) {
                // Prepare响应丢失时只用原id重查；服务端可返回原Plan或phase3/4终态。
                if (!session.travel.plan) {
                    try { await session.connection.request(20007, { sceneEpoch: source.scene, travelId: id, destination: this.travelDestination }); }
                    catch (error) { if (current() && error instanceof ProtocolResponseError) { session.travel.reset(); scope.id = session.travel.id; scope.generation = session.travel.generation; } throw error; }
                }
                if (!current()) throw new StaleMapAttachmentError('旧旅行核对回调已经失效');
                if (session.travel.active) await this.cancelTravel(id, source);
            }
            else { await session.connection.request(20009, { sceneEpoch: source.scene, travelId: id, cancel: false }); if (!current()) throw new StaleMapAttachmentError('旧旅行核对回调已经失效'); await this.waitTravel(id, source); }
            if (!current()) throw new StaleMapAttachmentError('旧旅行核对结束已经失效');
        } catch (error) {
            if (!current()) throw new StaleMapAttachmentError('旧旅行核对已经失效');
            throw error;
        } finally {
            if (prepared && (!current() || !session.travel.blocksInput)) this.releasePreparedTravel(prepared);
            if (current()) { this.showMapLoading(false); this.setInputBarrier(this.attaching || !session.ready || session.travel.blocksInput || session.teleport.blocksInput); this.refreshGame(); }
        }
    }
    private waitTravel(id: string, source: TravelContext): Promise<void> {
        const generation = session.travel.generation;
        return new Promise((resolve, reject) => {
            const finish = (error?: Error): void => { session.update.delete(check); this.mapWaiters.delete(cancel); if (error) reject(error); else resolve(); };
            const cancel = (): void => finish(new StaleMapAttachmentError('旅行等待已取消'));
            const check = (): void => {
                if (this.disposed || session.connection.generation !== source.connection || session.connection.binding.toString() !== source.binding.toString() || session.travel.id !== id || session.travel.generation !== generation) finish(new StaleMapAttachmentError('旅行连接已变化'));
                else if (session.travel.phase === 'complete' && session.ready && this.isWorldCurrent() && this.world!.isMapReady) finish();
                else if (['cancelled', 'expired'].includes(session.travel.phase)) finish(new Error('服务端已取消或终止本次旅行'));
            };
            session.update.add(check); this.mapWaiters.add(cancel); check();
        });
    }
    private releasePreparedTravel(owner?: NonNullable<typeof this.preparedTravel>): void {
        const prepared = this.preparedTravel; if (!prepared || (owner && prepared !== owner)) return;
        this.preparedTravel = undefined; prepared.viewport.release(); prepared.assets.dispose();
    }

    private refreshGame(): void {
        if (this.disposed || !this.world || !this.isWorldCurrent()) return;
        const baseline = session.baseline;
        const town = session.entry?.sceneKind === 2;
        if (town) this.auto = false;
        else if (session.hosting) this.auto = session.hosting.autoBattleEnabled;
        if (baseline) this.count = baseline.entities.filter((entity: any) => entity.entityKind === 3).length;
        if (baseline && this.lastBaseline !== baseline.stateSequence.toString()) {
            this.lastBaseline = baseline.stateSequence.toString(); this.world.apply(baseline);
            this.nav.observeAuthority(baseline.navigationState);
        }
        const identity = currentEntryIdentity();
        if ((session.ready || session.travel.phase === 'quiescing') && identity) {
            const born = session.petBorn.take(identity, baseline, petAppearDuration(this.world.manifest), session.presentationNow());
            if (born) this.world.presentPetBorn(born);
        }
        const quick = this.quickSkill();
        const presentation = { playerHandle: session.entry?.playerHandle ?? 0, role: session.role, wallet: session.wallet, inventory: session.inventory, baseline,
            bindingState: session.bindingState, bindingNotice: session.bindingNotice, teleportNotice: session.teleport.state, recycleNotice: session.recycleNotice, experienceNotice: session.experienceNotice,
            hosting: session.hosting, report: session.report, ready: session.ready, lastError: session.lastError,
            auto: this.auto, count: this.count, gameplay: this.world.gameplay, map: this.world.map,
            quickSkillLabel: quick ? skillQuickStateLabel(this.skillSnapshot(), quick) : undefined };
        this.hud?.refresh(presentation);
        this.find('MapViewport/Actions/Auto').getComponent(Toggle)!.interactable = session.ready && !town && !this.attaching;
        this.find('MapViewport/Actions/Count').getComponent(Button)!.interactable = session.ready && !town && !this.attaching;
        const travel = this.find('BottomHUD/Footer/Map').getComponent(Button)!;
        travel.interactable = (session.travel.phase === 'uncertain' && session.connection.connected) || !!(session.ready && !this.attaching && !session.travel.active && this.world.gameplay.town_travel?.enabled);
        this.text('BottomHUD/Footer/Map/Label', session.travel.phase === 'uncertain' ? '核对' : town ? '返回' : '城镇');
        this.skills?.refresh(this.skillSnapshot(), quick?.definition.id ?? 0);
        this.combatStatus?.refresh(baseline, session.ready);
        const player = baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
        if (session.ready && player && this.attached && this.world.isMapReady) {
            const arrival = session.teleport.takeArrival(this.attached, player.life, player.movementRevision);
            if (arrival) this.world.presentTeleportArrival(arrival);
        }
        const materialDisabled = !!quick && (needsCastingSupply(this.skillSnapshot(), quick) && this.world.gameplay.casting_supply?.enabled === false
            || quick.definition.kind === 'summon_pet' && this.skillSnapshot().mapNoPets === true);
        this.find('BottomHUD/Skills/BasicAttack').getComponent(Button)!.interactable = !!(session.ready && (!materialDisabled || player?.hp.isZero()));
        const supply = session.inventory?.equipped?.find((entry: any) => entry.slot === this.world!.gameplay.casting_supply?.slot)?.item;
        this.find('UIMgr/Panels/Role/Equip').active = !!supply;
        this.text('UIMgr/Panels/Role/Equip/Label', this.world.gameplay.casting_supply?.enabled ? '卸下护符' : '护符未开放');
        this.find('UIMgr/Panels/Role/Equip').getComponent(Button)!.interactable = !!(session.ready && this.world.gameplay.casting_supply?.enabled && supply);
        this.refreshInventoryContext();
        this.find('MapViewport/Aiming').active = !!this.aim;
        this.skillAssets?.refresh(this.skillAssetsSnapshot());
        this.autoRecycle?.refresh(this.recycleSnapshot(), this.economicBusy, this.unresolved || !!session.pending?.get(roleIdentity()));
        this.messages?.refresh({ ...presentation, roleIdentity: roleIdentity(), entry: session.entry });
        if (this.lastInventory !== session.inventory && this.selectedItem && !session.inventory?.items?.some((item: any) => item.instanceId.toString() === this.selectedItem.instanceId.toString())) this.selectedItem = null;
        if (this.lastInventory !== session.inventory) for (const panel of ['Inventory','Role']) {
            const list = this.find(`UIMgr/Panels/${panel}/List`).getComponent(List)!;
            const items = panel === 'Inventory' ? session.inventory?.items ?? [] : (session.inventory?.equipped ?? []).map((entry: any) => entry.item);
            list.setItems(items, (node, item: any) => { const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId); node.getChildByName('Text')!.getComponent(Label)!.string = `${definition?.name ?? item.itemId} ×${item.quantity} ${item.bound ? '绑定' : '非绑定'}`; }, true);
            if (!(list as any).__boundSelection) { (list as any).__boundSelection = true; list.node.on('list-item-click', (_index: number, selected: any) => {
                if (this.disposed) return;
                const currentItems = panel === 'Inventory' ? session.inventory?.items ?? [] : (session.inventory?.equipped ?? []).map((entry: any) => entry.item);
                const item = currentItems.find((row: any) => selected?.instanceId instanceof Uint8Array && row.instanceId.toString() === selected.instanceId.toString());
                this.selectedItem = item ?? null;
                this.refreshInventoryContext();
                if (!item) { this.popup!.show('Details', { title: '物品已变化', message: '所选物品已离开此列表\n请重新选择。' }); return; }
                const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId);
                const book = definition?.kind === 'book' ? `\n${canRecycleBook(item, this.world!.gameplay.skill_book_recycle ?? []) ? '可按白名单拆书为书页' : '此来源或品质不能拆书为书页'}` : '';
                const pages = definition?.kind === 'skill_page' ? pageMergeGroup(item, session.inventory?.items ?? [], this.world!.gameplay.items) : null;
                const merge = pages ? `\n${pages.reason ?? `可查看同源${pages.items.length}叠书页合并报价`}` : '';
                const requirements = definition ? equipmentLevelLines(definition, session.inventory?.level, session.role?.job) : [];
                this.popup!.show('Details', { title: definition?.name ?? '物品详情', message: `数量 ${item.quantity}\n${item.bound ? '绑定物品' : '非绑定物品'}\n来源 ${sourceName(item.sourceKind)}\n品质 ${item.quality ?? 0}${requirements.length ? '\n' + requirements.join('\n') : ''}${book}${merge}${manualRecycleBlockReason(item) ? '\n' + manualRecycleBlockReason(item) : ''}` });
            }); }
        }
        this.lastInventory = session.inventory;
        this.text('UIMgr/Panels/Hosting/Message', `剩余托管 ${session.hosting?.remainHostingSec ?? 0} 秒\n在线自动战斗不扣托管时间`);
        const report = session.report;
        this.text('UIMgr/Panels/Report/Message', autoRecycleReportLines(report).join('\n'));
        this.status(session.lastError || (session.ready ? '' : '正在同步场景'));
        if (session.ready && !this.reconciliationStarted) {
            this.reconciliationStarted = true;
            try { if (session.pending?.get(roleIdentity())) void this.submitPending().catch(error => this.error(error)); }
            catch (error) { this.unresolved = true; this.error(error); }
        }
    }

    // 选择只刷新已保存的上下文控件，不重绑列表或推进场景和经济流程。
    private refreshInventoryContext(): void {
        if (this.disposed || !this.world) return;
        if (this.lastInventory !== session.inventory && this.selectedItem) this.selectedItem = session.inventory?.items.find((item: any) => item.instanceId.toString() === this.selectedItem.instanceId.toString()) ?? null;
        const selectedDefinition = this.world.gameplay.items.find((item: any) => item.item_id === this.selectedItem?.itemId);
        const selectedSupply = selectedDefinition?.kind === 'casting_supply', supplyEnabled = this.world.gameplay.casting_supply?.enabled;
        this.text('UIMgr/Panels/Inventory/Equip/Label', selectedSupply ? supplyEnabled ? '装入护符' : '护符未开放' : '装备');
        const equipmentReason = equipmentLevelReason(selectedDefinition, session.inventory?.level, session.role?.job);
        this.find('UIMgr/Panels/Inventory/Equip').getComponent(Button)!.interactable = !!(session.ready && (selectedSupply ? supplyEnabled : !equipmentReason));
        this.find('UIMgr/Panels/Inventory/Recycle').getComponent(Button)!.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && this.selectedItem
            && (selectedDefinition?.kind === 'book' || (selectedDefinition?.kind === 'equipment' && !manualRecycleBlockReason(this.selectedItem))));
        const lockButton = this.find('UIMgr/Panels/Inventory/ItemLock').getComponent(Button)!;
        const selectedPage = selectedDefinition?.kind === 'skill_page', pageGroup = selectedPage ? pageMergeGroup(this.selectedItem, session.inventory?.items ?? [], this.world.gameplay.items) : null;
        lockButton.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && this.selectedItem
            && (selectedPage ? !pageGroup?.reason : selectedDefinition?.kind === 'equipment' && !((this.selectedItem.recycleBlockFlags ?? 0) & 14)));
        this.text('UIMgr/Panels/Inventory/ItemLock/Label', selectedPage ? '合并书页' : this.selectedItem?.locked ? '解锁装备' : '锁定装备');
        for (const panel of ['Inventory','Role']) {
            const selected = this.world.gameplay.items.find((row: any) => row.item_id === this.selectedItem?.itemId);
            const selection = this.selectedItem ? `当前选择：${selected?.name ?? '物品'}` : '点击条目查看详情并选择物品';
            this.text(`UIMgr/Panels/${panel}/Message`, selection + (panel === 'Role' ? '\n' + roleBindingText(session.bindingState, this.world.gameplay.map_catalog)
                + ((this.world.gameplay.rule_version ?? 0) >= 20 ? '\n' + progressionSummary(session.inventory?.level, session.inventory?.progression) : '')
                : selectedDefinition?.kind === 'equipment' && equipmentReason ? '\n' + equipmentReason : ''));
        }
    }

    private recycleSnapshot(): AutoRecycleSnapshot {
        const identity = currentEntryIdentity();
        return { ready: session.ready && !this.attaching && !this.leaving, role: session.role ? roleIdentity() : '', scope: identity ? attachmentKey(identity) : '',
            rule: session.entry?.resourceVersion ?? Long.UZERO, revision: session.inventory?.revision ?? Long.UZERO, policy: session.inventory?.autoRecycle ?? null,
            items: session.inventory?.items ?? [], equipped: session.inventory?.equipped ?? [], definitions: this.world?.gameplay.items ?? [], recycling: this.world?.gameplay.equipment_recycling };
    }
    private async saveAutoRecycle(applyExisting: boolean): Promise<void> {
        this.ensureNotLeaving();
        const preview = createAutoRecyclePreview(this.recycleSnapshot(), this.autoRecycle!.draft(), applyExisting);
        if (!await this.popup!.confirm('Confirm', autoRecycleConfirmation(preview), applyExisting ? '应用并回收当前背包' : '保存自动回收设置')) return;
        if (this.disposed) return;
        validateAutoRecyclePreview(preview, this.recycleSnapshot());
        await this.economic(40016, { policy: preview.policy, applyExisting });
    }
    private async setItemLock(): Promise<void> {
        this.ensureNotLeaving(); const selected = this.selectedItem;
        if (selected && this.world?.gameplay.items.some((row: any) => row.item_id === selected.itemId && row.kind === 'skill_page')) {
            await this.previewPreparedSkillAsset(preparePageMerge(this.skillAssetsSnapshot(), selected)); return;
        }
        const snapshot = this.recycleSnapshot();
        if (!snapshot.ready || !selected || !snapshot.definitions.some(row => row.item_id === selected.itemId && row.kind === 'equipment')) throw new Error('先选择背包装备');
        const key = recycleItemKey(selected.instanceId), version = selected.version.toString(), locked = !selected.locked;
        if ((selected.recycleBlockFlags ?? 0) & 14) throw new Error('装备正被穿戴、预览或占用，暂不能更改锁定');
        if (!await this.popup!.confirm('Confirm', locked ? '锁定后自动与手动回收均不能消耗此装备。' : '解锁后此装备可按已生效策略自动回收。', locked ? '锁定装备' : '解锁装备')) return;
        if (this.disposed) return;
        const current = this.recycleSnapshot(), item = current.items.find(row => recycleItemKey(row.instanceId) === key);
        if (!current.ready || current.scope !== snapshot.scope || current.role !== snapshot.role || current.revision.toString() !== snapshot.revision.toString()
            || !item || item.version.toString() !== version || ((item.recycleBlockFlags ?? 0) & 14)) throw new Error('装备状态已变化，请重新选择');
        await this.economic(40017, { itemInstanceId: new Uint8Array(item.instanceId), itemVersion: item.version, locked });
    }

    private skillAssetsSnapshot(): SkillAssetsSnapshot {
        return { ready: session.ready, roleIdentity: session.role ? roleIdentity() : '', sceneEpoch: session.entry?.sceneEpoch ?? Long.UZERO,
            ruleVersion: session.entry?.resourceVersion ?? Long.UZERO, inventory: session.inventory, wallet: session.wallet,
            definitions: this.world?.gameplay.items ?? [], skills: this.world?.gameplay.skills ?? [], offers: this.world?.gameplay.shop_offers ?? [],
            upgrades: this.world?.gameplay.skill_upgrades ?? [], recycle: this.world?.gameplay.skill_book_recycle ?? [] };
    }

    private async previewSkillAsset(): Promise<void> {
        this.ensureNotLeaving();
        await this.previewPreparedSkillAsset(this.skillAssets!.prepare());
    }

    private async previewPreparedSkillAsset(prepared: PreparedSkillAsset): Promise<void> {
        this.ensureNotLeaving(); const scope = currentEntryIdentity(), panel = this.skillAssets!;
        if (!scope || !isCurrentEntry(scope)) throw new Error('原角色场景已变化，请重新预览');
        panel.lock(true);
        try {
            const request = { ...prepared.request, quantity: Long.fromString(prepared.request.quantity.toString()), materials: prepared.request.materials.map(item => ({
                instanceId: item.instanceId, version: item.version, quantity: Long.fromString(item.quantity.toString()),
            })) };
            const quote: SkillAssetQuote = await session.connection.request(40011, request);
            if (this.disposed) return;
            const validate = (): void => {
                if (!isCurrentEntry(scope)) throw new Error('原角色场景已变化，请重新预览');
                validateSkillAssetContext(prepared.context, quote, this.skillAssetsSnapshot(), Date.now());
                validateQuotedMaterials(quote, prepared.request.materials, this.skillAssetsSnapshot());
                if (request.kind === 4) validatePageMergeQuote(quote, prepared.request.materials, this.skillAssetsSnapshot());
                if (quote.kind !== request.kind || (quote.kind === 1 && (quote.offerId !== request.offerId || quote.quantity.toString() !== request.quantity.toString()))
                    || (quote.kind === 2 && quote.skillId !== request.skillId)) throw new Error('报价与原操作不一致，请重新预览');
            };
            validate();
            if (!await panel.showQuote(quote) || this.disposed) return;
            validate();
            await this.economic(40013, { previewId: new Uint8Array(quote.previewId) });
        } finally { if (!this.disposed && this.skillAssets === panel) panel.lock(false); }
    }

    private skillSnapshot(): SkillSnapshot {
        const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
        const modes = session.skillModes && player?.life?.toString() === session.skillModes.playerLife.toString() ? session.skillModes : null;
        const supply = session.inventory?.equipped?.find((entry: any) => entry.slot === this.world?.gameplay.casting_supply?.slot)?.item;
        const supplyName = this.world?.gameplay.items?.find((item: any) => item.item_id === supply?.itemId)?.name;
        return { ready: session.ready, roleIdentity: session.role ? roleIdentity() : '', job: session.role?.job ?? -1,
            sceneEpoch: session.entry?.sceneEpoch ?? Long.UZERO, ruleVersion: session.entry?.resourceVersion ?? Long.UZERO,
            inventory: session.inventory, definitions: this.world?.gameplay.skills ?? [], playerStatuses: player?.statuses,
            ownedPet: session.baseline?.ownedPet, mapNoPets: this.world?.gameplay.map_catalog?.find((map: any) => map.id === session.entry?.roomId.toString())?.no_pets,
            mapNoTeleport: this.world?.gameplay.map_catalog?.find((map: any) => map.id === session.entry?.roomId.toString())?.no_teleport, modes, modesReceivedAt: session.skillModesReceivedAt,
            castingSupply: supply ? { name: supplyName ?? '施法护符', quantity: supply.quantity.toString() } : null,
            castingSupplyEnabled: this.world?.gameplay.casting_supply?.enabled, statusDefinitions: this.world?.gameplay.statuses };
    }

    private async learnSkill(): Promise<void> {
        const skill = this.skills?.selected; if (!skill) throw new Error('先选择技能');
        const preview = createSkillLearnPreview(this.skillSnapshot(), skill.definition.id);
        if (!await this.popup!.confirm('Confirm', preview.message, '学习技能')) return;
        if (this.disposed) return;
        validateSkillLearnPreview(preview, this.skillSnapshot());
        await this.economic(40010, { itemInstanceId: skillBookInstance(preview), skillId: preview.skillId });
    }

    private supplySnapshot(): CastingSupplySnapshot {
        const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
        return { ready: session.ready, roleIdentity: session.role ? roleIdentity() : '', sceneEpoch: session.entry?.sceneEpoch ?? Long.UZERO,
            ruleVersion: session.entry?.resourceVersion ?? Long.UZERO, playerLife: player?.life ?? Long.UZERO,
            inventory: session.inventory, config: this.world?.gameplay.casting_supply };
    }
    private async setCastingSupply(unequip: boolean): Promise<void> {
        const snapshot = this.supplySnapshot();
        if (!snapshot.config?.enabled) throw new Error('施法护符暂未开放');
        const item = unequip
            ? snapshot.inventory?.equipped.find(entry => entry.slot === snapshot.config?.slot)?.item
            : snapshot.inventory?.items.find(value => this.selectedItem?.instanceId?.length === 16 && value.instanceId.length === 16
                && value.instanceId.every((byte, index) => byte === this.selectedItem.instanceId[index]));
        if (!item) throw new Error(unequip ? '尚未装入护符' : '先在背包选择施法护符');
        const selection = captureSupply(snapshot, item), name = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId)?.name ?? '施法护符';
        if (!await this.popup!.confirm('Confirm', `${unequip ? '卸下' : '装入'}${name}？\n剩余 ${item.quantity}份 · ${item.bound ? '绑定' : '非绑定'}\n护符仅用于材料施法，不占强化装备槽`, unequip ? '卸下护符' : '装入护符')) return;
        if (this.disposed) return;
        validateSupply(selection, this.supplySnapshot(), unequip);
        await this.economic(40015, { itemInstanceId: new Uint8Array(selection.instanceId), itemVersion: Long.fromString(selection.version, true), unequip });
    }
    private submitCast(row: SkillRow, targetHandle: number, targetGrid: { x: number; y: number }, direction: number, originalSupply?: SupplySelection): void | Promise<void> {
        this.ensureNotLeaving();
        if (row.definition.kind === 'teleport') {
            session.connection.send(30003, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, skillId: row.definition.id }); return;
        }
        const snapshot = this.skillSnapshot();
        if (row.definition.kind === 'summon_pet' && snapshot.mapNoPets) throw new Error('当前地图禁止召唤');
        // 召唤只上行技能与朝向；原宠匹配、出生坐标和网络期间的模式变化均由服务器裁定。
        const intent = row.definition.kind === 'summon_pet'
            ? { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, skillId: row.definition.id, direction }
            : { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, skillId: row.definition.id,
                targetHandle, targetGrid: { x: targetGrid.x, y: targetGrid.y }, direction };
        if (isSummonRecall(snapshot, row) || !row.definition.material_quantity) { session.connection.send(30003, intent); return; }
        const supplySnapshot = this.supplySnapshot(), supply = originalSupply ?? selectedCastingSupply(supplySnapshot, row.definition);
        // 地面瞄准不因无关掉落重选；护符实例/版本/数量仍固定，经济上下文使用提交时修订。
        validateSupply(supply, supplySnapshot, true, false);
        return this.economic(40014, { ...intent, materialInstanceId: new Uint8Array(supply.instanceId), materialVersion: Long.fromString(supply.version, true),
            casterLife: Long.fromString(supply.life, true) });
    }

    private castSkill(skill: SkillRow, modeEnabled?: boolean): void | Promise<void> {
        this.ensureNotLeaving();
        if (!session.ready) throw new Error('场景尚未就绪');
        const current = skillRows(this.skillSnapshot()).find(row => row.definition.id === skill.definition.id);
        if (!current) throw new Error('技能状态已变化，请重新选择');
        const activation = skillActivation(current.definition);
        if (activation === 'toggle' || activation === 'charge') {
            const mode = skillMode(this.skillSnapshot(), current.definition.id);
            if (!mode) throw new Error('战斗状态尚未同步，请稍后再试');
            validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle, activation === 'charge');
            if (activation === 'charge' && mode.ready) throw new Error('烈火已就绪，普通攻击命中前方目标时释放');
            session.connection.send(30009, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence,
                skillId: current.definition.id, enabled: activation === 'toggle' ? modeEnabled ?? !mode.enabled : true });
            return;
        }
        if (isGroundSkill(current.definition) || isDirectionalSkill(current.definition)) {
            const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
            if (isGroundSkill(current.definition)) validateGroundTarget(current, session.baseline?.entities ?? [], player?.position ?? { x: 0, y: 0 }, this.world!.map, session.entry.playerHandle);
            else validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle);
            const supply = current.definition.material_quantity ? selectedCastingSupply(this.supplySnapshot(), current.definition) : undefined;
            this.aim = { skillId: current.definition.id, role: roleIdentity(), scene: session.entry.sceneEpoch.toString(), rule: session.entry.resourceVersion.toString(), supply };
            this.text('MapViewport/Aiming/Text', `${current.definition.name}：轻触${isDirectionalSkill(current.definition) ? '周围选择方向' : '地面施放'}`);
            this.find('MapViewport/Aiming').active = true; this.world!.showAim(player.position);
            if (this.ui?.top?.name === 'Skills') this.ui.back();
            return;
        }
        if (current.definition.target_kind === 'enemy_self' || current.definition.target_kind === 'friendly_self' || current.definition.kind === 'teleport') {
            const player = validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle);
            return this.submitCast(current, 0, player.position, player.direction);
        }
        const target = selectSkillTarget(current, session.baseline?.entities ?? [], this.selectedTarget, session.entry.playerHandle);
        return this.submitCast(current, target.entityHandle, target.position, 0);
    }

    private quickSkill(): SkillRow | null { return primarySkill(this.skillSnapshot(), session.role ? session.quickSkills.get(roleIdentity()) ?? 0 : 0); }
    private cancelAim(): void { this.aim = null; this.find('MapViewport/Aiming').active = false; this.world?.showAim(null); }
    private consumeGround(grid: { x: number; y: number }): boolean {
        if (this.leaving) return true;
        if (!this.aim) return false;
        try {
            if (!session.ready || this.aim.role !== roleIdentity() || this.aim.scene !== session.entry?.sceneEpoch.toString()
                || this.aim.rule !== session.entry?.resourceVersion.toString()) throw new Error('瞄准状态已变化，请重新选择技能');
            const row = skillRows(this.skillSnapshot()).find(value => value.definition.id === this.aim!.skillId);
            if (!row) throw new Error('技能状态已变化，请重新选择');
            const target = isDirectionalSkill(row.definition) ? validateDirectionTarget(row, session.baseline?.entities ?? [], grid, this.world!.map, session.entry.playerHandle)
                : validateGroundTarget(row, session.baseline?.entities ?? [], grid, this.world!.map, session.entry.playerHandle);
            const player = session.baseline.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
            const result = this.submitCast(row, 0, { x: target.x, y: target.y }, gridDirection(player.position, target, player.direction), this.aim.supply);
            if (result instanceof Promise) void result.catch(error => this.error(error));
            this.cancelAim();
        } catch (error) { this.error(error); }
        return true;
    }

    private async basicAttack(): Promise<void> {
        this.ensureNotLeaving();
        if (!session.ready) throw new Error('场景尚未就绪');
        const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
        if (player?.hp.isZero()) {
            if (!session.death || !(session.death.allowedActions & 1)) throw new Error('当前场景暂未开放复活');
            const death = session.death, role = roleIdentity(), epoch = session.entry.sceneEpoch.toString();
            if (await this.popup!.confirm('Confirm', '返回出生点复活？', '角色已倒下')) {
                if (this.disposed || roleIdentity() !== role || session.entry.sceneEpoch.toString() !== epoch || session.death !== death) throw new Error('复活状态已变化，请重新确认');
                await this.economic(30002, { deathId: death.deathId, action: 1 }); session.death = null;
            }
            return;
        }
        const targets = session.baseline?.entities.filter((entity: any) => entity.entityKind === 3 && !entity.hp.isZero()) ?? [];
        const target = targets.find((entity: any) => entity.entityHandle === this.selectedTarget) ?? targets[0];
        if (player) session.connection.send(30011, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence,
            skillId: 0, direction: target ? gridDirection(player.position, target.position, player.direction) : player.direction });
    }

    private potionSnapshot(): PotionSnapshot {
        return {
            ready: session.ready,
            roleIdentity: session.role ? roleIdentity() : '',
            sceneEpoch: session.entry?.sceneEpoch ?? Long.UZERO,
            ruleVersion: session.entry?.resourceVersion ?? Long.UZERO,
            inventory: session.inventory,
            definitions: this.world?.gameplay.items ?? [],
            player: session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle) ?? null,
        };
    }

    private async usePotion(itemId: number): Promise<void> {
        const preview = createPotionPreview(this.potionSnapshot(), itemId);
        if (preview.requiresConfirmation && !await this.popup!.confirm('Confirm', preview.message, preview.title)) return;
        if (this.disposed) return;
        validatePotionPreview(preview, this.potionSnapshot());
        await this.economic(40009, {
            itemInstanceId: potionInstanceId(preview), itemId: preview.itemId,
            sceneEpoch: Long.fromString(preview.sceneEpoch, true),
        });
    }

    private async autoState(): Promise<void> {
        this.ensureNotLeaving();
        try {
            if (!session.ready) throw new Error('场景尚未就绪');
            if (session.entry?.sceneKind === 2) throw new Error('城镇不刷怪，不能修改自动战斗和怪物数量');
            await session.connection.request(30004, { sceneEpoch: session.entry.sceneEpoch, enabled: this.auto, monsterCount: this.count });
        } finally {
            // Toggle会先翻转自身；拒绝或断线时也立即恢复服务端确认的状态。
            this.refreshGame();
        }
    }

    private async economic(id: number, value: any): Promise<void> {
        this.ensureNotLeaving();
        if (this.economicBusy || this.unresolved) throw new Error('上一笔操作正在核对，请等待回执');
        if (!session.pending || session.pending.get(roleIdentity())) throw new Error('上一笔操作需要沿原身份核对');
        const payload = { ...value, operation: operation() };
        try { session.pending.set(roleIdentity(), { id, value: payload }); }
        catch (error) { session.command = payload.operation.commandSeq; throw error; }
        await this.submitPending();
    }

    private async submitPending(): Promise<void> {
        const role = roleIdentity();
        const identity = currentEntryIdentity();
        const ledger = session.pending;
        const pending = ledger?.get(role);
        if (!pending || this.economicBusy) return;
        this.economicBusy = true;
        let result: any;
        try {
            result = await session.connection.request(pending.id, pending.value);
        } catch (error) { this.unresolved = true; throw error; }
        finally { this.economicBusy = false; }
        if (this.disposed || !identity || !isCurrentEntry(identity) || roleIdentity() !== role) {
            this.unresolved = true; throw new Error('原操作附着已变化，保留原身份等待重新核对');
        }
        const expected = (pending.value.operation as { opId: Uint8Array }).opId;
        if (!(result.opId instanceof Uint8Array) || result.opId.length !== expected.length || !expected.every((value, index) => value === result.opId[index])) {
            this.unresolved = true;
            throw new Error('操作回执身份不匹配，保留原请求等待核对');
        }
        if (result.pending) { this.unresolved = true; throw new Error('操作仍在处理中，请重新认证查询结果'); }
        ledger!.clear(role); this.unresolved = false;
        if (result.errorCode) throw new Error(protocolErrorMessage(pending.id, result.errorCode));
        this.status('操作已由服务端确认');
    }

    private async leave(hosting: boolean): Promise<void> {
        if (this.leaving) throw new Error('正在等待退出确认');
        if (hosting && !session.hosting?.remainHostingSec) throw new Error('先领取免费托管时长');
        // 先释放已有摇杆，再只对本次退出关闭新输入；普通资产等待不使用此屏障。
        this.releaseInput(); this.leaving = true; this.nav.setInputBlocked(true);
        try {
            if (session.ready && !session.travel?.blocksInput && !this.attaching && this.isWorldCurrent()) {
                const entry = session.entry, binding = session.connection.binding.toString(), role = roleIdentity(), generation = session.connection.generation;
                const ack = await session.connection.requestNavigationCancel(entry.sceneEpoch, entry.baselineId, ++this.inputSequence, entry.roomId);
                if (this.disposed || session.entry !== entry || session.connection.generation !== generation || session.connection.binding.toString() !== binding || roleIdentity() !== role
                    || session.entry?.playerHandle !== entry.playerHandle) throw new Error('退出期间连接或角色已变化，请重新退出');
                if (ack?.sceneEpoch?.toString() !== entry.sceneEpoch.toString() || ack.baselineId?.toString() !== entry.baselineId.toString()
                    || ack.assembledNavigationCancel !== true || !ack.navigationState || ![0, 4].includes(ack.navigationState.status)) throw new Error('远端导航取消尚未得到完整确认，请重新退出');
                const latest = session.baseline;
                if (latest && BigInt(latest.stateSequence.toString()) > BigInt(ack.stateSequence.toString())
                    && latest.navigationState && (![0, 4].includes(latest.navigationState.status)
                        || BigInt(latest.navigationState.revision.toString()) > BigInt(ack.navigationState.revision.toString()))) throw new Error('已有更新的导航目标，请重新退出');
                session.lastError = '';
            } else session.lastError = '连接未就绪，仅完成本地退出；远端导航取消未确认';
            this.nav.exitGame(); session.connection.close(); session.ready = false;
            if (!hosting) await this.changeScene('RoleSelect'); else { session.connection.token = ''; await this.changeScene('Login'); }
        } finally {
            // 切换成功后Creator会销毁旧组件并清空字段；只有留在原场景时才恢复输入。
            if (!this.disposed) { this.leaving = false; this.nav.setInputBlocked(this.inputBarrier); }
        }
    }

    private ensureNotLeaving(): void { if (this.leaving || this.attaching || session.travel?.blocksInput) throw new Error('正在等待退出或地图附着确认，请稍后操作'); }

    private async changeScene(name: string): Promise<void> { try { await session.scenes!.switchTo(name); } catch (error) { this.error(error); } }
    private find(path: string): Node { const value = this.node.getChildByPath(`Safe/${path}`); if (!value) throw new Error(`场景缺少已保存引用：${path}`); return value; }
    private edit(path: string): EditBox { return this.find(path).getComponent(EditBox)!; }
    private text(path: string, value: string): void { const label = this.find(path).getComponent(Label); if (label) label.string = value; }
    private status(message: string): void { if (this.disposed) return; const label = this.node.getChildByPath('Safe/Status')?.getComponent(Label); if (label) label.string = message; }
    private error(error: unknown): void { if (this.disposed || (error instanceof Error && (error as StaleMapAttachmentError).staleMapAttachment === true)) return; const message = error instanceof Error ? error.message : String(error); session.lastError = message; this.status(message); }
    private button(path: string, action: () => unknown, eventType: string = Button.EventType.CLICK): void {
        const node = this.find(path); const handler = (): void => {
            traceInput({ event: 'button', path, event_type: eventType, blocked: this.uiActionBusy || this.leaving });
            if (this.uiActionBusy || this.leaving) {
                if (node.getComponent(Toggle)) this.refreshGame();
                return;
            }
            try {
                session.lastError = '';
                const result = action();
                if (result instanceof Promise) { this.uiActionBusy = true; result.catch(error => this.error(error)).finally(() => { this.uiActionBusy = false; }); }
            } catch (error) { this.error(error); }
        };
        node.on(eventType, handler, this); this.buttons.push([node, eventType, handler]);
    }
}
