import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import type { OriginServer, ServerRegion } from './ServerDirectoryModel';

// 界面仅绑定场景内已有的列表，显示目录及入口状态。
export class ServerDirectory {
    private readonly regions: List;
    private readonly servers: List;
    private selected?: ServerRegion;

    constructor(regions: Node, servers: Node, private readonly enter: (server: OriginServer) => void) {
        const regionList = regions.getComponent(List), serverList = servers.getComponent(List);
        if (!regionList || !serverList) throw new Error('选服场景缺少已保存的分区和区服列表');
        this.regions = regionList;
        this.servers = serverList;
        regionList.node.on(List.EVENT_ITEM_CLICK, this.chooseRegion, this);
        serverList.node.on(List.EVENT_ITEM_CLICK, this.chooseServer, this);
    }

    show(regions: readonly ServerRegion[], preferredOrigin?: string): void {
        this.selected = regions.find(region => region.servers.some(server => server.originServerId.toString() === preferredOrigin))
            ?? regions.find(region => region.servers.some(server => server.available)) ?? regions[0];
        this.regions.setItems(regions, this.renderRegion, false);
        this.servers.setItems(this.selected?.servers ?? [], this.renderServer, false);
    }

    private renderRegion = (node: Node, value: unknown): void => {
        const region = value as ServerRegion;
        const selected = this.selected === region, marker = node.getChildByName('Selected');
        if (marker) marker.active = selected;
        node.getChildByName('Text')!.getComponent(Label)!.string = `${selected ? '● ' : ''}${region.name}`;
    };

    private renderServer = (node: Node, value: unknown): void => {
        const server = value as OriginServer;
        const online = node.getChildByName('Online'), unavailable = node.getChildByName('Unavailable');
        if (online && unavailable) {
            online.active = server.available;
            unavailable.active = !server.available;
        }
        node.getChildByName('Text')!.getComponent(Label)!.string = `${server.name}${!server.available && !unavailable ? '（暂不可用）' : ''}`;
        node.getComponent(Button)!.interactable = server.available;
    };

    private chooseRegion = (_index: number, region: ServerRegion): void => {
        if (region === this.selected) return;
        this.selected = region;
        this.regions.setRenderer(this.renderRegion);
        this.servers.setItems(region.servers, this.renderServer, false);
    };

    private chooseServer = (_index: number, server: OriginServer): void => {
        if (server.available && this.selected?.servers.includes(server)) this.enter(server);
    };

    dispose(): void {
        if (isValid(this.regions.node, true)) this.regions.node.off(List.EVENT_ITEM_CLICK, this.chooseRegion, this);
        if (isValid(this.servers.node, true)) this.servers.node.off(List.EVENT_ITEM_CLICK, this.chooseServer, this);
    }
}
