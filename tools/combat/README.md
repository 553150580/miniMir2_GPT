# 技能公式独立对拍

通过`./tools/dev python tools/combat/install_dotnet.py`复现已锁定的微软官方.NET10 SDK；首次无锁才选择最新稳定版。升级必须显式执行`./tools/dev python tools/combat/install_dotnet.py --upgrade`。每次均向官方清单确认同版本/平台/URL/SHA512，并核对包与已安装可执行文件，不静默更新版本；官方网络请求最多尝试3次。安装仅放入用户独立工具目录。宿主架构以实际检测为准；`tools/dev`读取`conf/toolchain/dotnet.lock.json`，隔离缓存并关闭遥测和首次体验，不修改系统.NET。

运行`./tools/dev python tools/combat/run_source_oracle.py`。工具先锁定Crystal本地源码SHA，逐字提取`MagicInfo.cs:13–22,180–222`和`Envir.FillMagicInfoList`各候选默认initializer，配最小类型与固定Random桩后由真实.NET编译执行。无需NuGet外部依赖。来源文件保持只读；不会执行参考服或加载账号数据库。

`.work/combat-modes/oracle/report.json`记录版本、来源行号/摘要、实际运行结果；`vectors.json`可供Go回归读取，其中随机偏移从0起、上界不包含。各rank0–3和两个技能随机项分别全枚举，包含烈火45/85/90与大整数浮点边界。`ppm-differences.json`明确标出合并整数倍率不能替代源float32的样本。

需要更新Go回归向量时，由服务端测试维护者执行`./tools/dev python tools/combat/run_source_oracle.py --go-fixture server/internal/devconfig/testdata/source-damage-vectors.json.gz`。该参数仅裁出13个输入/预期字段并以mtime=0生成确定性gzip；正常运行不会写服务端文件。向量不读取当前Excel或服务端待测函数，来源公式改变会先被SHA门禁拒绝。

这些是固定抽样公式证据，不证明目标1.80归属、来源实服数据库、完整随机序列、全伤害管线或玩法平衡。运行数值仍只维护唯一Excel；此处从只读源码生成测试向量，不作为运行配置。
