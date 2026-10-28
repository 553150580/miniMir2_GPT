# 21升级预览生产缺陷修复

根因：合法rule15 TemporalFlux的`duration_kind=fixed_ms`、`duration_seconds_by_level=[]`。原`skillQuote.Proto`将它归入道士状态分支并访问该空数组，升级21的实际报价会panic；并非只需更新旧长度断言。

经root授权，仅修改`persistence/skill_assets.go`及对应测试。新增Flux分支只生成`mp_cost`、`duration_ms`、`teleport_mana_penalty_percent`，不生成攻击、命中、道术或护符比较项。21三次升级测试使用41007ms与11/23/37/53的隔离值，验证从实际Buff字段取值，不填造当前30秒/30%的候选常量；同时加强所有非攻杀技能不得显示攻击上限的断言。

实际证据：`before.txt`为修前原单元真实panic，exit1，总8.637秒；`after.txt`为原单元与新增21回归的race通过，总10.215秒；`persistence.txt`为完整persistence包race通过，27方法、114测试节点（含子测试），0失败、0跳过，包5.266秒/受控命令10.259秒，exit0。各日志配对JSON记录实际命令及输入摘要。未启动Mongo，未运行全量Go。修改已冻结，最终全包由root重验。

客户端只读结论：SkillModel详情已处理固定时长；通用SkillNumericComparison协议接受字符串key，无需新增PB。升级报价由SkillAssetsModel渲染，原中文名称字典缺两个新key，会回退“数值对照”；已报告root，由客户端owner获得授权补标签和展示测试，本任务未改TS。

最终唯一配置只读复核未发现新增阻断：书13为候选、独立商店27价格2000金币，升级211/212/213沿原书2/3/4本与页5/10/20，不新增初始赠书；21仅手动、无材料，MP10+rank×3仍为临时施法成本。Flux固定30000ms唯一维护，逐阶30%只在属性数组，秒数数组为空，成功后置effect21独立于cast_impact，失败不触发；keep_values刷新、非死亡移除及健康World移交保持均为明确合同。初始绑定池仍关闭。NoTeleport两map均显式false，Town原静态位图与闭门走路位图分离；跨引擎来源与具体数值仍为开发候选，不宣称完整1.80认证。本报告不替代实际21施法、WS、Creator或容量验收。
