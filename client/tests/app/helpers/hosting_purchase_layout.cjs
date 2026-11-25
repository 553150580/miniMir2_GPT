const assert = require('node:assert/strict');

// 旧界面快照只移除本次追加的托管控件，以及为旧浏览器静态资源隔离所变更的场景 UUID。
module.exports = function previousHostingLayout(scene) {
    const root = scene[scene[0].scene.__id__];
    if (root._id === '4bfcf774-6b94-5164-88df-f9b744e24b0b') root._id = 'ca7d8bf1-9100-59e3-b59b-5ba330141897';
    const panel = scene[339];
    assert.equal(panel._name, 'Hosting');
    const added = new Map([[1993, 'BuyMinus'], [2000, 'BuyQuantity'], [2003, 'BuyPlus'], [2010, 'BuyQuote'], [2017, 'BuyReceipt']]);
    panel._children = panel._children.filter(ref => {
        const name = added.get(ref.__id__);
        if (!name) return true;
        assert.equal(scene[ref.__id__]._name, name);
        assert.equal(scene[ref.__id__]._parent.__id__, 339);
        return false;
    });
    return scene;
};
