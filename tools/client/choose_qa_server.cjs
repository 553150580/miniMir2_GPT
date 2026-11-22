// 通过触屏选择当前分区第一个可用服，供单服开发及隔离浏览器验收复用。
async function chooseQAServer(page, scene) {
    await scene('ServerSelect');
    const handle = await page.waitForFunction(() => {
        const content = cc.find('Canvas/Safe/ServerList/View/Content');
        const row = content?.children.find(node => node.activeInHierarchy && node.getComponent(cc.Button)?.interactable);
        const transform = row?._uiProps?.uiTransformComp;
        if (!transform) return null;
        const box = transform.getBoundingBoxToWorld();
        const visible = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
        return {
            x: rect.x + (box.x + box.width / 2) / visible.width * rect.width,
            y: rect.y + (visible.height - box.y - box.height / 2) / visible.height * rect.height,
        };
    }, null, { timeout: 20000 });
    const point = await handle.jsonValue();
    await handle.dispose();
    await page.touchscreen.tap(point.x, point.y);
    await scene('RoleSelect');
}

module.exports = { chooseQAServer };
