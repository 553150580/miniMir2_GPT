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
import { accountIdentity, GameReconnect } from './GameReconnect';
import { connectRoleEntry, isRoleAccountAuthenticationError } from './RoleEntry';
import { ServerDirectory } from './ServerDirectory';
import { directoryRegions } from './ServerDirectoryModel';
import type { OriginServer } from './ServerDirectoryModel';
import { PendingOperations } from './PendingOperations';
import { PendingRoleCreation, RoleCreationRequest } from './PendingRoleCreation';
import { GameWorldView } from './GameWorldView';
import { WorldAssets } from './WorldAssets';
import { GameHUD } from './GameHUD';
import { GameMessages } from './GameMessages';
import { createPotionPreview, potionInstanceId, PotionSnapshot, validatePotionPreview } from './PotionModel';
import { ACCOUNT_AUTHENTICATION_MESSAGE, protocolErrorMessage } from './GameErrors';
import { GameSkills } from './GameSkills';
import { GameTasksCodex } from './GameTasksCodex';
import { GameBoss } from './GameBoss';
import type { WorldBossEntryPreparation, WorldBossEntryTarget } from './GameBoss';
import { createSkillLearnPreview, gridDirection, isDirectionalSkill, isSummonRecall, needsCastingSupply, isGroundSkill, primarySkill, selectSkillTarget, skillActivation, skillMode, skillQuickStateLabel, skillRows, SkillRow, SkillSnapshot, skillBookInstance, validateCaster, validateDirectionTarget, validateGroundTarget, validateSkillLearnPreview } from './SkillModel';
import { observeAutoSkillSettings, savedAutoSkillSettings, savedQuickSkill, saveSkillPreference } from './SkillPreferences';
import type { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import { traceInput } from './InputTrace';
import { GameSkillAssets } from './GameSkillAssets';
import { GameNpcShop } from './GameNpcShop';
import { npcShopForRole, npcShopSnapshot } from './NpcShopCatalog';
import type { VendorQuote, VendorSelection } from './NpcShopModel';
import { assetInteger, canRecycleBook, pageMergeGroup, preparePageMerge, PreparedSkillAsset, SkillAssetQuote, SkillAssetsSnapshot, sourceName, validatePageMergeQuote, validateQuotedMaterials, validateSkillAssetContext } from './SkillAssetsModel';
import { GameCombatStatus } from './GameCombatStatus';
import { captureSupply, CastingSupplySnapshot, selectedCastingSupply, SupplySelection, validateSupply } from './CastingSupplyModel';
import { MapAttachmentCoordinator, StaleMapAttachmentError } from './MapAttachmentCoordinator';
import { attachmentKey } from './MapTravelModel';
import type { AttachmentIdentity, TravelContext } from './MapTravelModel';
import { MapViewportPreparation } from './MapViewportPreparation';
import { TownDirectory } from './TownDirectory';
import { townDirectoryRegions, TownDirectoryMapRow } from './TownMapModel';
import type { TownNpc } from './TownMapModel';
import { roleBindingText } from './RoleBindingModel';
import { petAppearDuration } from './PetBornModel';
import { GameAutoRecycle } from './GameAutoRecycle';
import { AutoRecycleSnapshot, autoRecycleConfirmation, autoRecycleReportLines, createAutoRecyclePreview, manualRecycleBlockReason, recycleItemKey, validateAutoRecyclePreview } from './AutoRecycleModel';
import { inventoryCapacityText } from './InventoryDisplay';
import { equipmentLevelLines, progressionDetailLines, progressionSummary } from './LevelGrowthModel';
import { createEquipmentPreview, equipmentActionReason, equipmentConfirmation, equipmentDetailLines, equipmentDurabilityText, equipmentSlotName, EquipmentSnapshot, EquippedSelection, validateEquipmentPreview } from './EquipmentModel';
import { combatAttributeLines } from './CombatStatusModel';
import { GameWarehouse, WarehouseInput } from './GameWarehouse';
import { validateWarehousePreview, warehouseConfig, WarehouseRequest } from './WarehouseModel';
import { bindClientAudio, originalButtonSound, playClientCue } from './ClientAudio';
import { CodeInvalidArgument, CodeOperationConflict, CodeRoleSlotOrNameOccupied } from '../generated/protocol_ids';

const { ccclass, property } = _decorator;
const names = ['男战士', '女法师', '男道士'];
let signInResume: { accountId: string; name: string; origin: string | undefined; role: string | null } | null = null;

function selectedRoleIdentity(role: any): string {
    return `${role.key.originServerId.toString()}:${accountIdentity(role.key.roleId)}`;
}

interface PreparedBossEntry {
    target: WorldBossEntryTarget;
    source: AttachmentIdentity;
    rule: string;
    sent: boolean;
    assets?: WorldAssets;
    timer?: ReturnType<typeof setTimeout>;
    release(): void;
    detach(): void;
}

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
    private tasksCodex?: GameTasksCodex;
    private boss?: GameBoss;
    private skillAssets?: GameSkillAssets;
    private npcShop?: GameNpcShop;
    private npcShopContext?: { npc: TownNpc; scope: string };
    private autoRecycle?: GameAutoRecycle;
    private warehouse?: GameWarehouse;
    private townDirectory?: TownDirectory;
    private combatStatus?: GameCombatStatus;
    private releaseCombatStatusInvalidation?: () => void;
    private ui?: UIMgr;
    private popup?: PopupView;
    private selectedRole = -1;
    private selectedJob = 0;
    private selectedItem: any = null;
    private selectedEquipped: EquippedSelection | null = null;
    private selectedTarget: { handle: number; life: string } | null = null;
    private pendingSkillMode?: { skillId: number; enabled: boolean };
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
    private bossEntry?: PreparedBossEntry;
    private bossReconnect?: { target: string; connection: number; role: string; assets: WorldAssets; timer: ReturnType<typeof setTimeout> };
    // 世界首领期间保留一个原个人图资源所有者，用于返回和健康重连。
    private bossReturn?: { role: string; assets: WorldAssets };
    private travelSource?: TravelContext;
    private travelDestination = 1;
    private travelDestinationMap?: Long;
    private travelUiSequence = 0;
    private enterAttempt = 0;
    private roleDirectoryReady = false;
    private roleDirectoryTask?: Promise<boolean>;
    private roleCreationTask?: Promise<void>;
    private leaving = false;
    private tapSound?: () => void;
    private releaseInput = (): void => { if (this.joystick) this.joystick.release(); else this.nav.releaseJoystick(); };

    async start(): Promise<void> {
        profiler.hideStats();
        const releaseAudio = bindClientAudio(this.node, this.sceneKind);
        if (releaseAudio) {
            this.cleanups.push(releaseAudio);
            this.tapSound = () => playClientCue(originalButtonSound, 0.68);
        }
        session.scenes ??= new SceneManager(new SceneLoader(new CreatorSceneBackend(director)), ['Boot', 'Login', 'ServerSelect', 'RoleSelect', 'Game'], this.sceneKind);
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
            else if (this.sceneKind === 'ServerSelect') await this.serverScene();
            else if (this.sceneKind === 'RoleSelect') await this.roleScene();
            else if (this.sceneKind === 'Game') await this.gameScene();
        } catch (error) { this.error(error); }
    }

    update(dt: number): void {
        if (this.attached && session.entry) this.world?.observeEquipment(this.attached, session.entry.playerHandle, session.inventory);
        this.world?.update(dt, session.role?.job ?? 0);
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
        this.bossEntry?.release(); this.releaseBossReconnect(); this.releaseBossReturn();
        for (const [node, eventType, handler] of this.buttons) if (isValid(node, true)) node.off(eventType, handler, this);
        for (const cleanup of this.cleanups) cleanup();
        game.off(Game.EVENT_HIDE, this.releaseInput, this);
        if (typeof window !== 'undefined') window.removeEventListener('blur', this.releaseInput);
    }

    private loginScene(): void {
        if (signInResume) {
            this.edit('Account').string = signInResume.name;
            this.status(ACCOUNT_AUTHENTICATION_MESSAGE);
        }
        const login = async (register: boolean): Promise<void> => {
            if (!/^[A-Za-z0-9_]{6,20}$/.test(this.edit('Account').string.trim()) || !/^[A-Za-z0-9_]{6,20}$/.test(this.edit('Password').string)) throw new Error('账号和密码需6～20位字母、数字或下划线');
            this.status('正在认证');
            const token = session.connection.token;
            const name = this.edit('Account').string.trim();
            const result = await session.connection.rpc(register ? 10009 : 10001, { account: name, password: this.edit('Password').string });
            if (this.disposed || session.connection.token !== token) return;
            const account = accountIdentity(result.accountId);
            if (signInResume?.accountId !== account) signInResume = null;
            session.account = { id: account, name };
            this.clearSelectedServer();
            session.connection.token = result.accountToken; session.lastError = '';
            this.edit('Password').string = '';
            await this.changeScene('ServerSelect');
        };
        this.button('Login', () => login(false)); this.button('Register', () => login(true));
    }

    private async serverScene(): Promise<void> {
        const account = session.account, token = session.connection.token;
        if (!account || !token) { await this.changeScene('Login'); return; }
        const current = (): boolean => !this.disposed && session.account === account && session.connection.token === token;
        const abort = new AbortController();
        let entering = false, refreshTask: Promise<void> | undefined;
        const directory = new ServerDirectory(this.find('RegionList'), this.find('ServerList'), server => {
            if (!current() || entering) return;
            entering = true;
            this.tapSound?.();
            void this.selectServer(server).catch(error => this.error(error)).finally(() => { entering = false; });
        });
        this.cleanups.push(() => abort.abort(), () => directory.dispose());
        const refresh = (): Promise<void> | undefined => {
            if (entering) return;
            if (refreshTask) return refreshTask;
            this.status('正在取得区服目录…');
            const task = session.connection.rpc(10011, {}, abort.signal).then(value => {
                if (!current()) return;
                const regions = directoryRegions(value);
                const preferred = session.originServer?.originServerId.toString() ?? signInResume?.origin;
                directory.show(regions, preferred);
                const available = regions.some(region => region.servers.some(server => server.available));
                this.status(available ? '请选择要进入的区服' : '目前没有可进入的区服，可稍后刷新。');
            }).catch(async error => {
                if (!current()) return;
                if (isRoleAccountAuthenticationError(error)) await this.reauthenticateRole();
                else this.error(error);
            });
            refreshTask = task;
            void task.finally(() => { if (refreshTask === task) refreshTask = undefined; });
            return task;
        };
        this.button('Reload', refresh);
        this.button('Back', async () => { if (!entering) await this.logout(); });
        await refresh();
    }

    private async selectServer(server: OriginServer): Promise<void> {
        const account = session.account;
        if (!account || !server.available) return;
        const origin = server.originServerId.toString();
        this.clearSelectedServer();
        session.originServer = server;
        // 旧入口 1 仍使用原有键，其他入口各自保存未决建角及经济记录。
        session.pending = new PendingOperations(sys.localStorage, `minimir.pending.v1.${account.id}.${origin}`);
        session.roleCreation = new PendingRoleCreation(sys.localStorage, `minimir.create-role.v1.${account.id}.${origin}`);
        if (signInResume?.origin !== origin) signInResume = null;
        await this.changeScene('RoleSelect');
    }

    private clearSelectedServer(): void {
        session.originServer = null;
        session.role = null; session.roles = []; session.ready = false; session.entry = null; session.baseline = null;
        session.pending = null; session.roleCreation = null;
    }

    private async logout(): Promise<void> {
        signInResume = null; session.connection.token = ''; session.account = null;
        this.clearSelectedServer();
        await this.changeScene('Login');
    }

    private async roleScene(): Promise<void> {
        if (!session.connection.token) { await this.changeScene('Login'); return; }
        const server = session.originServer;
        if (!server) { await this.changeScene('ServerSelect'); return; }
        this.text('Title', `${server.name} · 选择角色`);
        for (let i = 0; i < 5; i++) this.button(`Role${i}`, () => {
            this.selectedRole = i;
            if (!this.roleDirectoryReady) return this.fetchRoles();
            this.refreshRoles();
        });
        for (let i = 0; i < 3; i++) this.button(`JobTabs/Job${i}`, () => { this.selectedJob = i; this.status(`创建职业：${names[i]}`); });
        this.button('Logout', () => this.logout());
        this.button('BackServers', () => this.changeScene('ServerSelect'));
        this.button('Create', () => this.createRole());
        this.button('Enter', async () => {
            if (!this.roleDirectoryReady && !await this.fetchRoles()) return;
            const role = session.roles[this.selectedRole]; if (!role) throw new Error('先选择一个角色');
            const attempt = ++this.enterAttempt, token = session.connection.token, account = session.account;
            const current = (): boolean => !this.disposed && this.enterAttempt === attempt && session.connection.token === token
                && session.account === account && session.originServer === server;
            const requireScope = (): void => {
                if (!current() || session.roles[this.selectedRole] !== role) throw new StaleMapAttachmentError('旧选角入场已经失效');
            };
            session.preparedWorld?.dispose(); session.preparedWorld = null;
            const assets = await WorldAssets.prepare(role.job, (done, total) => {
                if (!this.disposed) this.status(`准备动作资源 ${done}/${total}`);
            });
            if (!current() || session.roles[this.selectedRole] !== role) { assets.dispose(); return; }
            session.preparedWorld = assets;
            let generation: number | undefined;
            try {
                // 动作资源准备完成才取短票，避免网络加载占用入场基线的确认期限。
                this.status('正在获取入场票据');
                session.role = role; session.ready = false; session.entry = null; session.baseline = null;
                generation = await connectRoleEntry(session.connection, role.key, requireScope);
                requireScope();
                const entry = currentEntryIdentity();
                if (!entry || entry.connection !== generation || !isCurrentEntry(entry)) throw new StaleMapAttachmentError('旧选角入场已经失效');
                await session.scenes!.switchTo('Game');
            } catch (error) {
                if (session.preparedWorld === assets) { assets.dispose(); session.preparedWorld = null; }
                if (!current() || session.roles[this.selectedRole] !== role) return;
                if (isRoleAccountAuthenticationError(error)) { await this.reauthenticateRole(); return; }
                if (generation !== undefined && session.connection.generation === generation) session.connection.close();
                throw error;
            }
        });
        if (!await this.fetchRoles()) return;
        if (session.roleCreation?.get()) await this.createRole();
    }

    private fetchRoles(): Promise<boolean> {
        if (this.roleDirectoryTask) return this.roleDirectoryTask;
        const task = this.loadRoles();
        this.roleDirectoryTask = task;
        const clear = (): void => { if (this.roleDirectoryTask === task) this.roleDirectoryTask = undefined; };
        void task.then(clear, clear);
        return task;
    }

    private async loadRoles(): Promise<boolean> {
        const token = session.connection.token, ledger = session.roleCreation, account = session.account, server = session.originServer;
        if (!server) return false;
        const current = (): boolean => !this.disposed && session.connection.token === token && session.roleCreation === ledger
            && session.account === account && session.originServer === server;
        let result: any;
        try { result = await session.connection.rpc(10003, { originServerId: server.originServerId }); }
        catch (error) {
            if (!current()) return false;
            if (isRoleAccountAuthenticationError(error)) { await this.reauthenticateRole(); return false; }
            throw error;
        }
        if (!current()) return false;
        if (!Array.isArray(result.roles) || result.roles.some((role: any) => !Long.isLong(role?.key?.originServerId)
            || !server.originServerId.equals(role.key.originServerId))) throw new Error('角色目录与所选区服不匹配');
        session.roles = result.roles;
        if (!this.roleDirectoryReady) this.status('');
        this.roleDirectoryReady = true;
        if (signInResume?.accountId === account?.id && signInResume?.origin === server.originServerId.toString()) {
            const role = signInResume.role;
            signInResume = null;
            if (role) this.selectedRole = session.roles.findIndex(value => selectedRoleIdentity(value) === role);
        }
        if (this.selectedRole < 0 && session.roles.length) this.selectedRole = 0;
        this.refreshRoles();
        return true;
    }

    private createRole(): Promise<void> {
        if (this.roleCreationTask) return this.roleCreationTask;
        const task = this.submitRoleCreation(); this.roleCreationTask = task;
        const clear = (): void => { if (this.roleCreationTask === task) this.roleCreationTask = undefined; };
        void task.then(clear, clear); return task;
    }

    private async submitRoleCreation(): Promise<void> {
        const ledger = session.roleCreation, token = session.connection.token, account = session.account, server = session.originServer;
        if (!ledger || !token || !server || this.disposed) throw new Error('请先认证并选择区服后创建角色');
        const current = (): boolean => !this.disposed && session.connection.token === token && session.roleCreation === ledger
            && session.account === account && session.originServer === server;
        if (!this.roleDirectoryReady && (!await this.fetchRoles() || !current())) return;
        let request = ledger.get();
        const replaying = request !== null;
        if (!request) {
            const free = [1,2,3,4,5].find(slot => !session.roles.some(role => role.slot === slot));
            if (free === undefined) throw new Error('五个角色槽已满');
            request = { originServerId: server.originServerId, slot: free, job: this.selectedJob, name: this.edit('Name').string.trim(), opId: newOperationId() };
            ledger.set(request);
        }
        if (!request.originServerId.equals(server.originServerId)) throw new Error('原建角记录与所选区服不匹配');
        this.status(`正在核对原建角：${request.name}`);
        let result: any;
        try { result = await session.connection.rpc(10005, request); }
        catch (error) {
            if (!current()) return;
            if (isRoleAccountAuthenticationError(error)) { await this.reauthenticateRole(); return; }
            if (error instanceof ProtocolResponseError && error.messageId === 10006
                && (error.code === CodeRoleSlotOrNameOccupied || (!replaying && (error.code === CodeInvalidArgument || error.code === CodeOperationConflict)))) {
                // 槽位或名称冲突只有服务端已封存原操作的确定拒绝才可结束未知重查。
                ledger.clear(request.opId);
                try { await this.fetchRoles(); } catch { /* 保留原拒绝提示，下次选角重查目录。 */ }
            }
            throw error;
        }
        if (!current()) return;
        const matches = (role: any, original: RoleCreationRequest): boolean => role?.key?.originServerId?.toString() === original.originServerId.toString()
            && role.key.roleId instanceof Uint8Array && role.key.roleId.length === 16 && role.slot === original.slot && role.job === original.job
            && role.name === original.name.normalize('NFKC');
        if (!matches(result, request)) throw new Error('原建角回执身份不匹配，保留原请求等待核对');
        if (!await this.fetchRoles() || !current()) return;
        const selected = session.roles.findIndex(role => matches(role, request!) && role.key.roleId.every((byte: number, index: number) => byte === result.key.roleId[index]));
        if (selected < 0) throw new Error('原建角尚未出现在当前目录，请沿原操作核对');
        ledger.clear(request.opId); this.selectedRole = selected; this.refreshRoles();
        this.status('原建角已确认');
    }

    private async reauthenticateRole(): Promise<void> {
        const account = session.account;
        const selected = session.roles[this.selectedRole];
        const previous = signInResume?.accountId === account?.id ? signInResume : null;
        const origin = session.originServer?.originServerId.toString() ?? previous?.origin;
        signInResume = account ? { accountId: account.id, name: account.name, origin, role: selected ? selectedRoleIdentity(selected) : previous?.role ?? null } : null;
        session.connection.close(); session.connection.token = '';
        session.preparedWorld?.dispose(); session.preparedWorld = null;
        session.account = null;
        // 只释放内存引用，重新选择原账号的原区服后读取未决记录。
        this.clearSelectedServer();
        session.lastError = ACCOUNT_AUTHENTICATION_MESSAGE;
        this.status(ACCOUNT_AUTHENTICATION_MESSAGE);
        await this.changeScene('Login');
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
        this.cleanups.push(() => { this.releaseCombatStatusInvalidation?.(); this.combatStatus?.dispose(); });
        this.skills = new GameSkills(safe, this.popup, (title, lines) => this.combatStatus!.showDetails(title, lines));
        this.cleanups.push(() => this.skills?.dispose());
        this.tasksCodex = new GameTasksCodex(this.find('UIMgr/Panels/TasksCodex'), () => {
            const identity = currentEntryIdentity();
            return identity && session.ready && !this.disposed ? attachmentKey(identity) : '';
        }, () => session.connection.request(40027, {}), request => this.economic(40029, request),
        id => this.world?.gameplay.map_catalog?.find((row: { id: string }) => row.id === id)?.name,
        id => this.world?.gameplay.items?.find((row: { item_id: number }) => row.item_id === id)?.name,
        id => this.world?.monsterCodexFrame(id) ?? null,
        error => this.error(error));
        this.cleanups.push(() => this.tasksCodex?.dispose());
        const bossPanel = this.find('UIMgr/Panels/BossChallenge');
        this.boss = new GameBoss(bossPanel, session.connection, () => session.entry && {
            sceneEpoch: session.entry.sceneEpoch, sceneKind: session.entry.sceneKind,
            ready: session.ready && !this.attaching && !session.travel.active && !session.teleport.blocksInput,
            level: session.inventory?.level ?? session.role?.level ?? 0,
            worldBoss: this.world?.gameplay.world_boss,
        }, () => { if (this.ui?.top === bossPanel) this.ui.back(); }, (target, signal) => this.prepareBossEntry(target, signal));
        this.cleanups.push(() => this.boss?.dispose());
        this.skillAssets = new GameSkillAssets(safe, this.popup, error => this.error(error));
        this.cleanups.push(() => this.skillAssets?.dispose());
        this.npcShop = new GameNpcShop(safe, this.popup, {
            quote: selection => this.quoteNpc(selection), commit: (selection, quote) => this.commitNpc(selection, quote),
            report: error => this.error(error),
        });
        this.cleanups.push(() => this.npcShop?.dispose());
        this.autoRecycle = new GameAutoRecycle(this.find('UIMgr/Panels/Settings'));
        this.warehouse = new GameWarehouse(safe, () => this.warehouseInput(), request => session.connection.request(40020, request));
        const warehouse = this.warehouse; this.cleanups.push(() => warehouse.dispose());
        const directory = new TownDirectory(this.find('UIMgr/Panels/TownDirectory'), row => this.selectTownDestination(row));
        this.townDirectory = directory; this.cleanups.push(() => directory.dispose());
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
        const bossVisibilityChanged = (): void => this.boss?.visibilityChanged();
        this.ui.node.on(UIMgr.EVENT_CHANGED, bossVisibilityChanged, this);
        this.cleanups.push(() => {
            if (this.ui && isValid(this.ui.node, true)) {
                this.ui.node.off(UIMgr.EVENT_CHANGED, this.releaseInput, this);
                this.ui.node.off(UIMgr.EVENT_CHANGED, bossVisibilityChanged, this);
            }
            if (this.popup && isValid(this.popup.node, true)) this.popup.node.off(PopupView.EVENT_CHANGED, this.releaseInput, this);
        });
        this.bindGameButtons();
        const refresh = (): void => this.handleSessionUpdate(); session.update.add(refresh); this.cleanups.push(() => session.update.delete(refresh));
        const reconnect = new GameReconnect(this.node.getChildByName('ReconnectDialog')!,
            () => !this.attaching && this.isWorldCurrent() && this.world!.isMapReady, () => {
                this.showMapLoading(false);
                this.handleSessionUpdate();
                while (this.popup?.back()) {}
            }, () => this.attachCurrentEntry(), () => this.leave(false, true), () => this.tapSound?.());
        this.cleanups.push(() => reconnect.dispose());
        if (!session.connection.disconnectReason) await this.attachCurrentEntry();
    }

    private bindGameButtons(): void {
        this.button('UIMgr/Panels/Functions/Pages/Page0/Warehouse', async () => { this.ui!.open('Warehouse'); await this.warehouse!.open(1); });
        this.button('UIMgr/Panels/Warehouse/Personal', () => this.warehouse!.open(1));
        this.button('UIMgr/Panels/Warehouse/Shared', () => this.warehouse!.open(2));
        this.button('UIMgr/Panels/Warehouse/Refresh', () => this.warehouse!.reload());
        this.button('UIMgr/Panels/Warehouse/Previous', () => this.warehouse!.turnPage(-1));
        this.button('UIMgr/Panels/Warehouse/Next', () => this.warehouse!.turnPage(1));
        this.button('UIMgr/Panels/Warehouse/Gold', () => this.warehouse!.chooseCurrency(1));
        this.button('UIMgr/Panels/Warehouse/Yuanbao', () => this.warehouse!.chooseCurrency(2));
        for (const [name, action] of [['DepositItem', 1], ['WithdrawItem', 2], ['DepositMoney', 3], ['WithdrawMoney', 4], ['Expand', 5]] as const)
            this.button('UIMgr/Panels/Warehouse/' + name, () => this.warehouseAction(action));
        this.button('UIMgr/Panels/Warehouse/Details', () => {
            const warehouse = this.warehouse!, source = warehouse.snapshot.attachment, details = warehouse.detail(this.world!.gameplay.items);
            this.combatStatus!.showLiveDetails(details.title, () => {
                if (this.disposed || this.warehouse !== warehouse || warehouse.snapshot.attachment !== source) return null;
                try { return warehouse.detail(this.world!.gameplay.items).lines; } catch { return ['原物品已变化，请返回重新选择']; }
            });
        });
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
        this.button('BottomHUD/Footer/Map', () => this.openMapDirectory());
        if (session.entry?.resourceVersion.greaterThanOrEqual(Long.fromInt(24, true))) this.text('BottomHUD/Footer/Map/Label', '地图');
        const yuanbao = (value: Long): string => `${value.div(10)}.${value.mod(10)}`;
        this.button('TopHUD/WalletButton', () => this.popup!.show('Details', { title: '四钱包', message: session.wallet ? `金币 ${session.wallet.gold}\n绑定金币 ${session.wallet.boundGold}\n元宝 ${yuanbao(session.wallet.yuanbaoUnits)}\n绑定元宝 ${yuanbao(session.wallet.boundYuanbaoUnits)}` : '等待余额同步' }));
        this.button('MapViewport/Actions/Auto', () => { this.auto = this.find('MapViewport/Actions/Auto').getComponent(Toggle)!.isChecked; return this.autoState(); }, Toggle.EventType.TOGGLE);
        this.button('MapViewport/Actions/Count', () => { this.count = this.count === 1 ? 4 : this.count === 4 ? 8 : 1; return this.autoState(); });
        this.button('BottomHUD/Skills/RedPotion', () => this.usePotion(35));
        this.button('BottomHUD/Skills/BluePotion', () => this.usePotion(36));
        this.button('MapViewport/Actions/Attack', () => this.basicAttack());
        this.optionalButton('MapViewport/Actions/TargetDetails', () => this.openTargetDetails());
        this.button('BottomHUD/Skills/BasicAttack', () => {
            if (session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle)?.hp.isZero()) return this.basicAttack();
            const skill = this.quickSkill();
            if (!skill) return this.basicAttack();
            return this.castSkill(skill);
        });
        this.button('BottomHUD/Skills/SkillsPanel', () => { this.ui!.open('Skills'); this.refreshGame(); });
        this.button('UIMgr/Panels/Skills/Learn', () => this.learnSkill());
        this.optionalButton('UIMgr/Panels/Skills/Details', () => this.skills?.openDetails());
        this.optionalButton('UIMgr/Panels/Skills/AutoCast', () => this.changeAutoSkillSettings());
        this.optionalButton('UIMgr/Panels/Skills/SkillAuto', () => {
            const skill = this.skills?.selected;
            if (!skill?.learned || !skill.definition.auto_enabled) throw new Error('此技能当前仅可手动使用');
            return this.changeAutoSkillSettings(skill.definition.id);
        });
        this.button('UIMgr/Panels/Skills/Use', () => {
            const skill = this.skills?.selected; if (!skill) throw new Error('先选择技能');
            return this.castSkill(skill);
        });
        this.button('UIMgr/Panels/Skills/ModeToggle', () => {
            const skill = this.skills?.selected; if (!skill) throw new Error('先选择技能');
            const enabled = this.find('UIMgr/Panels/Skills/ModeToggle').getComponent(Toggle)!.isChecked;
            try { return this.castSkill(skill, enabled); }
            catch (error) { this.refreshGame(); throw error; }
        }, Toggle.EventType.TOGGLE);
        this.button('UIMgr/Panels/Skills/BindQuick', () => {
            const selected = this.skills?.selected;
            if (!selected?.learned || !selected.definition.active) throw new Error('请选择已学会的主动技能');
            const role = roleIdentity(), id = selected.definition.id;
            saveSkillPreference(sys?.localStorage, role, 'quick', id);
            session.quickSkills.set(role, id); this.refreshGame();
        });
        this.button('MapViewport/Aiming/Cancel', () => this.cancelAim());
        this.button('MapViewport/BuffBar/Overview', () => this.combatStatus!.openOverview());
        this.button('UIMgr/Panels/Role/Equip', () => this.changeEquipment(true));
        this.button('UIMgr/Panels/Role/Recycle', () => {
            const identity = currentEntryIdentity(); if (!identity) return;
            this.combatStatus?.showLiveDetails('战斗属性', () => !this.disposed && isCurrentEntry(identity)
                ? combatAttributeLines(session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle)?.combatAttributes) : null);
        });
        for (const [path, kind] of [['Shop', 1], ['Upgrade', 2], ['Salvage', 3]] as const) this.button(`UIMgr/Panels/Skills/${path}`, () => {
            const selected = this.skills?.selected;
            if (kind === 2 && !selected?.learned) throw new Error('先学会并选择要升级的技能');
            this.skillAssets!.open(kind, kind === 2 ? selected!.definition.id : 0); this.ui!.open('SkillAssets'); this.refreshGame();
        });
        this.button('UIMgr/Panels/SkillAssets/Reset', () => this.skillAssets!.reset());
        this.button('UIMgr/Panels/SkillAssets/Preview', () => this.previewSkillAsset());
        this.button('UIMgr/Panels/SkillAssets/PurchaseQuantity/Minus', () => this.skillAssets!.adjustPurchase(-1));
        this.button('UIMgr/Panels/SkillAssets/PurchaseQuantity/Plus', () => this.skillAssets!.adjustPurchase(1));
        this.button('MapViewport/Mission', () => this.popup!.show('Help', { title: '战斗状态', message: '点击空地寻路，推动摇杆接管方向。自动战斗与离线托管分别设置。' }));
        this.button('BottomHUD/QuickNav/Boss', () => {
            this.ui!.open('BossChallenge');
            return this.boss!.open();
        });
        const bossPath = 'UIMgr/Panels/BossChallenge/';
        this.button(bossPath + 'Personal', () => this.boss!.show(3));
        this.button(bossPath + 'World', () => this.boss!.show(4));
        this.button(bossPath + 'Refresh', () => this.boss!.reload());
        this.button(bossPath + 'Action', () => this.boss!.action());
        this.button(bossPath + 'Previous', () => this.boss!.turn(-1));
        this.button(bossPath + 'Next', () => this.boss!.turn(1));
        this.button('BottomHUD/QuickNav/Quest', () => {
            this.ui!.open('TasksCodex');
            return this.tasksCodex!.reload();
        });
        this.button('UIMgr/Panels/TasksCodex/Claim', () => this.tasksCodex!.claim());
        this.button('UIMgr/Panels/TasksCodex/Refresh', () => this.tasksCodex!.reload());
        this.button('BottomHUD/Skills/PetPanel', () => this.combatStatus?.showPet());
        this.button('UIMgr/Panels/Settings/Help', () => this.popup!.show('Help', { title: '操作帮助', message: '左下摇杆控制八方向，轻触空地寻路。打开面板和聊天保留地面目标，主动方向接管才终止原寻路。点击半透明背景返回。'
            + (session.role?.job === 0 ? '\n刺杀、半月需手动开启；烈火需手动充能。自动战斗仅使用已开启的模式和已就绪的烈火。' : '') }));
        this.button('UIMgr/Panels/Hosting/Leave', () => this.leave(true));
        this.button('UIMgr/Panels/Hosting/Claim', () => this.economic(40006, { benefitKind: 0 }));
        for (const panel of ['Inventory','Role']) {
            if (panel === 'Role') continue;
            this.button(`UIMgr/Panels/${panel}/Equip`, () => this.changeEquipment(false));
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
        if (this.bossReturn && identity && isCurrentEntry(identity) &&
            (identity.role !== this.bossReturn.role || (session.entry.sceneKind !== 1 && session.entry.sceneKind !== 4))) this.releaseBossReturn();
        if (this.bossEntry && !isCurrentEntry(this.bossEntry.source) && (!identity || !this.matchesBossEntry(identity))) this.bossEntry.release();
        if (this.bossReconnect && (!identity || !isCurrentEntry(identity) || identity.connection !== this.bossReconnect.connection)) this.releaseBossReconnect();
        if (!identity || !isCurrentEntry(identity)) {
            this.closeMapDirectory();
            this.closeNpcShop();
            this.warehouse?.detach();
            this.attachment?.invalidate(); this.attachmentTarget = ''; this.attachmentTask = undefined; this.attaching = false;
            this.setInputBarrier(true); this.releasePreparedTravel(); this.status(session.lastError || '连接已变化，请重新进入'); return;
        }
        if (!this.attached || attachmentKey(this.attached) !== attachmentKey(identity)) {
            this.closeMapDirectory();
            this.closeNpcShop();
            this.warehouse?.detach();
            void this.attachCurrentEntry().catch(error => { if (!(error instanceof StaleMapAttachmentError)) this.error(error); }); return;
        }
        if (session.teleport.state) this.world?.presentTeleportDeparture(session.teleport.state);
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
                    } else if (!(assets = this.takeBossAssets(identity) ?? this.takeBossReturn(identity, entry.resourceVersion.toString()))) {
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
                    if (assets.mapId !== identity.map.toString() || assets.sceneKind !== entry.sceneKind || String(assets.gameplay.rule_version) !== entry.resourceVersion.toString()) throw new Error('地图资源或规则版本与真实入场身份不一致');
                    await this.waitBaseline(identity);
                    if (isCurrentEntry(identity)) {
                        if (entry.sceneKind === 4 && session.entryBossTransition) this.retainBossReturn(identity, entry.resourceVersion.toString());
                        if (entry.sceneKind === 1) this.retainBossForReconnect(identity, entry.resourceVersion.toString());
                    }
                    return assets;
                } catch (error) { prime?.release(); prime = undefined; assets?.dispose(); throw error; }
            },
            mount: assets => {
                if (assets.sceneKind === 1 && this.attached?.map.toString() !== identity.map.toString())
                    this.count = assets.map.initial_monster_count ?? 1;
                session.progressionCurve = typeof assets.gameplay.progression?.curve_version === 'string' ? assets.gameplay.progression.curve_version : null;
                this.releaseCombatStatusInvalidation?.(); this.releaseCombatStatusInvalidation = undefined;
                this.combatStatus?.dispose(); this.combatStatus = undefined;
                this.selectedTarget = null; this.aim = null; this.pendingSkillMode = undefined; this.lastBaseline = ''; this.lastInventory = null; this.reconciliationStarted = false;
                const viewport = this.find('MapViewport'), world = viewport.getChildByName('World')!;
                const view = new GameWorldView(viewport, world, world.getChildByName('Entities')!, role.name, role.job, entry.sceneEpoch, entry.playerHandle);
                this.world = view; this.attached = identity;
                view.bindPresentation(identity, () => !this.disposed && !!session.role && this.world === view && isCurrentEntry(identity),
                    () => { if (session.role && isCurrentEntry(identity)) session.petBorn.discardPending(); });
                this.hud!.setMapFrame(assets.background, assets);
                view.prepare(assets, entity => { if (this.world === view && this.canSendInput()) this.selectWorldEntity(entity); },
                    npc => { if (this.world === view && this.canSendInput()) this.selectTownNpc(npc); });
                const combatStatus = new GameCombatStatus(this.node.getChildByName('Safe')!, this.ui!, this.popup!, assets.statusBadgePrefab, entry.playerHandle, assets.gameplay.statuses, { sceneEpoch: entry.sceneEpoch, baselineId: entry.baselineId, mapId: entry.roomId });
                this.combatStatus = combatStatus;
                const releaseStatus = assets.onBeforeInvalidation(reason => {
                    if (reason !== 'dispose') return;
                    combatStatus.dispose(); releaseStatus();
                    if (this.combatStatus === combatStatus) {
                        this.combatStatus = undefined;
                        this.releaseCombatStatusInvalidation = undefined;
                    }
                });
                this.releaseCombatStatusInvalidation = releaseStatus;
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
                if (identity.serverTransitionId && !session.entryBossTransition && !session.teleport.acknowledge(identity)) throw new StaleMapAttachmentError('瞬移附着确认代次已经变化');
                const autoSkillSettings = savedAutoSkillSettings(sys?.localStorage, identity.role);
                session.connection.send(4, { sceneEpoch: entry.sceneEpoch, baselineId: entry.baselineId, ...(autoSkillSettings && { autoSkillSettings }) });
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
    private usesMapDirectory(): boolean { return (this.world?.gameplay.rule_version ?? 0) >= 24; }
    private openMapDirectory(): void | Promise<void> {
        if (!this.usesMapDirectory() || session.travel.phase === 'uncertain') return this.travelMap();
        this.validateTravel();
        const rows = townDirectoryRegions(this.world!.gameplay, session.entry.roomId.toString());
        this.ui!.open('TownDirectory'); this.townDirectory!.show(rows);
    }
    private closeMapDirectory(): void {
        if (this.townDirectory && this.ui?.top === this.townDirectory.panel) this.ui.back();
    }
    private selectTownDestination(row: TownDirectoryMapRow): void {
        if (this.uiActionBusy || !this.townDirectory || this.ui?.top !== this.townDirectory.panel) return;
        this.tapSound?.(); this.uiActionBusy = true;
        void this.travelMap(row).catch(error => this.error(error)).finally(() => { this.uiActionBusy = false; });
    }
    private async travelMap(selection?: TownDirectoryMapRow): Promise<void> {
        if (session.travel.phase === 'uncertain' && this.travelSource) { await this.reconcileTravel(this.travelSource); return; }
        this.validateTravel();
        if (this.world!.gameplay.town_travel?.enabled !== true) throw new Error('城镇往返尚未开放');
        if (session.travel.active) throw new Error('上一旅行计划尚未确定，请等待原计划状态或重新连接');
        const source = this.travelContext(), entry = session.entry, id = Array.from(newOperationId(), byte => byte.toString(16).padStart(2, '0')).join('');
        if (selection && !townDirectoryRegions(this.world!.gameplay, entry.roomId.toString()).some(region => region.maps.some(row => row.mapId === selection.mapId
            && row.destination === selection.destination && !row.current))) throw new Error('原地图选项已变化');
        const destination = selection?.destination ?? (entry.sceneKind === 2 ? 2 : 1);
        const targetMapId = selection ? Long.fromString(selection.mapId, true) : undefined;
        const scope = { id, generation: session.travel.begin(source, id, destination, targetMapId), ui: ++this.travelUiSequence };
        const current = (): boolean => !this.disposed && this.travelUiSequence === scope.ui && session.travel.id === scope.id && session.travel.generation === scope.generation;
        let prepared: typeof this.preparedTravel;
        this.travelSource = source; this.travelDestination = destination; this.travelDestinationMap = targetMapId;
        if (selection) this.closeMapDirectory();
        this.releaseInput(); this.showMapLoading(true, '申请目标地图计划');
        try {
            const plan = await session.connection.request(20007, { sceneEpoch: entry.sceneEpoch, travelId: id, destination, ...(targetMapId ? { targetMapId } : {}) });
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
            if (assets.sceneKind !== plan.targetSceneKind) throw new Error('旅行目标地图类型与资源不匹配');
            const view = this.find('MapViewport').getComponent(UITransform)!, scale = this.world!.world.scale;
            await prime.prepare(plan.position, view.width / scale.x, view.height / scale.y);
            if (!current()) throw new StaleMapAttachmentError('旧旅行视口准备已经失效');
            this.validateTravel(); session.travel.prepared(this.travelContext(), Date.now()); this.showMapLoading(false);
            const accepted = selection || await this.popup!.confirm('Confirm', destination === 1 ? '目标城镇资源已准备。确认前往城镇？原个人战斗现场将由服务端保留。' : '目标个人地图资源已准备。确认返回原战斗现场？', '地图往返');
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
                    const targetMapId = this.travelDestinationMap;
                    try { await session.connection.request(20007, { sceneEpoch: source.scene, travelId: id, destination: this.travelDestination, ...(targetMapId ? { targetMapId } : {}) }); }
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

    // 首领场只有服务端发预留席位后才允许附着。昂贵的资源加载在请求席位之前完成。
    private async prepareBossEntry(target: WorldBossEntryTarget, signal: AbortSignal): Promise<WorldBossEntryPreparation> {
        const source = currentEntryIdentity(), entry = session.entry, role = session.role;
        const config = this.world?.gameplay.world_boss;
        if (this.disposed || this.leaving || this.attaching || signal.aborted || !source || !role || !isCurrentEntry(source) || !session.ready
            || entry.sceneKind !== 1 || session.travel.active || session.teleport.blocksInput || config?.enabled !== true
            || config.map_id !== target.mapId || config.template_id !== target.templateId) throw new StaleMapAttachmentError('世界首领准备身份已变化');
        this.bossEntry?.release();
        const candidate: PreparedBossEntry = {
            target, source, rule: entry.resourceVersion.toString(), sent: false, release: () => {
                if (this.bossEntry !== candidate) return;
                this.bossEntry = undefined;
                candidate.detach(); candidate.assets?.dispose();
                if (isCurrentEntry(source) && !this.attaching) this.showMapLoading(false);
            },
            detach: () => {
                clearTimeout(candidate.timer);
                signal.removeEventListener('abort', candidate.release);
            },
        };
        this.bossEntry = candidate;
        signal.addEventListener('abort', candidate.release, { once: true });
        const current = (): boolean => this.bossEntry === candidate && !signal.aborted && !this.disposed && !this.leaving
            && !this.attaching && isCurrentEntry(source) && session.ready && !session.travel.active && !session.teleport.blocksInput;
        this.releaseInput();
        this.showMapLoading(true, '准备世界首领资源', 0, 1);
        try {
            const assets = await WorldAssets.prepare(role.job, (done, total) => {
                if (!current()) throw new StaleMapAttachmentError('世界首领资源准备已取消');
                this.showMapLoading(true, `准备世界首领资源 ${done}/${total}`, done, total);
            }, target.mapId);
            if (this.bossEntry !== candidate) {
                assets.dispose();
                throw new StaleMapAttachmentError('世界首领资源准备已失效');
            }
            candidate.assets = assets;
            if (!current() || assets.mapId !== target.mapId || assets.sceneKind !== 4 || String(assets.gameplay.rule_version) !== candidate.rule)
                throw new StaleMapAttachmentError('世界首领资源与原场次不一致');
            this.showMapLoading(true, '重新核对世界首领场次');
            return {
                dispatch: () => {
                    if (!current()) throw new StaleMapAttachmentError('世界首领入场已失效');
                    candidate.sent = true;
                    candidate.timer = setTimeout(candidate.release, 10000);
                    this.showMapLoading(true, '等待服务端确认入场');
                },
                release: candidate.release,
            };
        } catch (error) {
            candidate.release();
            throw error;
        }
    }

    private matchesBossEntry(identity: AttachmentIdentity): boolean {
        const prepared = this.bossEntry;
        return !!prepared?.assets && prepared.sent && session.entryBossTransition && session.entry.sceneKind === 4
            && identity.serverTransitionId === prepared.target.occurrenceId && identity.map.toString() === prepared.target.mapId
            && identity.connection === prepared.source.connection && identity.role === prepared.source.role
            && identity.binding.toString() === prepared.source.binding.toString() && session.entry.resourceVersion.toString() === prepared.rule;
    }

    private takeBossAssets(identity: AttachmentIdentity): WorldAssets | undefined {
        const prepared = this.bossEntry;
        if (prepared?.assets && this.matchesBossEntry(identity)) {
            this.bossEntry = undefined;
            prepared.detach();
            return prepared.assets;
        }
        const warm = this.bossReconnect;
        if (warm && session.entryBossTransition && session.entry.sceneKind === 4 && identity.serverTransitionId === warm.target
            && identity.connection === warm.connection && identity.role === warm.role && identity.map.toString() === warm.assets.mapId
            && session.entry.resourceVersion.toString() === String(warm.assets.gameplay.rule_version)) {
            this.bossReconnect = undefined;
            clearTimeout(warm.timer);
            return warm.assets;
        }
    }

    // 健康重连先确认普通图，再由服务端恢复同一首领席位。跨这次普通图挂接保留旧视图已加载的资源。
    private retainBossForReconnect(identity: AttachmentIdentity, rule: string): void {
        const previous = this.attached;
        if (!previous?.serverTransitionId || previous.connection === identity.connection || previous.role !== identity.role
            || !this.world?.isMapReady || String(this.world.gameplay.rule_version) !== rule) return;
        const assets = this.world.takeAssets(4);
        if (!assets) return;
        this.releaseBossReconnect();
        this.bossReconnect = { target: previous.serverTransitionId, connection: identity.connection, role: identity.role, assets,
            timer: setTimeout(() => this.releaseBossReconnect(), 35000) };
    }

    private releaseBossReconnect(): void {
        const warm = this.bossReconnect;
        if (!warm) return;
        this.bossReconnect = undefined;
        clearTimeout(warm.timer);
        warm.assets.dispose();
    }

    // 进入首领前转交原挂机图资源，重连的首个个人图基线无需重新冷载。
    private retainBossReturn(identity: AttachmentIdentity, rule: string): void {
        const previous = this.attached, view = this.world;
        if (!previous || previous.role !== identity.role || previous.connection !== identity.connection ||
            !view?.isMapReady || String(view.gameplay.rule_version) !== rule) return;
        const assets = view.takeAssets(1);
        if (!assets) return;
        this.releaseBossReturn();
        this.bossReturn = { role: previous.role, assets };
    }

    private takeBossReturn(identity: AttachmentIdentity, rule: string): WorldAssets | undefined {
        const warm = this.bossReturn;
        if (!warm || session.entry.sceneKind !== 1) return;
        if (identity.role !== warm.role || identity.map.toString() !== warm.assets.mapId ||
            String(warm.assets.gameplay.rule_version) !== rule) {
            this.releaseBossReturn();
            return;
        }
        this.bossReturn = undefined;
        return warm.assets;
    }

    private releaseBossReturn(): void {
        const warm = this.bossReturn;
        this.bossReturn = undefined;
        warm?.assets.dispose();
    }

    private refreshGame(): void {
        if (this.disposed || !this.world || !this.isWorldCurrent()) return;
        const baseline = session.baseline;
        const town = session.entry?.sceneKind === 2;
        const challenge = session.entry?.sceneKind >= 3;
        if (this.ui?.top?.name === 'BossChallenge') this.boss?.render();
        if (session.inventory) this.tasksCodex?.observe(session.inventory.revision);
        if (town || challenge) this.auto = false;
        else if (session.hosting) this.auto = session.hosting.autoBattleEnabled;
        if (baseline && !town && !challenge) {
            const observed = baseline.entities.filter((entity: any) => entity.entityKind === 3).length;
            if (observed) this.count = observed;
        }
        if (baseline && this.lastBaseline !== baseline.stateSequence.toString()) {
            this.lastBaseline = baseline.stateSequence.toString(); this.world.apply(baseline);
            this.nav.observeAuthority(baseline.navigationState);
        }
        const identity = currentEntryIdentity();
        if ((session.ready || session.travel.phase === 'quiescing') && identity) {
            const world = this.world;
            const born = session.petBorn.take(identity, baseline,
                event => petAppearDuration(world.manifest, event.definitionId, world.gameplay), session.presentationNow());
            if (born) this.world.presentPetBorn(born);
        }
        const skills = this.skillSnapshot(), boundQuick = this.boundQuickSkillId(skills.roleIdentity), quick = this.quickSkill();
        const presentation = { playerHandle: session.entry?.playerHandle ?? 0, role: session.role, wallet: session.wallet, inventory: session.inventory, baseline,
            bindingState: session.bindingState, bindingNotice: session.bindingNotice, teleportNotice: session.teleport.state, recycleNotice: session.recycleNotice, experienceNotice: session.experienceNotice, warehouseNotice: session.warehouseNotice,
            hosting: session.hosting, report: session.report, ready: session.ready, lastError: session.lastError,
            auto: this.auto, count: this.count, gameplay: this.world.gameplay, map: this.world.map,
            quickSkillLabel: quick ? skillQuickStateLabel(skills, quick) : undefined };
        this.hud?.refresh(presentation);
        this.find('MapViewport/Actions/Auto').getComponent(Toggle)!.interactable = session.ready && !town && !challenge && !this.attaching;
        this.find('MapViewport/Actions/Count').getComponent(Button)!.interactable = session.ready && !town && !challenge && !this.attaching;
        const travel = this.find('BottomHUD/Footer/Map').getComponent(Button)!;
        travel.interactable = !challenge && ((session.travel.phase === 'uncertain' && session.connection.connected) || !!(session.ready && !this.attaching && !session.travel.active && this.world.gameplay.town_travel?.enabled));
        this.text('BottomHUD/Footer/Map/Label', this.usesMapDirectory() ? '地图' : session.travel.phase === 'uncertain' ? '核对' : town ? '返回' : '城镇');
        this.skills?.refresh(skills, quick?.definition.id ?? 0, boundQuick);
        this.combatStatus?.refresh(baseline, session.ready);
        const player = baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
        if (session.ready && player && this.attached && this.world.isMapReady) {
            const arrival = session.teleport.takeArrival(this.attached, player.life, player.movementRevision);
            if (arrival) this.world.presentTeleportArrival(arrival);
        }
        const materialDisabled = !!quick && (needsCastingSupply(skills, quick) && skills.castingSupplyEnabled === false
            || quick.definition.kind === 'summon_pet' && skills.mapNoPets === true);
        this.find('BottomHUD/Skills/BasicAttack').getComponent(Button)!.interactable = !!(session.ready
            && ((!materialDisabled && (!this.pendingSkillMode || this.pendingSkillMode.skillId !== quick?.definition.id)) || player?.hp.isZero()));
        const targetDetails = this.node?.getChildByPath?.('Safe/MapViewport/Actions/TargetDetails')?.getComponent(Button);
        if (targetDetails) targetDetails.node.active = targetDetails.interactable = !!(session.ready && this.selectedWorldEntity());
        this.refreshInventoryContext();
        this.find('MapViewport/Aiming').active = !!this.aim;
        this.skillAssets?.refresh(this.skillAssetsSnapshot());
        if (this.npcShopContext && this.ui?.top?.name === 'NpcShop') this.npcShop?.refresh(this.npcShopState());
        this.autoRecycle?.refresh(this.recycleSnapshot(), this.economicBusy, this.unresolved || !!session.pending?.get(roleIdentity()));
        this.warehouse?.refresh(this.warehouseInput());
        this.messages?.refresh({ ...presentation, roleIdentity: roleIdentity(), entry: session.entry });
        if (this.lastInventory !== session.inventory && this.selectedItem && !session.inventory?.items?.some((item: any) => item.instanceId.toString() === this.selectedItem.instanceId.toString())) this.selectedItem = null;
        if (this.lastInventory !== session.inventory) for (const panel of ['Inventory','Role']) {
            const list = this.find(`UIMgr/Panels/${panel}/List`).getComponent(List)!;
            const items = panel === 'Inventory' ? session.inventory?.items ?? [] : (session.inventory?.equipped ?? []).map((entry: any) => entry.item);
            list.setItems(items, (node, item: any) => {
                const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId);
                const label = node.getChildByName('Text')!.getComponent(Label)!;
                const name = definition?.name ?? item.itemId, ownership = item.bound ? '绑定' : '非绑定';
                const durability = definition?.kind === 'equipment' ? equipmentDurabilityText(item) : '';
                if (panel === 'Inventory') {
                    label.string = `${name}\n×${item.quantity} · ${ownership}${durability ? `\n${durability.includes('已损坏') ? '已损坏' : durability}` : ''}`;
                    return;
                }
                const worn = session.inventory?.equipped.find((entry: any) => recycleItemKey(entry.item.instanceId) === recycleItemKey(item.instanceId));
                const part = worn ? `${definition?.kind === 'casting_supply' ? '耗材' : equipmentSlotName(worn.slot)} · ` : '';
                label.string = `${part}${name} ×${item.quantity} ${ownership}${durability ? ` · ${durability}` : ''}`;
            }, true);
            if (!(list as any).__boundSelection) { (list as any).__boundSelection = true; list.node.on('list-item-click', (_index: number, selected: any) => {
                if (this.disposed) return;
                const currentItems = panel === 'Inventory' ? session.inventory?.items ?? [] : (session.inventory?.equipped ?? []).map((entry: any) => entry.item);
                const item = currentItems.find((row: any) => selected?.instanceId instanceof Uint8Array && row.instanceId.toString() === selected.instanceId.toString());
                if (panel === 'Inventory') this.selectedItem = item ?? null;
                else {
                    const worn = item && session.inventory?.equipped?.find((entry: any) => entry.item === item);
                    this.selectedEquipped = worn ? { slot: worn.slot, item: worn.item } : null;
                }
                this.refreshInventoryContext();
                if (!item) { this.popup!.show('Details', { title: '物品已变化', message: '所选物品已离开此列表\n请重新选择。' }); return; }
                const definition = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId);
                const book = definition?.kind === 'book' ? `\n${canRecycleBook(item, this.world!.gameplay.skill_book_recycle ?? []) ? '可按白名单拆书为书页' : '此来源或品质不能拆书为书页'}` : '';
                const pages = definition?.kind === 'skill_page' ? pageMergeGroup(item, session.inventory?.items ?? [], this.world!.gameplay.items) : null;
                const merge = pages ? `\n${pages.reason ?? `可查看同源${pages.items.length}叠书页合并报价`}` : '';
                const requirements = definition ? equipmentLevelLines(definition, session.inventory?.level, session.role?.job) : [];
                if (definition?.kind === 'equipment' && this.combatStatus) {
                    const identity = currentEntryIdentity(), id = new Uint8Array(item.instanceId);
                    this.combatStatus.showLiveDetails(definition.name, () => {
                        if (this.disposed || !identity || !isCurrentEntry(identity)) return null;
                        const current = (panel === 'Inventory' ? session.inventory?.items : session.inventory?.equipped?.map((entry: any) => entry.item))
                            ?.find((value: any) => recycleItemKey(value.instanceId) === recycleItemKey(id));
                        if (!current) return ['原装备已离开此列表，请重新选择'];
                        const currentDefinition = this.world?.gameplay.items.find((row: any) => row.item_id === current.itemId);
                        return equipmentDetailLines(currentDefinition, current, session.inventory?.level, session.role?.job, panel === 'Role');
                    }); return;
                }
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
        this.text('UIMgr/Panels/Inventory/Capacity', inventoryCapacityText(session.inventory));
        if (this.lastInventory !== session.inventory && this.selectedItem) this.selectedItem = session.inventory?.items.find((item: any) => item.instanceId.toString() === this.selectedItem.instanceId.toString()) ?? null;
        if (this.selectedEquipped) {
            const selected = this.selectedEquipped;
            const current = session.inventory?.equipped?.find((entry: any) => entry.slot === selected.slot && recycleItemKey(entry.item.instanceId) === recycleItemKey(selected.item.instanceId));
            this.selectedEquipped = current ? { slot: current.slot, item: current.item } : null;
        }
        const selectedDefinition = this.world.gameplay.items.find((item: any) => item.item_id === this.selectedItem?.itemId);
        const selectedSupply = selectedDefinition?.kind === 'casting_supply', supplyEnabled = this.world.gameplay.casting_supply?.enabled;
        this.text('UIMgr/Panels/Inventory/Equip/Label', selectedSupply ? supplyEnabled ? '装入耗材' : '耗材未开放' : '装备');
        const equipmentReason = equipmentActionReason(this.equipmentSnapshot(), this.selectedItem, false);
        this.find('UIMgr/Panels/Inventory/Equip').getComponent(Button)!.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && (selectedSupply ? supplyEnabled : !equipmentReason));
        this.find('UIMgr/Panels/Inventory/Recycle').getComponent(Button)!.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && this.selectedItem
            && (selectedDefinition?.kind === 'book' || (selectedDefinition?.kind === 'equipment' && !manualRecycleBlockReason(this.selectedItem))));
        const lockButton = this.find('UIMgr/Panels/Inventory/ItemLock').getComponent(Button)!;
        const selectedPage = selectedDefinition?.kind === 'skill_page', pageGroup = selectedPage ? pageMergeGroup(this.selectedItem, session.inventory?.items ?? [], this.world.gameplay.items) : null;
        lockButton.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && this.selectedItem
            && (selectedPage ? !pageGroup?.reason : selectedDefinition?.kind === 'equipment' && !((this.selectedItem.recycleBlockFlags ?? 0) & 14)));
        this.text('UIMgr/Panels/Inventory/ItemLock/Label', selectedPage ? '合并书页' : this.selectedItem?.locked ? '解锁装备' : '锁定装备');
        const worn = this.selectedEquipped, wornDefinition = this.world.gameplay.items.find((row: any) => row.item_id === worn?.item.itemId);
        const wornSupply = wornDefinition?.kind === 'casting_supply';
        this.find('UIMgr/Panels/Role/Equip').active = true;
        this.find('UIMgr/Panels/Role/Recycle').active = true;
        this.text('UIMgr/Panels/Role/Equip/Label', wornSupply ? supplyEnabled ? '卸下耗材' : '耗材未开放' : '卸下装备');
        this.text('UIMgr/Panels/Role/Recycle/Label', '战斗属性');
        this.find('UIMgr/Panels/Role/Equip').getComponent(Button)!.interactable = !!(session.ready && !this.economicBusy && !this.unresolved && worn
            && (wornSupply ? supplyEnabled : !equipmentActionReason(this.equipmentSnapshot(), worn.item, true, worn.slot)));
        this.find('UIMgr/Panels/Role/Recycle').getComponent(Button)!.interactable = !!session.ready;
        for (const panel of ['Inventory','Role']) {
            const item = panel === 'Inventory' ? this.selectedItem : this.selectedEquipped?.item;
            const selected = this.world.gameplay.items.find((row: any) => row.item_id === item?.itemId);
            const selection = item ? `当前选择：${selected?.name ?? '物品'}` : panel === 'Role' ? '选择已穿戴装备可查看或卸下' : '点击条目查看详情并选择物品';
            this.text(`UIMgr/Panels/${panel}/Message`, selection + (panel === 'Role' ? '\n' + roleBindingText(session.bindingState, this.world.gameplay.map_catalog)
                + ((this.world.gameplay.rule_version ?? 0) >= 20 ? '\n' + progressionSummary(session.inventory?.level, session.inventory?.progression) : '')
                : selectedDefinition?.kind === 'equipment' && equipmentReason ? '\n' + equipmentReason : ''));
        }
    }

    private warehouseInput(): WarehouseInput {
        const identity = currentEntryIdentity(), role = session.role ? roleIdentity() : '';
        return { ready: session.ready && !this.attaching && !this.leaving, attachment: identity && isCurrentEntry(identity) ? attachmentKey(identity) : '',
            role, rule: session.entry?.resourceVersion.toString() ?? '', config: warehouseConfig(this.world?.gameplay.warehouses),
            inventory: session.inventory?.items ?? [], wallet: session.wallet, definitions: this.world?.gameplay.items ?? [],
            writeBlocked: this.economicBusy || this.unresolved || !!(role && session.pending?.get(role)) };
    }
    private async warehouseAction(action: number): Promise<void> {
        this.ensureNotLeaving();
        const warehouse = this.warehouse!, preview = warehouse.prepare(action);
        if (!await this.popup!.confirm('Confirm', warehouse.confirmation(preview, this.world!.gameplay.items), '仓库操作确认')) return;
        if (this.disposed) return;
        validateWarehousePreview(preview, warehouse.snapshot);
        try {
            await this.economic(40022, preview.request);
            if (!this.disposed && this.warehouse === warehouse && this.warehouseInput().attachment === preview.attachment) await warehouse.committed(preview.request.scope);
        } catch (error) {
            if (!this.disposed && this.warehouse === warehouse && this.warehouseInput().attachment === preview.attachment) warehouse.rejected(error, preview.request.scope);
            throw error;
        }
    }

    private equipmentSnapshot(): EquipmentSnapshot {
        const identity = currentEntryIdentity();
        return { ready: session.ready, scope: identity && isCurrentEntry(identity) ? attachmentKey(identity) : '', roleIdentity: session.role ? roleIdentity() : '',
            ruleVersion: session.entry?.resourceVersion ?? Long.UZERO, level: session.inventory?.level ?? 0, job: session.role?.job ?? -1,
            items: session.inventory?.items ?? [], equipped: session.inventory?.equipped ?? [], definitions: this.world?.gameplay.items ?? [] };
    }

    private async changeEquipment(unequip: boolean): Promise<void> {
        this.ensureNotLeaving();
        const item = unequip ? this.selectedEquipped?.item : this.selectedItem;
        const definition = this.world?.gameplay.items.find((row: any) => row.item_id === item?.itemId);
        if (definition?.kind === 'casting_supply') return this.setCastingSupply(unequip);
        if (!item) throw new Error(unequip ? '先选择已穿戴装备' : '先选择背包装备');
        const preview = createEquipmentPreview(this.equipmentSnapshot(), item, unequip, unequip ? this.selectedEquipped!.slot : undefined);
        if (!await this.popup!.confirm('Confirm', equipmentConfirmation(preview, definition!.name), unequip ? '卸下装备' : '穿戴装备')) return;
        if (this.disposed) return;
        validateEquipmentPreview(preview, this.equipmentSnapshot());
        await this.economic(40001, { itemInstanceId: new Uint8Array(preview.itemInstanceId), slot: preview.slot,
            unequip: preview.unequip, itemVersion: preview.itemVersion });
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
            modePending: this.pendingSkillMode, autoSkillSettings: observeAutoSkillSettings(session.hosting?.autoSkillSettings),
            castingSupply: supply ? { name: supplyName ?? '施法耗材', quantity: supply.quantity.toString() } : null,
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
        if (!snapshot.config?.enabled) throw new Error('施法耗材暂未开放');
        const selectedWorn = this.selectedEquipped;
        const item = unequip
            ? snapshot.inventory?.equipped.find(entry => entry.slot === selectedWorn?.slot && entry.slot === snapshot.config?.slot
                && recycleItemKey(entry.item.instanceId) === recycleItemKey(selectedWorn!.item.instanceId))?.item
            : snapshot.inventory?.items.find(value => this.selectedItem?.instanceId?.length === 16 && value.instanceId.length === 16
                && value.instanceId.every((byte, index) => byte === this.selectedItem.instanceId[index]));
        if (!item) throw new Error(unequip ? '尚未装入耗材' : '先在背包选择施法耗材');
        const selection = captureSupply(snapshot, item), name = this.world!.gameplay.items.find((row: any) => row.item_id === item.itemId)?.name ?? '施法耗材';
        if (!await this.popup!.confirm('Confirm', `${unequip ? '卸下' : '装入'}${name}？\n剩余 ${item.quantity}份 · ${item.bound ? '绑定' : '非绑定'}\n耗材用于施法，不占强化装备槽`, unequip ? '卸下耗材' : '装入耗材')) return;
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
        // 地面瞄准保留原耗材实例与版本，经济上下文采用提交时修订。
        validateSupply(supply, supplySnapshot, true, false);
        return this.economic(40014, { ...intent, materialInstanceId: new Uint8Array(supply.instanceId), materialVersion: Long.fromString(supply.version, true),
            casterLife: Long.fromString(supply.life, true) });
    }

    private castSkill(skill: SkillRow, modeEnabled?: boolean): void | Promise<void> {
        this.ensureNotLeaving();
        if (!session.ready) throw new Error('场景尚未就绪');
        const snapshot = this.skillSnapshot();
        const current = skillRows(snapshot).find(row => row.definition.id === skill.definition.id);
        if (!current) throw new Error('技能状态已变化，请重新选择');
        const activation = skillActivation(current.definition);
        if (activation === 'toggle' || activation === 'charge') {
            const mode = skillMode(snapshot, current.definition.id);
            if (!mode || this.pendingSkillMode) throw new Error('战斗状态尚未同步，请稍后再试');
            validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle, activation === 'charge');
            if (activation === 'charge' && mode.ready) throw new Error('烈火已就绪，普通攻击命中前方目标时释放');
            return this.submitSkillMode(current.definition.id, activation === 'toggle' ? modeEnabled ?? !mode.enabled : true, activation === 'charge');
        }
        // 普通技能由服务端核验魔法与自动补给，材料施法仍在本地预检魔法。
        const checkCost = needsCastingSupply(snapshot, current);
        if (isGroundSkill(current.definition) || isDirectionalSkill(current.definition)) {
            const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
            if (isGroundSkill(current.definition)) validateGroundTarget(current, session.baseline?.entities ?? [], player?.position ?? { x: 0, y: 0 }, this.world!.map, session.entry.playerHandle, checkCost);
            else validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle, checkCost);
            const supply = current.definition.material_quantity ? selectedCastingSupply(this.supplySnapshot(), current.definition) : undefined;
            this.aim = { skillId: current.definition.id, role: roleIdentity(), scene: session.entry.sceneEpoch.toString(), rule: session.entry.resourceVersion.toString(), supply };
            this.text('MapViewport/Aiming/Text', `${current.definition.name}：轻触${isDirectionalSkill(current.definition) ? '周围选择方向' : '地面施放'}`);
            this.find('MapViewport/Aiming').active = true; this.world!.showAim(player.position);
            if (this.ui?.top?.name === 'Skills') this.ui.back();
            return;
        }
        if (current.definition.target_kind === 'enemy_self' || current.definition.target_kind === 'friendly_self' || current.definition.kind === 'teleport') {
            const player = validateCaster(current, session.baseline?.entities ?? [], session.entry.playerHandle, checkCost);
            return this.submitCast(current, 0, player.position, player.direction);
        }
        const target = selectSkillTarget(current, session.baseline?.entities ?? [], this.selectedWorldEntity()?.entityHandle ?? null, session.entry.playerHandle, checkCost);
        return this.submitCast(current, target.entityHandle, target.position, 0);
    }

    private async submitSkillMode(skillId: number, enabled: boolean, charge: boolean): Promise<void> {
        const identity = currentEntryIdentity(), snapshot = this.skillSnapshot(), life = snapshot.modes?.playerLife.toString();
        if (!identity || !isCurrentEntry(identity) || life === undefined) throw new Error('战斗状态尚未同步，请稍后再试');
        const pending = { skillId, enabled }; this.pendingSkillMode = pending;
        try {
            this.refreshGame();
            const modes = await session.connection.request(30009, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence, skillId, enabled });
            if (this.disposed || !isCurrentEntry(identity) || this.skillSnapshot().modes?.playerLife.toString() !== life) return;
            const confirmed = skillMode({ ...snapshot, modes }, skillId);
            if (modes?.playerLife?.toString() !== life || !confirmed || (charge ? !confirmed.ready : confirmed.enabled !== enabled))
                throw new Error('技能状态未确认，请查看当前状态后重试');
        } finally {
            if (this.pendingSkillMode === pending) { this.pendingSkillMode = undefined; this.refreshGame(); }
        }
    }

    private quickSkill(): SkillRow | null {
        const snapshot = this.skillSnapshot(), role = snapshot.roleIdentity;
        return primarySkill(snapshot, this.boundQuickSkillId(role));
    }

    private boundQuickSkillId(role: string): number {
        if (!role) return 0;
        const cached = session.quickSkills.get(role);
        if (cached !== undefined) return cached;
        const saved = savedQuickSkill(sys?.localStorage, role);
        session.quickSkills.set(role, saved);
        return saved;
    }

    private selectedWorldEntity(): any {
        const target = this.selectedTarget;
        return target ? session.baseline?.entities.find((entity: any) => entity.entityHandle === target.handle && entity.life?.toString() === target.life) : undefined;
    }

    private selectWorldEntity(entity: any): void {
        if (this.consumeGround(entity.position)) return;
        this.selectedTarget = { handle: entity.entityHandle, life: entity.life.toString() };
        this.refreshGame();
        if (session.hosting?.autoBattleEnabled && session.entry?.sceneKind === 1 && entity.entityKind === 3 && !entity.hp.isZero()) {
            try {
                session.connection.send(30016, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++this.inputSequence,
                    targetHandle: entity.entityHandle, targetLife: entity.life });
            } catch (error) { this.error(error); }
        }
    }

    private selectTownNpc(npc: TownNpc): void {
        if (this.consumeGround(npc.position)) return;
        const identity = currentEntryIdentity();
        const shop = this.npcShop && this.world && npcShopForRole(this.world.gameplay, npc.role);
        if (!shop || !identity || !isCurrentEntry(identity) || !this.npcShop) {
            this.popup!.show('Details', { title: npc.name, message: npc.role });
            return;
        }
        this.npcShopContext = { npc, scope: `${attachmentKey(identity)}/${npc.id}` };
        this.npcShop.open(npc.name, shop.key);
        this.ui!.open('NpcShop');
        this.npcShop.refresh(this.npcShopState());
    }

    private closeNpcShop(): void {
        if (!this.npcShopContext) return;
        this.npcShopContext = undefined;
        this.npcShop?.refresh(null);
        if (this.ui?.top?.name === 'NpcShop') this.ui.back();
    }

    private npcShopState() {
        const context = this.npcShopContext, identity = currentEntryIdentity(), gameplay = this.world?.gameplay;
        if (!context || !identity || !isCurrentEntry(identity) || !gameplay
            || context.scope !== `${attachmentKey(identity)}/${context.npc.id}`) return null;
        const shop = npcShopForRole(gameplay, context.npc.role);
        if (!shop) return null;
        const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry.playerHandle);
        const { x, y } = context.npc.position, range = gameplay.equipment_durability?.repair_range_cells;
        const nearby = player && !player.hp.isZero() && Number.isSafeInteger(range)
            && Math.max(Math.abs(player.position.x - x), Math.abs(player.position.y - y)) <= range;
        const matchedRevision = !!session.inventory && !!session.wallet && session.inventory.revision.equals(session.wallet.revision);
        const ready = !!nearby && matchedRevision && this.canSendInput() && !this.economicBusy && !this.unresolved
            && !session.pending?.get(roleIdentity());
        return { ...npcShopSnapshot(shop, context.scope, session.inventory, ready, gameplay.items, !!gameplay.equipment_durability?.special),
            reason: nearby ? '正在同步角色资产，请稍候' : '请走到这位商人身旁，再进行交易或修理' };
    }

    private npcScope(selection: VendorSelection) {
        const current = this.npcShopState();
        if (this.ui?.top?.name !== 'NpcShop' || !current?.ready || current.scope !== selection.scope || current.shopKey !== selection.shopKey) {
            throw new Error('请靠近原商人，并等待角色资产同步');
        }
        const sceneEpoch = session.entry.sceneEpoch as Long, npcId = Long.fromString(this.npcShopContext!.npc.id, true);
        const row = (selection.action === 1 ? current.buy : selection.action === 2 ? current.sell : current.repair)
            .find(item => item.key === selection.key && !item.disabledReason);
        if (!row || row.itemId !== selection.itemId || row.offerId !== selection.offerId
            || row.itemVersion?.toString() !== selection.itemVersion?.toString()) throw new Error('原商品或装备已变化，请重新选择');
        return { sceneEpoch, npcId, row };
    }

    private async quoteNpc(selection: VendorSelection): Promise<VendorQuote> {
        const { sceneEpoch, npcId, row } = this.npcScope(selection);
        const packs = selection.quantity / assetInteger(row.quantityStep ?? 1);
        const quote = await session.connection.request(40024, { sceneEpoch, npcId, action: selection.action, offerId: row.offerId,
            instanceId: selection.instanceId, itemVersion: selection.itemVersion === undefined ? undefined : Long.fromString(selection.itemVersion.toString(), true),
            quantity: Long.fromString(packs.toString()) });
        this.npcScope(selection);
        if (!npcId.equals(quote.npcId) || !sceneEpoch.equals(quote.sceneEpoch)
            || (selection.itemVersion !== undefined && quote.itemVersion.toString() !== selection.itemVersion.toString())) {
            throw new Error('商店报价与原场景或物品不一致，请重新报价');
        }
        const price = [quote.gold, quote.boundGold, quote.ingot, quote.boundIngot].reduce((sum, amount) => sum + assetInteger(amount), 0n);
        return { ...quote, price, quantity: quote.itemQuantity, instanceId: quote.instanceId?.length ? quote.instanceId : undefined };
    }

    private async commitNpc(selection: VendorSelection, quote: VendorQuote): Promise<void> {
        const { sceneEpoch, npcId } = this.npcScope(selection);
        if (quote.npcId.toString() !== npcId.toString()) throw new Error('原商人已经变化，请重新报价');
        await this.economic(40026, { quoteId: new Uint8Array(quote.quoteId), sceneEpoch, npcId });
    }

    private openTargetDetails(): void {
        const selected = this.selectedWorldEntity();
        if (selected) this.combatStatus?.showEntity(selected.entityHandle, selected.life.toString());
    }

    private cancelAim(): void { this.aim = null; this.find('MapViewport/Aiming').active = false; this.world?.showAim(null); }
    private consumeGround(grid: { x: number; y: number }): boolean {
        if (this.leaving) return true;
        if (!this.aim) return false;
        try {
            if (!session.ready || this.aim.role !== roleIdentity() || this.aim.scene !== session.entry?.sceneEpoch.toString()
                || this.aim.rule !== session.entry?.resourceVersion.toString()) throw new Error('瞄准状态已变化，请重新选择技能');
            const snapshot = this.skillSnapshot();
            const row = skillRows(snapshot).find(value => value.definition.id === this.aim!.skillId);
            if (!row) throw new Error('技能状态已变化，请重新选择');
            const checkCost = needsCastingSupply(snapshot, row);
            const target = isDirectionalSkill(row.definition) ? validateDirectionTarget(row, session.baseline?.entities ?? [], grid, this.world!.map, session.entry.playerHandle, checkCost)
                : validateGroundTarget(row, session.baseline?.entities ?? [], grid, this.world!.map, session.entry.playerHandle, checkCost);
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
        const selected = this.selectedWorldEntity();
        const target = targets.includes(selected) ? selected : targets[0];
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

    private async changeAutoSkillSettings(skillId?: number): Promise<void> {
        this.ensureNotLeaving();
        const identity = currentEntryIdentity(), current = observeAutoSkillSettings(session.hosting?.autoSkillSettings);
        if (!session.ready || !identity || !isCurrentEntry(identity) || !current) throw new Error('自动施法设置尚未同步');
        const ids = new Set(current.disabledSkillIds);
        if (skillId !== undefined && !ids.delete(skillId)) ids.add(skillId);
        const settings = { enabled: skillId === undefined ? !current.enabled : current.enabled, disabledSkillIds: Array.from(ids).sort((a, b) => a - b) };
        const response = await session.connection.request(30004, { sceneEpoch: session.entry.sceneEpoch, autoSkillSettings: settings });
        if (this.disposed || !isCurrentEntry(identity)) return;
        const confirmed = observeAutoSkillSettings(response?.autoSkillSettings);
        if (!confirmed || confirmed.enabled !== settings.enabled || confirmed.disabledSkillIds.join() !== settings.disabledSkillIds.join()) throw new Error('自动施法设置未确认，请重新查看');
        try { saveSkillPreference(sys?.localStorage, identity.role, 'auto', confirmed); }
        catch { throw new Error('当前自动施法设置已生效，浏览器未能记住该偏好'); }
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
        if (pending.id === 40022 && identity && isCurrentEntry(identity)) session.warehouseReceipts.expect(attachmentKey(identity),
            (pending.value.operation as { opId: Uint8Array }).opId, pending.value as unknown as WarehouseRequest);
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
        if (result.errorCode) {
            if (pending.id === 40022) session.warehouseReceipts.cancel(expected);
            throw new ProtocolResponseError(pending.id, result.errorCode);
        }
        this.status('操作已由服务端确认');
    }

    private requireHostingExit(baseline = session.baseline): void {
        if (!session.ready || this.attaching || session.travel.blocksInput || session.teleport.blocksInput
            || !this.isWorldCurrent() || !session.hosting) throw new Error('托管状态尚未同步，请恢复连接后再试');
        if (session.entry.sceneKind !== 1) throw new Error('请返回普通挂机地图再开始离线托管');
        const player = baseline?.entities.find((entity: any) => entity.entityHandle === session.entry.playerHandle);
        if (!player || player.hp.isZero()) throw new Error('请在角色存活且状态已同步时开始离线托管');
        if (!session.hosting.remainHostingSec) throw new Error('先领取免费托管时长');
        if (!session.hosting.autoBattleEnabled) throw new Error('请先开启自动战斗，再开始离线托管');
    }

    private async leave(hosting: boolean, localOnly = false): Promise<void> {
        if (this.leaving) throw new Error('正在等待退出确认');
        if (hosting) this.requireHostingExit();
        // 先释放已有摇杆，再只对本次退出关闭新输入；普通资产等待不使用此屏障。
        this.releaseInput(); this.leaving = true; this.nav.setInputBlocked(true);
        try {
            if (!localOnly && session.ready && !session.travel.blocksInput && !this.attaching && this.isWorldCurrent()) {
                const entry = session.entry, binding = session.connection.binding.toString(), role = roleIdentity(), generation = session.connection.generation;
                const ack = await session.connection.requestNavigationCancel(entry.sceneEpoch, entry.baselineId, ++this.inputSequence, entry.roomId);
                if (this.disposed || session.entry !== entry || session.connection.generation !== generation || session.connection.binding.toString() !== binding || roleIdentity() !== role
                    || session.entry?.playerHandle !== entry.playerHandle) throw new Error('退出期间连接或角色已变化，请重新退出');
                if (ack?.sceneEpoch?.toString() !== entry.sceneEpoch.toString() || ack.baselineId?.toString() !== entry.baselineId.toString()
                    || ack.assembledNavigationCancel !== true || !ack.navigationState || ![0, 4].includes(ack.navigationState.status)) throw new Error('远端导航取消尚未得到完整确认，请重新退出');
                const latest = session.baseline;
                const hasNewerBaseline = latest && BigInt(latest.stateSequence.toString()) > BigInt(ack.stateSequence.toString());
                if (hasNewerBaseline && latest.navigationState && (![0, 4].includes(latest.navigationState.status)
                        || BigInt(latest.navigationState.revision.toString()) > BigInt(ack.navigationState.revision.toString()))) throw new Error('已有更新的导航目标，请重新退出');
                if (hosting) this.requireHostingExit(hasNewerBaseline ? latest : ack);
                session.lastError = '';
            } else session.lastError = '连接未就绪，仅完成本地退出；远端导航取消未确认';
            this.nav.exitGame(); session.connection.close(); session.ready = false;
            if (hosting) { session.connection.token = ''; session.account = null; this.clearSelectedServer(); }
            await session.scenes!.switchTo(session.connection.token ? 'RoleSelect' : 'Login');
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
    private optionalButton(path: string, action: () => unknown): void {
        if (this.node?.getChildByPath?.(`Safe/${path}`)) this.button(path, action);
    }
    private button(path: string, action: () => unknown, eventType: string = Button.EventType.CLICK): void {
        const node = this.find(path); const handler = (): void => {
            traceInput({ event: 'button', path, event_type: eventType, blocked: this.uiActionBusy || this.leaving });
            if (this.uiActionBusy || this.leaving) {
                if (node.getComponent(Toggle)) this.refreshGame();
                return;
            }
            try {
                session.lastError = '';
                this.tapSound?.();
                const result = action();
                if (result instanceof Promise) { this.uiActionBusy = true; result.catch(error => this.error(error)).finally(() => { this.uiActionBusy = false; }); }
            } catch (error) { this.error(error); }
        };
        node.on(eventType, handler, this); this.buttons.push([node, eventType, handler]);
    }
}
