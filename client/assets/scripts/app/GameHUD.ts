import { _decorator, Component, Label, Node, ProgressBar, Sprite, SpriteFrame, Toggle, UITransform } from 'cc';
import { createHudState, HudInput, miniMapPoint } from './HudModel';

const { ccclass } = _decorator;

// 绑定Safe下已保存的HUD引用；此组件只更新展示数据与小地图角色点。
@ccclass('GameHUD')
export class GameHUD extends Component {
    private readonly labels = new Map<string, Label>();
    private readonly bars = new Map<string, ProgressBar>();

    setMapFrame(frame: SpriteFrame): void {
        const sprite = this.saved('TopHUD/MiniMap/Map').getComponent(Sprite)!;
        sprite.type = Sprite.Type.SIMPLE;
        sprite.trim = false;
        sprite.sizeMode = Sprite.SizeMode.CUSTOM;
        sprite.spriteFrame = frame;
    }

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
            ['BottomHUD/Skills/BasicAttack/Label', state.attack],
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
        const bounds = map.getComponent(UITransform)!;
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
