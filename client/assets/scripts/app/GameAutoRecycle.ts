import { Button, EditBox, Label, Node, Toggle } from 'cc';
import Long from 'long';
import { AutoRecyclePolicy, AutoRecycleSnapshot, autoRecyclePolicyText, observedAutoRecycle, parseProtectedEquipmentNames, protectedEquipmentNames, RecycleDefinition } from './AutoRecycleModel';

// 只绑定已保存的Settings子树；编辑草稿不会提交，也不会冒充服务器已生效设置。
export class GameAutoRecycle {
    private role = '';
    private dirty = false;
    private policy: AutoRecyclePolicy | null = null;
    private definitions: readonly RecycleDefinition[] = [];
    constructor(private readonly panel: Node) {}
    private node(path: string): Node { const value = this.panel.getChildByPath(path); if (!value) throw new Error('自动回收保存控件缺失：' + path); return value; }
    private toggle(path: string): Toggle { return this.node(path).getComponent(Toggle)!; }
    private edit(path: string): EditBox { return this.node(path).getComponent(EditBox)!; }
    markDirty(): void { this.dirty = true; }
    reset(): void {
        if (!this.policy) return;
        this.toggle('AutoRecycleEnabled').setIsCheckedWithoutNotify(this.policy.enabled);
        for (let id = 0; id < 5; id++) this.toggle('RecycleQuality' + id).setIsCheckedWithoutNotify(this.policy.qualityIds.includes(id));
        this.edit('RecycleMaxLevel').string = this.policy.maxRequiredLevel.toString();
        this.edit('RecycleProtectedTypes').string = protectedEquipmentNames(this.policy.protectedItemIds, this.definitions);
        this.toggle('RecycleProtectBound').setIsCheckedWithoutNotify(this.policy.protectBound);
        this.toggle('RecycleProtectUnbound').setIsCheckedWithoutNotify(this.policy.protectUnbound);
        this.dirty = false;
    }
    draft(): AutoRecyclePolicy {
        if (!this.policy) throw new Error('等待服务器自动回收设置');
        const level = this.edit('RecycleMaxLevel').string.trim(), types = this.edit('RecycleProtectedTypes').string.trim();
        if (!/^(0|[1-9]\d*)$/.test(level)) throw new Error('需求等级上限需填非负整数，0表示不限');
        return observedAutoRecycle({ enabled: this.toggle('AutoRecycleEnabled').isChecked,
            qualityIds: [0, 1, 2, 3, 4].filter(id => this.toggle('RecycleQuality' + id).isChecked), maxRequiredLevel: Number(level),
            protectedItemIds: parseProtectedEquipmentNames(types, this.definitions),
            protectBound: this.toggle('RecycleProtectBound').isChecked, protectUnbound: this.toggle('RecycleProtectUnbound').isChecked,
            revision: Long.fromString(this.policy.revision.toString(), true) });
    }
    refresh(snapshot: AutoRecycleSnapshot, busy: boolean, unknown: boolean): void {
        this.definitions = snapshot.definitions;
        if (this.role !== snapshot.role) { this.role = snapshot.role; this.dirty = false; this.policy = null; }
        const previous = this.policy;
        this.policy = snapshot.policy ? observedAutoRecycle(snapshot.policy) : null;
        if (!this.dirty && this.policy) this.reset();
        const text = this.node('Message').getComponent(Label)!;
        text.string = autoRecyclePolicyText(this.policy) + (unknown ? '\n提交结果待核对，当前生效状态以服务器为准' : busy ? '\n正在提交设置' : this.dirty ? '\n草稿未保存，不影响已生效设置' : '\n默认关闭；只回收装备，保护始终生效');
        if (this.dirty && previous && this.policy && !previous.revision.equals(this.policy.revision)) { this.dirty = false; this.reset(); text.string += '\n服务器设置已更新，已恢复当前设置'; }
        const ready = snapshot.ready && !!this.policy && !busy && !unknown;
        for (const name of ['AutoRecycleEnabled', 'RecycleProtectBound', 'RecycleProtectUnbound', ...[0, 1, 2, 3, 4].map(id => 'RecycleQuality' + id)]) this.toggle(name).interactable = ready;
        this.edit('RecycleMaxLevel').enabled = ready; this.edit('RecycleProtectedTypes').enabled = ready;
        for (const name of ['RecycleSave', 'RecycleApply', 'RecycleReset']) this.node(name).getComponent(Button)!.interactable = ready;
    }
}
