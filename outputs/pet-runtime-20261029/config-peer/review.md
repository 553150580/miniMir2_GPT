# rule16配置独立复核

结论：当前真实候选与fixture未发现阻断问题。本次只读Go/测试及真实JSON，没有运行Go，没有改冻结输入。独立交叉核对命令 `./tools/dev.ps1 python3 .work/pet-runtime-20261029/config-peer/audit_candidate.py` exit0；13个相关输入摘要前后相同，见`candidate-cross-check.json`。

- `devconfig/pet_runtime.go:92`先按类型递归检查全部导出JSON字段，拒绝未知键、缺项与非指针null；`decodePetContract:149`再严格解码并拒绝尾随内容。唯一数值指针是成长阈值，随后Validate要求仅末级为null。真实Base恰好含Fighter全部JSON字段，undead在Definition顶层。
- `ValidatePetRuntime:331`与`Fixture.UnmarshalJSON`分别覆盖版本/缺配置和显式null。独立catalog强制development_only=true、runtime_enabled=false；rule15混入PetRuntime被拒；rule16缺配置被拒。当前legacy模式Base逐字段等于旧Pet，候选与fixture定义、预算完全相同，17与护符保持关闭。
- `Clone:252`复制两组rank切片、Growth切片及每个非空阈值指针；Base无引用字段。`FighterAt:268`先限level/maximum，7类属性加法先验非负及MaxInt64边界，间隔用除法判断乘法是否可安全执行，超出减量返回明确最小间隔。模板Level15不随训练级改变。
- `game/pet_training.go:103–128`成功credit只应用一次；只有一个if升级，扣一个阈值并保留carry；满本birth上限后不再捕获/累积经验，保留最后升级的carry；成长只改MaxHP/MaxMP/Fighter，不回满当前HP。原宠ref消失后不恢复临时对象。该部分同时与既有训练专项的unknown、旧birth、满级carry和零EXP用例逐项对照，未把静态阅读写成新执行通过。
- `devconfig/pet_runtime_test.go`已有5个顶层方法、21项畸形候选子例，覆盖来源向量、严格输入、深复制/溢出、版本与实际配置同源。范围正确，没有用默认候选顶替真实文件，也没有skip缺失candidate。

边界：Go loader检查typed结构和语义，不重新读取两本Excel或执行source_evidence摘要认证；来源摘要由上游编译器/--check及阶段fixture锁维护。本次另外核对这些摘要确实匹配当前两书、schema和新编译器，不能据此宣称运行加载完成了数字签名验证。128仍是每owner身体预算，不是全进程或整机容量验收。
