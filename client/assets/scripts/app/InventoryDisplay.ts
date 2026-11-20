// 每个背包实例占一格。堆叠数量与已穿戴装备独立于背包容量。
export function inventoryCapacityText(inventory?: { readonly items: readonly unknown[]; readonly capacity: number } | null): string {
    if (!inventory || !Array.isArray(inventory.items) || !Number.isSafeInteger(inventory.capacity) || inventory.capacity < 0) return '—/—';
    return `${inventory.items.length}/${inventory.capacity}`;
}
