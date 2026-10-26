#!/usr/bin/env node
// 仅调用用户级已锁定工具，不使用npx动态安装或修改Codex捆绑依赖。
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { spawnSync } = require('node:child_process');
const root = path.resolve(__dirname, '../..');
const tools = process.env.MINIMIR_NODE_TOOLS || path.join(os.homedir(), '.local/share/minimir-toolchain/node-tools');
const expected = { protobufjs: '8.8.1', 'protobufjs-cli': '2.7.1', long: '5.3.2' };
for (const [name, version] of Object.entries(expected)) {
  const metadata = JSON.parse(fs.readFileSync(path.join(tools, 'node_modules', name, 'package.json'), 'utf8'));
  if (metadata.version !== version) throw new Error(`${name}版本应为${version}，实际为${metadata.version}`);
}
fs.mkdirSync(path.join(root, 'client/assets/scripts/generated/protobuf'), { recursive: true });
function run(file, args) {
  const r = spawnSync(process.execPath, [file, ...args], { cwd: root, stdio: 'inherit' });
  if (r.error) throw r.error;
  if (r.status !== 0) process.exit(r.status || 1);
}
run(path.join(tools, 'node_modules/protobufjs-cli/bin/pbjs'), [
  '-t', 'static-module', '-w', 'commonjs', '--force-long', '--keep-case', '--no-convert',
  '--root', 'minimir_v1_candidate', '-p', 'shared/proto', '-o', 'client/assets/scripts/generated/protobuf/client_pb.js',
  'shared/proto/common.proto', 'shared/proto/account.proto', 'shared/proto/world.proto', 'shared/proto/assets.proto',
]);
run(path.join(tools, 'node_modules/protobufjs-cli/bin/pbts'), ['-o', 'client/assets/scripts/generated/protobuf/client_pb.d.ts', 'client/assets/scripts/generated/protobuf/client_pb.js']);
run(path.join(root, 'tools/protocol/verify_client_pb.cjs'), ['--report', 'conf/toolchain/client-pb-verification.json']);
