export interface HeadFrame { offset_y: number; declared_empty?: boolean; visible_alpha_pixels?: number; }
export interface HeadProfile { layers: readonly { package: string; actions: Record<string, { directions: readonly { frames: readonly string[] }[] }> }[]; }
export interface HeadLabelTemplate { bodyY: number; bodyHeight: number; bodyAnchorY: number; bodyScaleY: number; nameY: number; hpY: number; }

// 与原WIL绘制的16-offset_y一致；站立/行走身体和头发的顶边固定，不采样武器、施法或倒地动作。
export function profileHeadTop(profile: HeadProfile, frames: Readonly<Record<string, HeadFrame>>): number {
    let top = -Infinity;
    for (const layer of profile.layers) {
        if (layer.package === 'Weapon') continue;
        for (const action of ['standing', 'walking']) for (const direction of layer.actions[action]?.directions ?? []) for (const key of direction.frames) {
            const frame = frames[key];
            if (!frame || frame.declared_empty || frame.visible_alpha_pixels === 0) continue;
            if (!Number.isFinite(frame.offset_y)) throw new Error('形象头顶原帧偏移无效');
            top = Math.max(top, 16 - frame.offset_y);
        }
    }
    if (!Number.isFinite(top)) throw new Error('形象缺少可核验的站立或行走身体帧');
    return top;
}
export function entityHeadLabels(profile: HeadProfile, frames: Readonly<Record<string, HeadFrame>>, template: HeadLabelTemplate) {
    const headTop = profileHeadTop(profile, frames);
    const savedTop = template.bodyY + template.bodyHeight * (1 - template.bodyAnchorY) * Math.abs(template.bodyScaleY);
    if (!Number.isFinite(savedTop) || template.nameY <= template.hpY || template.hpY <= savedTop) throw new Error('实体模板的名字、血条必须依次保存在头顶上方');
    const delta = headTop - savedTop;
    return { headTop, nameY: template.nameY + delta, hpY: template.hpY + delta };
}
export function entityDisplayName(kind: number, definitionId: number, playerName: string, job: number, gameplay: {
    monster_profiles?: readonly { definition_id: number; name: string }[];
    pet_profiles?: readonly { definition_id: number; name: string; job_id: number }[];
}): string {
    if (kind === 1) return playerName;
    if (kind === 2) return gameplay.pet_profiles?.find(row => row.definition_id === definitionId && row.job_id === job)?.name ?? '';
    if (kind === 3) return gameplay.monster_profiles?.find(row => row.definition_id === definitionId)?.name ?? '';
    return kind === 4 ? '掉落' : '';
}
