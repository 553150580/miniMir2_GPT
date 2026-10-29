# 尸体GUI工具交付

成功入口由 final-run.json 精确锁定 `run-fae0a7a32d924a0cadc240c9e3a2f812/report.json`，SHA256 `bea98ea872c9f3d3cde6ebdfed4254c82e1f0a011b8371dddd9baf5196ea3aed`；静态报告目录 `static-final-editbox`。

本轮实际执行：`./tools/dev.ps1 python tools/client/prepare_pet_runtime_qa.py --run --report-dir .work/pet-runtime-20261029/gui-review`，最终会话23573退出0；8项Python、7项CJS、node --check及不启动prepare通过。

真实Creator/Chrome+专属QA三节点/随机数据库，正常保存UI注册、道士创角、入场，5项GUI检查通过，0页面错误、0警告。原活宠HP1经怪物真实首击变为同handle2/Life0尸体；正常触屏详情及PetPanel区分；换角新票保留原尸体；原15秒QA期限权威移除、节点失效、Sprite解绑及详情撤销。截图01至05均在成功run/browser。已查看02/03/05截图，文本分别为宠物尸体、当前没有存活宠物、该实体已离场。

本工具未更改产品TS/Go/协议/Excel/场景/Prefab；主fixture/public/两书/保存布局/构建输入及4工具在本轮前后稳定。凭据只驻内存、不记原始登录包。节点以PID/start_ticks/image/RunID归属校验，数据库仅精确完整application_meta marker核实后删除，健康instance三节点与本轮runtime.json匹配。finally已停专属QA、删除专属DB、恢复主三节点健康，cleanup_errors为空。

范围限制：隔离cfg将HP调1、怪攻击20高命中和长动作、尸体TTL15秒；不等于原180秒TTL、17召唤/耗符购符、自然成长、冷重建、128容量、真实手机或完整1.80认证。NoMaster友方过滤由game专项证据承担，本GUI不假开14/17、不seed corpse/技能/绑定/临时位置。

## 保留的历史失败

- `run-adebc3d5f4f94026a13e3ef0d79c1698`：工具首版自建旧标记集合使Migrate拒绝认领；未启动浏览器，修为标准application_meta专属完整marker。 helper报告SHA256 `1f3b9e24db878f873b6480eabe302dde07c83f2c4dc750bfcd7ef05a7deb2c09`；自有QA清理/主恢复均为true，历史原字节保留。
- `run-7da62ae6a2e646699eca69a28a7c55a7`：前4GUI检查通过；健康重入后9点采样过粗，被邻实体hit矩形遮挡，未找到真实触屏点；改49点外角密采样并补几何反例。 helper报告SHA256 `d6941e31d4a3c67ef7299586b8362026ddee6b141c6d13acd51c7028e9ed76e4`；自有QA清理/主恢复均为true，历史原字节保留。
- `run-d3bce423248449849b05c2052c2d9ce7`：未观察到初始宠物，全部基线pets=[]；创角职业触屏尚无确认，未证明实际job，后续加job只读确认和入场活宠断言。 helper报告SHA256 `a325d6f33d259ffa4178bb88e837a1e02b890b48d160a1c3fe1a4b5669d224f4`；自有QA清理/主恢复均为true，历史原字节保留。
- `run-b9b82c89c8f04a4fb648ee594947b352`：工具把真实文案“创建职业：男道士”错写为“创建职业：道士”导致guard误拒；未创角/入场，现身份只按job=2。 helper报告SHA256 `a20f55a2f15d93a9c25ebea438f12c3a02860e510d80fb8a89a74a17da41a78a`；自有QA清理/主恢复均为true，历史原字节保留。
- `run-c4f8c8257d984de0bbda03e509c9f059`：Login保存EditBox首次触屏未激活input；未创角/入场，场景加入Loading关闭及双RAF等待。 helper报告SHA256 `b160dc39e3ceb4877d305716889de70feaa9184dfb5c54ccd3d67575f5bb3009`；自有QA清理/主恢复均为true，历史原字节保留。
- `run-792567e3ef3c4b3da167839b74b3b936`：工具重试仅看任一input可见，切Password前未先tap，复用旧Account输入框，注册未完成；修回每个目标先正常tap并补实际helper反例。 helper报告SHA256 `01861cb7a0d87605ec3d88e5f031ab4dd58f6bbd9c6afb90d6c9a3eb5a89df46`；自有QA清理/主恢复均为true，历史原字节保留。

上述失败均为QA工具/前置条件问题，本任务未据此改产品。正常主配置Town最终7checks/0error/0warning，UI最终6checks/0error/1orientation warning，见本阶段browser-town-selection.json与browser-ui-selection.json精确SHA指针。

## 正常主配置回归历史与交还

- browser-town/ 首轮未确认job，正常道士宠物断言失败；保留原报告和截图。
- browser-ui/ 首轮保存EditBox触屏前置未就绪，input可见等待超时；保留原现场。
- browser-ui-final/ 新加isFocused门禁超时；保留原现场。最终browser-ui-final-2复用已通过的普通触屏输入前置，6项业务断言保持。

成功GUI后已恢复main并顺序完成正常Town/UI回归，DB窗口归还root。root随后为最终Go受控stop main，不代表本helper恢复失败。最终client-review/report.json已重新冻结220输入并全348/strict/PB通过，旧216证据在historical-before-town-readiness中原字节保留。
