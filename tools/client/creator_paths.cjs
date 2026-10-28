// 只定位安装目录，不写引擎、不改全局模块路径；Mac 与 Windows 共用同一版本约束。
const fs = require('node:fs');
const path = require('node:path');
const app = process.env.MINIMIR_CREATOR_APP ?? (process.platform === 'win32'
    ? 'D:/Program Files/CocosCreator/3.8.8'
    : '/Applications/Cocos/Creator/3.8.8/CocosCreator.app');
const resources = path.join(app, process.platform === 'win32' ? 'resources' : 'Contents/Resources');
const typescriptRoot = path.join(resources, 'app.asar.unpacked/node_modules/typescript');
const typescript = require(typescriptRoot);
if (!typescript.version.startsWith('5.8.')) throw new Error(`需要 Creator 3.8.8 自带 TypeScript 5.8，实际 ${typescript.version}`);
module.exports = { app, resources, typescriptRoot, typescript, engineRoot: path.join(resources, 'resources/3d/engine') };
