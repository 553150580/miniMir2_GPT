import Long from 'long';

export interface OriginServer {
    readonly originServerId: Long;
    readonly name: string;
    readonly available: boolean;
}

export interface ServerRegion {
    readonly regionId: string;
    readonly name: string;
    readonly servers: readonly OriginServer[];
}

type DirectoryValue = { regions?: readonly { regionId?: unknown; name?: unknown; servers?: readonly {
    originServerId?: unknown; name?: unknown; available?: unknown;
}[] }[] };

// 从账号服读取目录。永久入口在本次目录中须唯一，并在跨场景时保持 64 位精度。
export function directoryRegions(value: DirectoryValue): ServerRegion[] {
    if (!Array.isArray(value?.regions)) throw new Error('区服目录响应无效');
    const regions = new Set<string>(), servers = new Set<string>();
    return value.regions.map((region: NonNullable<DirectoryValue['regions']>[number]) => {
        if (typeof region?.regionId !== 'string' || !region.regionId.trim() || regions.has(region.regionId)
            || typeof region.name !== 'string' || !region.name.trim() || !Array.isArray(region.servers)) throw new Error('分区目录响应无效');
        regions.add(region.regionId);
        return { regionId: region.regionId, name: region.name, servers: region.servers.map(
            (server: NonNullable<typeof region.servers>[number]) => {
                if (!Long.isLong(server?.originServerId) || !server.originServerId.unsigned || server.originServerId.isZero()
                    || servers.has(server.originServerId.toString()) || typeof server.name !== 'string' || !server.name.trim()
                    || typeof server.available !== 'boolean') throw new Error('区服条目无效');
                servers.add(server.originServerId.toString());
                return { originServerId: server.originServerId, name: server.name, available: server.available };
            },
        ) };
    });
}
