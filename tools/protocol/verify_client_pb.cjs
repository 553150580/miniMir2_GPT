#!/usr/bin/env node
// 静态客户端Protobuf候选的线格式检查；不代表业务、鉴权或Creator构建验收。
'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { createRequire } = require('node:module');

const projectRoot = path.resolve(__dirname, '../..');
const clientRoot = path.join(projectRoot, 'client');
// 直接验证客户端自己的固定运行依赖和普通Node模块解析，不借用外部CLI的NODE_PATH。
const runtimeRequire = createRequire(path.join(clientRoot, 'package.json'));
const runtimeVersions = {};
for (const [name, expected] of Object.entries({ protobufjs: '8.8.1', long: '5.3.2' })) {
  const actual = JSON.parse(fs.readFileSync(path.join(clientRoot, 'node_modules', name, 'package.json'), 'utf8')).version;
  assert.equal(actual, expected, `客户端${name}运行版本与锁定版本不符`);
  runtimeVersions[name] = actual;
}
const Long = runtimeRequire('long');
const protobuf = runtimeRequire('protobufjs');
const minimal = runtimeRequire('protobufjs/minimal');
const generatedPath = path.join(clientRoot, 'assets/scripts/generated/protobuf/client_pb.js');
const declarationsPath = path.join(clientRoot, 'assets/scripts/generated/protobuf/client_pb.d.ts');
const generatedText = fs.readFileSync(generatedPath, 'utf8');
const declarationsText = fs.readFileSync(declarationsPath, 'utf8');
assert.equal(minimal.util.Long, Long, '运行时必须实际加载Long');
assert(!generatedText.includes('.toNumber('), '候选不能含64位到Number转换');
assert(!/number\s*\|\s*Long|Long\s*\|\s*number/.test(declarationsText), '64位声明不能放宽成number');
const candidate = require(generatedPath).minimir.v1;
const inputs = ['common.proto', 'account.proto', 'world.proto', 'assets.proto'];
const reflection = new protobuf.Root().loadSync(inputs.map(p => path.join(projectRoot, 'shared/proto', p)), { keepCase: true });
const ns = reflection.lookup('minimir.v1');
const hash = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const types = ns.nestedArray.filter(t => t instanceof protobuf.Type);
const integer64 = new Set(['int64', 'uint64', 'sint64', 'fixed64', 'sfixed64']);
const longChecks = [];
const byteChecks = [];
const messageChecks = [];

for (const type of types) {
  const Codec = candidate[type.name];
  assert(Codec && typeof Codec.encode === 'function' && typeof Codec.decode === 'function', `${type.name}缺少静态编解码`);
  const emptyBytes = Codec.encode(Codec.create()).finish();
  assert(Codec.decode(emptyBytes) instanceof Codec, `${type.name}空消息往返失败`);
  messageChecks.push(type.name);
  for (const field of type.fieldsArray) {
    if (integer64.has(field.type)) {
      const unsigned = field.type === 'uint64' || field.type === 'fixed64';
      const samples = unsigned ? ['0', '9007199254740993', '18446744073709551615'] : ['0', '9007199254740993', '9223372036854775807', '-9223372036854775808'];
      const escapedField = field.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      assert(new RegExp('\\b' + escapedField + ': Long;').test(declarationsText), `${type.name}.${field.name}未声明Long`);
      for (const decimal of samples) {
        const value = Long.fromString(decimal, unsigned);
        const message = Codec.create({ [field.name]: value });
        assert.equal(Codec.verify(message), null, `${type.name}.${field.name}Long输入无效`);
        const decoded = Codec.decode(Codec.encode(message).finish());
        assert(Long.isLong(decoded[field.name]), `${type.name}.${field.name}解码未保留Long`);
        assert.equal(decoded[field.name].toString(), decimal, `${type.name}.${field.name}精度变化`);
        assert.equal(decoded[field.name].unsigned, unsigned, `${type.name}.${field.name}符号变化`);
      }
      longChecks.push({ message: type.name, field: field.name, type: field.type, cases: samples.length });
    }
    if (field.type === 'bytes') {
      const value = Uint8Array.from([0, 1, 2, 3, 0x7f, 0x80, 0xfe, 0xff, 10, 20, 30, 40, 50, 60, 70, 80]);
      const source = field.repeated ? [value, new Uint8Array(), value.slice(0, 3)] : value;
      const decoded = Codec.decode(Codec.encode(Codec.create({ [field.name]: source })).finish());
      if (field.repeated) {
        assert.equal(decoded[field.name].length, 3);
        assert.deepEqual(Array.from(decoded[field.name][0]), Array.from(value));
        assert.equal(decoded[field.name][1].length, 0);
        assert.deepEqual(Array.from(decoded[field.name][2]), [0, 1, 2]);
      } else {
        assert.deepEqual(Array.from(decoded[field.name]), Array.from(value));
      }
      byteChecks.push({ message: type.name, field: field.name, repeated: field.repeated });
    }
  }
}

const roleId = Uint8Array.from({ length: 16 }, (_, i) => i * 17);
const maxU64 = Long.fromString('18446744073709551615', true);
const hugeMoney = Long.fromString('9007199254740993', false);
const nested = candidate.HostingReport.create({
  hosting_session_id: roleId,
  used_seconds: maxU64,
  kills: Long.fromString('9007199254740993', true),
  experience: hugeMoney,
  earned: candidate.WalletState.create({ gold: hugeMoney, bound_gold: Long.ZERO, yuanbao_units: hugeMoney, bound_yuanbao_units: Long.ONE, revision: maxU64 }),
  drops: [candidate.ItemView.create({ instance_id: roleId, item_id: 1, quantity: hugeMoney, bound: true, version: maxU64 })],
  stop_reason: 0,
  settlement_sequence: maxU64,
});
assert.equal(candidate.HostingReport.verify(nested), null);
const nestedDecoded = candidate.HostingReport.decode(candidate.HostingReport.encode(nested).finish());
assert.equal(nestedDecoded.earned.yuanbao_units.toString(), '9007199254740993');
assert.equal(nestedDecoded.drops[0].quantity.toString(), '9007199254740993');
assert.equal(nestedDecoded.drops[0].version.toString(), '18446744073709551615');
assert.deepEqual(Array.from(nestedDecoded.drops[0].instance_id), Array.from(roleId));
const text = candidate.LocalLoginRequest.create({ account: 'wire_test_actor', password: 'wire_test_only' });
const textDecoded = candidate.LocalLoginRequest.decode(candidate.LocalLoginRequest.encode(text).finish());
assert.equal(textDecoded.account, 'wire_test_actor');
assert.equal(textDecoded.password, 'wire_test_only');
const utf8 = candidate.RoleSummary.create({ name: '测试角色甲' });
assert.equal(candidate.RoleSummary.decode(candidate.RoleSummary.encode(utf8).finish()).name, '测试角色甲');

// 对已知消息的畸形线格式必须拒绝；不以unknown状态冒充验证通过。
assert.throws(() => candidate.RoleKey.decode(Uint8Array.from([0x0a, 0x10, 0x01])));
const report = {
  status: 'passed',
  validation_scope: 'candidate_static_protobuf_wire_codec_only',
  node_version: process.version,
  runtime_long_loaded: true,
  int64_number_conversion: false,
  message_count: messageChecks.length,
  message_names: messageChecks,
  int64_field_count: longChecks.length,
  int64_boundary_cases: longChecks.reduce((sum, x) => sum + x.cases, 0),
  int64_checks: longChecks,
  bytes_field_count: byteChecks.length,
  bytes_checks: byteChecks,
  nested_wallet_and_drops: 'passed',
  utf8_strings: 'passed',
  truncated_known_message_rejected: true,
  proto_sources: inputs.map(name => ({ path: 'shared/proto/' + name, sha256: hash(path.join(projectRoot, 'shared/proto', name)) })),
  generated_files: [generatedPath, declarationsPath].map(p => ({ path: path.relative(projectRoot, p), sha256: hash(p), bytes: fs.statSync(p).size })),
  runtime_dependency_scope: 'client/node_modules',
  runtime_dependencies: runtimeVersions,
  business_logic_verified: false,
  authentication_verified: false,
  creator_project_build_verified: false,
  limitations: ['协议仍为研发草案；未实现服务器业务或账号认证。', '本检查不替代Creator实际打包、真机或网络互通验收。', 'bytes线格式不会替服务端校验永久ID必须16字节等业务约束。', '业务调用必须显式构造Long；禁止Number/parseInt/long.toNumber传递64位值。'],
};
const outputArg = process.argv.indexOf('--report');
if (outputArg >= 0) {
  const output = path.resolve(projectRoot, process.argv[outputArg + 1]);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(report, null, 2) + '\n');
}
console.log(JSON.stringify({ status: report.status, scope: report.validation_scope, message_count: report.message_count, int64_field_count: report.int64_field_count, int64_boundary_cases: report.int64_boundary_cases, bytes_field_count: report.bytes_field_count, creator_project_build_verified: false }));
