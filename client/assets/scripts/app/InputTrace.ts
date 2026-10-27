type TraceValue = string | number | boolean;
export type InputTraceRecord = Record<string, TraceValue>;

function uint(value: unknown): number | undefined {
    return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 && value <= 0xffffffff ? value : undefined;
}

// 调试输出只投影允许字段，绝不展开原始报文、账号、操作凭据或票据。
export function inputTraceMessage(direction: 'send' | 'receive', id: number, sequence: number, value: any): InputTraceRecord | null {
    const row: InputTraceRecord = { event: direction, message_id: id, packet_seq: sequence };
    const field = (name: string, source: unknown): void => { const number = uint(source); if (number !== undefined) row[name] = number; };
    if (direction === 'send') {
        if (id === 30003) { field('skill_id', value?.skillId); field('input_sequence', value?.inputSequence); }
        else if (id === 30004) { if (typeof value?.enabled === 'boolean') row.enabled = value.enabled; }
        else if (id === 40009) field('item_id', value?.itemId);
        else if (id === 40010) field('skill_id', value?.skillId);
        else return null;
    } else {
        if (id === 30005) { if (typeof value?.autoBattleEnabled === 'boolean') row.enabled = value.autoBattleEnabled; }
        else if (id === 30006) {
            field('skill_id', value?.skillId); field('input_sequence', value?.inputSequence);
            const cast = value?.castId?.toString();
            if (typeof cast === 'string' && /^(?:0|[1-9]\d{0,19})$/.test(cast)) row.cast_id = cast;
        } else return null;
    }
    return row;
}

export function traceInput(record: InputTraceRecord | null): void {
    if (!record || typeof location === 'undefined' || new URLSearchParams(location.search).get('renderQA') !== '1') return;
    console.info('输入验收', JSON.stringify({ at_ms: Date.now(), ...record }));
}
