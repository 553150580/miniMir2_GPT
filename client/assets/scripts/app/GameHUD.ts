import { _decorator, Component, isValid, Label, Node, ProgressBar, Sprite, SpriteFrame, Toggle, UITransform } from 'cc';
import { createHudState, HudInput, miniMapPoint } from './HudModel';

const { ccclass } = _decorator;

// 绑定Safe下已保存的HUD引用；此组件只更新展示数据与小地图角色点。
@ccclass('GameHUD')
export class GameHUD extends Component {
    private readonly labels = new Map<string, Label>();
    private readonly bars = new Map<string, ProgressBar>();
    private miniMapSlot: { width: number; height: number } | null = null;
    private releaseMapInvalidation?: () => void;
    private clearMapFrame?: () => void;
    private mapBinding = 0;

    setMapFrame(frame: SpriteFrame | null, lifecycle?: { onBeforeInvalidation(listener: () => void): () => void }): void {
        this.releaseMapInvalidation?.(); this.releaseMapInvalidation = undefined;
        const binding = ++this.mapBinding;
        // 保存的槽位不拉伸；不同来源小地图在同一槽内等比显示。
        this.saved('TopHUD/MiniMap/Map').getComponent(Sprite)!.spriteFrame = null;
        const sprite = this.saved('TopHUD/MiniMap/Map/Surface').getComponent(Sprite)!;
        const transform = sprite.node.getComponent(UITransform)!;
        this.miniMapSlot ??= { width: transform.width, height: transform.height };
        sprite.type = Sprite.Type.SIMPLE;
        sprite.trim = false;
        sprite.sizeMode = Sprite.SizeMode.CUSTOM;
        sprite.spriteFrame = frame;
        sprite.enabled = frame !== null;
        if (frame) {
            const size = frame.originalSize, slot = this.miniMapSlot;
            if (size.width <= 0 || size.height <= 0) throw new Error('小地图原图尺寸无效');
            const scale = Math.min(slot.width / size.width, slot.height / size.height);
            transform.setContentSize(size.width * scale, size.height * scale);
        }
        const clear = (): void => {
            if (this.mapBinding !== binding) return;
            // primary背景与source小地图均在完整WorldAssets失效前同步解绑。
            if (isValid(sprite, true)) { sprite.spriteFrame = null; sprite.enabled = false; }
        };
        this.clearMapFrame = clear;
        if (lifecycle) {
            try {
                const release = lifecycle.onBeforeInvalidation(() => {
                    clear();
                    release();
                    if (this.releaseMapInvalidation === release) this.releaseMapInvalidation = undefined;
                });
                this.releaseMapInvalidation = release;
            } catch (error) { clear(); this.clearMapFrame = undefined; throw error; }
        }
    }

    onDestroy(): void { this.clearMapFrame?.(); this.clearMapFrame = undefined; this.mapBinding++; this.releaseMapInvalidation?.(); this.releaseMapInvalidation = undefined; }

    refresh(input: HudInput): void {
        const state = createHudState(input);
        const values: Array<[string, string]> = [
            ['TopHUD/RoleName', state.roleName], ['TopHUD/Level', state.level], ['TopHUD/Job', state.job],
            ['TopHUD/Gold', state.gold], ['TopHUD/Yuanbao', state.yuanbao], ['TopHUD/Position', state.position],
            ['TopHUD/MapName', state.mapName], ['BottomHUD/ExperienceText', state.experienceText],
            ['BottomHUD/Footer/HPText', state.hpText], ['BottomHUD/Footer/MPText', state.mpText],
            ['BottomHUD/Skills/RedPotion/Count', state.redPotionCount], ['BottomHUD/Skills/BluePotion/Count', state.bluePotionCount],
            ['MapViewport/Mission/Text', state.objective], ['MapViewport/Actions/Auto/Label', state.auto],
            ['MapViewport/Actions/Count/Label', state.count], ['MapViewport/Actions/Attack/Label', state.attack],
            ['BottomHUD/Skills/BasicAttack/Label', state.attack === '复活' ? state.attack : input.quickSkillLabel ?? state.attack],
        ];
        for (const [path, value] of values) this.text(path, value);
        this.progress('BottomHUD/Footer/HP', state.hp);
        this.progress('BottomHUD/Footer/MP', state.mp);
        this.progress('BottomHUD/Experience', state.experience);
        const auto = this.saved('MapViewport/Actions/Auto').getComponent(Toggle);
        if (!auto) throw new Error('HUD自动战斗入口缺少Toggle');
        auto.interactable = input.ready;
        if (auto.isChecked !== input.auto) auto.setIsCheckedWithoutNotify(input.auto);
        for (let job = 0; job < 3; job++) this.saved(`TopHUD/Avatar/Portrait${job}`).active = job === input.role?.job;
        const map = this.saved('TopHUD/MiniMap/Map');
        const bounds = this.saved('TopHUD/MiniMap/Map/Surface').getComponent(UITransform)!;
        const point = miniMapPoint(state.playerPosition, input.map, bounds.width, bounds.height, bounds.anchorPoint.x, bounds.anchorPoint.y);
        const dot = map.getChildByName('PlayerDot');
        if (!dot) throw new Error('场景缺少已保存引用：TopHUD/MiniMap/Map/PlayerDot');
        dot.active = point !== null;
        if (point) dot.setPosition(point.x, point.y, dot.position.z);
    }

    private saved(path: string): Node {
        const node = this.node.getChildByPath(path);
        if (!node) throw new Error(`场景缺少已保存HUD引用：${path}`);
        return node;
    }

    private text(path: string, value: string): void {
        let label = this.labels.get(path);
        if (!label) {
            label = this.saved(path).getComponent(Label) ?? undefined;
            if (!label) throw new Error(`HUD节点缺少Label：${path}`);
            this.labels.set(path, label);
        }
        if (label.string !== value) label.string = value;
    }

    private progress(path: string, value: number): void {
        let bar = this.bars.get(path);
        if (!bar) {
            bar = this.saved(path).getComponent(ProgressBar) ?? undefined;
            if (!bar) throw new Error(`HUD节点缺少ProgressBar：${path}`);
            this.bars.set(path, bar);
        }
        if (bar.progress !== value) bar.progress = value;
    }
}
