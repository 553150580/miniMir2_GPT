# 城镇 NPC 原版外观接入

规则 30 的银杏山谷、边界村和比奇城分别配置 6、6、9 位 NPC。独立外观清单以原有 NPC 编号关联工作簿中已登记的外观号。地图格、原商店职责、交互入口和碰撞继续使用同一玩法配置。

当前 11 个外观来自本机参考客户端的 `npc.wil` 和 `npc.WIX`。已按原图库摘要、原画尺寸、透明度与偏移检查全部 132 张站立帧。常见外观采用 MA35，边界守将 27 采用 MA37 起点 1680。两类均有三向各四帧，间隔 200 毫秒。来源表没有方向列，试玩统一采用原序列的第一个方向。原包像素校验与关系取证只代表本机开发范围，版本来源、发行权利和所有 NPC 行为另行核验。 [外观来源记录](../资料索引/城镇NPC原版外观来源.json)

将原库只读导入 `.work` 下的独立 Creator 工程，先生成再核对实际文件。输出目录为该工程的 `assets/resources/development`。

```powershell
.\tools\dev.ps1 python tools/resources/prepare_town_npc_wil_preview.py 'D:/123DownLoad/Miro2_1.80/传奇1.80客户端/Data' --output '<独立工程>/assets/resources/development'
.\tools\dev.ps1 python tools/resources/prepare_town_npc_wil_preview.py 'D:/123DownLoad/Miro2_1.80/传奇1.80客户端/Data' --output '<独立工程>/assets/resources/development' --check
```

当前导出同时保存每一张原站姿的透明命中位图。客户端按正在播放的原画帧判定触摸，点击透明地面交给地图的原导航，点击可见像素仍可打开对应 NPC 交互。位图来自原 WIL 帧，原 PNG、脚点和碰撞格不变。重新运行原版素材导入可以刷新项目的生成清单。清单按内容生成 Creator 导入地址，原试玩旧索引仍可读取历史清单。

联机核验应逐城观察全部 NPC，确认边界守将没有借用通用玩家形象，站立帧可循环，脚点与头顶名牌一致，NPC 仍挡住原格并打开对应商店。离开城镇时清理动作引用，回城时只预载该城镇使用的展示序列。原图生成目录和本机 Web Mobile 包保留在独立试玩范围内。
