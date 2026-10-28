const fs=require('node:fs');
const path=require('node:path');
// 保留修前探针原文；只将同一输入的预期反向为全部解绑，并补新实现的保存world引用。
let source=fs.readFileSync(path.join(__dirname,'dynamic-frame-probe.cjs'),'utf8');
source=source.replace('viewport:new Node(\'Viewport\'),visuals:','viewport:new Node(\'Viewport\'),world:new Node(\'SavedWorld\'),visuals:');
source=source.replace("assert.deepEqual(released,['active skill Sprite','active attached status Sprite','actor retired earlier this frame']);","assert.deepEqual(released,[]);");
new Function('require',source)(require);
