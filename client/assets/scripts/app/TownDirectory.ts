import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import type { TownDirectoryMapRow, TownDirectoryRegion } from './TownMapModel';

export class TownDirectory {
    private readonly regions: List;
    private readonly maps: List;
    private selected?: TownDirectoryRegion;
    constructor(readonly panel: Node, private readonly select: (row: TownDirectoryMapRow) => void) {
        this.regions = panel.getChildByName('RegionList')?.getComponent(List) as List;
        this.maps = panel.getChildByName('MapList')?.getComponent(List) as List;
        if (!this.regions || !this.maps) throw new Error('地图两列列表尚未保存在场景');
        this.regions.node.on(List.EVENT_ITEM_CLICK, this.chooseRegion, this);
        this.maps.node.on(List.EVENT_ITEM_CLICK, this.chooseMap, this);
    }
    show(regions: TownDirectoryRegion[]): void {
        this.selected = regions.find(region => region.maps.some(map => map.current)) ?? regions[0];
        this.regions.setItems(regions, this.renderRegion, false);
        this.maps.setItems(this.selected.maps, this.renderMap, false);
    }
    private renderRegion = (node: Node, value: unknown): void => {
        const region = value as TownDirectoryRegion;
        node.getChildByName('Text')!.getComponent(Label)!.string = `${region === this.selected ? '● ' : ''}${region.label}`;
    };
    private renderMap = (node: Node, value: unknown): void => {
        const row = value as TownDirectoryMapRow, type = row.destination === 1 ? '城镇' : row.destination === 3 ? `${row.minimumLevel}级可进` : '野外';
        node.getChildByName('Text')!.getComponent(Label)!.string = `${row.label} · ${type}${row.current ? '（当前）' : ''}`;
        node.getComponent(Button)!.interactable = !row.current && !row.levelLocked;
    };
    private chooseRegion = (_index: number, region: TownDirectoryRegion): void => {
        if (!this.panel.activeInHierarchy || region === this.selected) return;
        this.selected = region; this.regions.setRenderer(this.renderRegion); this.maps.setItems(region.maps, this.renderMap, false);
    };
    private chooseMap = (_index: number, row: TownDirectoryMapRow): void => {
        if (this.panel.activeInHierarchy && this.selected?.maps.includes(row) && !row.current && !row.levelLocked) this.select(row);
    };
    dispose(): void {
        if (isValid(this.regions.node, true)) this.regions.node.off(List.EVENT_ITEM_CLICK, this.chooseRegion, this);
        if (isValid(this.maps.node, true)) this.maps.node.off(List.EVENT_ITEM_CLICK, this.chooseMap, this);
    }
}
