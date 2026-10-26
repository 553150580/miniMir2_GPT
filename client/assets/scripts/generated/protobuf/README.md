# 客户端 Protobuf 候选

本目录由 `shared/proto` 四份草案通过真实 `protobufjs-cli` 生成，包含静态 CommonJS JavaScript 和 TypeScript 声明。生成与线格式检查通过不表示服务器业务、鉴权、Creator 构建或真机互通已经验收。

在项目根目录执行：

```bash
node tools/protocol/generate_client_pb.cjs
node tools/protocol/verify_client_pb.cjs --report conf/toolchain/client-pb-verification.json
```

工具默认安装于 `~/.local/share/minimir-toolchain/node-tools`；需要迁移时设置 `MINIMIR_NODE_TOOLS`。生成脚本核对固定版本：`protobufjs@8.8.1`、`protobufjs-cli@2.7.1`、`long@5.3.2`。对应安装清单与完整依赖锁保存在 `conf/toolchain/node-tools.package*.json`。

生成参数保留 `snake_case` 字段，使用 `--force-long` 保证64位字段的 TypeScript 类型为 `Long`；`--no-convert` 排除可能把64位值转成不安全 Number 的对象转换辅助方法。客户端通过 `Long.fromString(decimal, unsigned)` 或精确的32位高低段构造64位值，禁止经 Number、parseInt 或 `.toNumber()` 中转。

客户端独立`package.json`和`package-lock.json`固定运行依赖 `protobufjs@8.8.1` 与 `long@5.3.2`，CLI仍在外部用户工具目录。生成文件使用正常包名，Node检查直接从`client/node_modules`解析客户端实际依赖，不使用外部CLI的`NODE_PATH`；正式Creator打包器集成仍待验收。

本轮检查覆盖全部草案消息的编码/解码、所有64位字段的精确边界、bytes及重复bytes、嵌套钱包与物品、UTF-8字符串和已知消息的截断拒绝。`int64/uint64` 超过 `2^53` 的值保留为 Long。具体结果与输入摘要见 `conf/toolchain/client-pb-verification.json`。

安装来源为 [npm官方registry](https://registry.npmjs.org/)、[protobuf.js官方仓库](https://github.com/protobufjs/protobuf.js)和[官方CLI说明](https://github.com/protobufjs/protobuf.js/blob/master/cli/README.md)。`--force-long` 与静态代码选项均按该CLI说明使用；Long精确64位语义参考 [long.js官方仓库](https://github.com/dcodeIO/long.js)。
