import { Color, Graphics } from 'cc';

interface Cell { x: number; y: number }

// 仅勾出完整基线中的火墙有效地格，不推测命中范围或持续时间。
export class FireWallView {
    private last = '';

    constructor(private readonly graphics: Graphics, private readonly grid: (x: number, y: number) => Cell) {}

    apply(cells: readonly Cell[], width: number, height: number): void {
        const valid = cells.filter(({ x, y }) => x >= 0 && x < width && y >= 0 && y < height);
        const identity = valid.map(({ x, y }) => `${x},${y}`).join(';');
        if (identity === this.last) return;
        this.last = identity;
        this.graphics.clear();
        this.graphics.fillColor = new Color(224, 93, 32, 106);
        this.graphics.strokeColor = new Color(255, 166, 55, 225);
        this.graphics.lineWidth = 2;
        for (const { x, y } of valid) {
            const center = this.grid(x, y), side = this.grid(x + 1, y);
            const dx = Math.abs(side.x - center.x) * .9, dy = Math.abs(side.y - center.y) * .9;
            this.graphics.moveTo(center.x - dx, center.y);
            this.graphics.lineTo(center.x, center.y + dy);
            this.graphics.lineTo(center.x + dx, center.y);
            this.graphics.lineTo(center.x, center.y - dy);
            this.graphics.close();
        }
        this.graphics.fill();
        this.graphics.stroke();
    }

    clear(): void { this.last = ''; this.graphics.clear(); }
}
