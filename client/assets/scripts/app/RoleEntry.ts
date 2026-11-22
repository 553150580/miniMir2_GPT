import type Long from 'long';
import { CodeRouteChanged, CodeUnauthenticated } from '../generated/protocol_ids';
import { GameConnection, ProtocolResponseError } from './GameConnection';
import { isRoleAccountRejection } from './GameErrors';

type RoleKey = { originServerId: Long; roleId: Uint8Array };

export function isRoleAccountAuthenticationError(error: unknown): boolean {
    return error instanceof ProtocolResponseError && isRoleAccountRejection(error.messageId, error.code);
}

export function isEntryTicketError(error: unknown): boolean {
    return error instanceof ProtocolResponseError && error.messageId === 20002
        && (error.code === CodeUnauthenticated || error.code === CodeRouteChanged);
}

// 只用同角色新短票重试一次，账号认证、网络错误与普通业务拒绝由当前入口处理。
export async function connectRoleEntry(connection: GameConnection, role: RoleKey, requireScope: () => void,
    signal?: AbortSignal): Promise<number> {
    for (let attempt = 0; ; attempt++) {
        requireScope();
        const ticket = await connection.rpc(10007, { role }, signal);
        requireScope();
        const entering = connection.connect(ticket.ticket, signal);
        const generation = connection.generation;
        try {
            await entering;
        } catch (error) {
            requireScope();
            if (connection.generation === generation) connection.close();
            if (attempt === 0 && isEntryTicketError(error)) continue;
            throw error;
        }
        requireScope();
        return generation;
    }
}
