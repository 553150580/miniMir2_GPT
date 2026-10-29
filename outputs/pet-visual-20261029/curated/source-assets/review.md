# 17 新生表现来源与资源审查

本轮仅只读核对源码、现有正式资源、原 WIL/WIX、补充 Lib/声音与既有下载元数据；没有运行来源程序、DB、Creator，没有改 Excel、生成器、正式 manifest/profile 或布局。接触表只是将原格按原尺寸排列的 `.work` 审查图片，不是新造美术。

## 可实施结论

可补的最小图像集合是 **Magic 原帧 1500–1509 起手**和 **Mon4 原帧 2860–2869 新生**，共 20 个当前 manifest 尚未收录的原格。起手 10 帧合计 600ms/additive；新生 10 帧各 100ms、共 1000ms/additive、所有方向共用同一序列。新生动作须消费权威的一次新生事实；健康重观、普通 Recall、重建节点均不应重播。

后两项原 FrameSet 参数已通过补找到的 `Monster/78.Lib` 实证，不再仅由 Mon4 的 `+340` 位置猜测。该 Lib 的历史下载链尚未找回，保留“本地技术资产来源候选”；当前 Mon4 与 Lib 的透明轮廓和锚点完全相同，但 RGB 有小幅差异，不能写成逐字节相同的来源版本。沿用本项目已有 Mon4 原像素符合现有跨来源候选边界。

## 首次 Extra / Summoned 合同

| 本项目已复制源码 | 事实 |
| --- | --- |
| `资料参考/crystal/Server/MirObjects/Monsters/BoneFamiliar.cs:12,15–25` | 初方向 DownLeft；`Spawned` 先 `base.Spawned()`，随后才 `Summoned=true`；`GetInfo().Extra=Summoned`。 |
| `资料参考/crystal/Server/MirObjects/MapObject.cs:339–354` | `base.Spawned` 将对象加入环境，并在353广播信息；因此首次广播 Extra=false，后续 GetInfo 为 true。 |
| `资料参考/crystal/Client/MirObjects/MonsterObject.cs:222–232` | BoneFamiliar 等仅在 `!info.Extra` 时排入 Appear。 |
| 同文件 `:276,357,633–649` | 优先使用 BodyLibrary 的 Frames；帧号为 `Start+(Count+Skip)*Direction+FrameIndex`；Appear 读取自己的 FrameInterval 并调用 PlaySummonSound。 |
| 同文件 `:648–659` | 骨骼 Appear 没有额外地面 Effect；448起的特殊 Effect 只属于 HellKnight 等分支。 |

因此500ms是召唤技能到期，2000ms是来源 SpawnReady 边界，二者都不能拿来替代出场动画的1000ms；也不能通过 CastAccepted+500ms在客户端自行认定新生成功。

## 起手与新生原格、时序、混合

| 内容 | 确切依据 | 本项目资源状态 |
| --- | --- | --- |
| 起手 | `PlayerObject.cs:1724–1726`：`Effect(Libraries.Magic,1500,10,Frame.Count*FrameInterval,this)`；`Frames.cs:166` 的 Spell 为6帧×100ms，故600ms。`Effect.cs:23,38–50,129–132` 默认 Blend=true、跟随 owner，按原图 offset 绘制。 | Magic1500–1509可见10/10；全部未在当前 manifest。不得用1510–1519：这10格都是4×1且alpha全0。 |
| 新生 | 下述真实 Monster78.Lib 的 Appear：start224/count10/skip−10/interval100/reverse=false/blend=true，因此方向stride0。`MonsterObject.cs:4306–4309` 对 Frame.Blend 调 DrawBlend。 | Mon4 2860–2869可见10/10；接触表可见骨骼从蓝白云团中出现。2870–2879均4×1、alpha全0，不能扩成20帧或方向变体。 |
| 原索引体系 | `Shared/Enums.cs:307` BoneFamiliar=78；`:1196` SummonSkeleton=65；原客户端按 `Libraries.Monsters[(ushort)BaseImage]` 选库。 | Crystal Lib compact224与WIL原索引2860不是同一索引空间，不能把224直接作为Mon4帧号。 |

原 WIL/WIX 严格通过现有 `inspect_wil.py` 读取：`#ILIB v1.0`、256色板、version0、未压缩、8位色板索引、bottom-up、无行padding；PNG应保持原尺寸、原offset，不裁切、不放大重绘。Magic尺寸40–72×48–79，Mon4新生尺寸56–120×48–131，逐帧offset见 `audit.json`。

加色依据还包括 `MLibrary.cs:692–709` 的 DrawBlend→SetBlend，以及 `DXManager.cs:353–379` 默认 NORMAL 的 SourceAlpha/One；不是仅凭字段名推断。

### 找回的 Monster78.Lib

路径：`D:/123DownLoad/GoMir2_DS/.local/crystal-actors-src/Data/Monster/78.Lib`。不是同名NPC/CWeapon库。大小382901字节，SHA256 `de0033d3a81d04277a679d68a942516c7365c086d27ddd650ae5ea5cfac3a678`。

按 `MLibrary.cs:513–545` 与 `Frames.cs:231–242` 只读解析：版本3、234张图、frameSeek382617、8个动作；每项为1 byte action、8个 little-endian int32、2个bool，共35字节；动作表精确读至EOF。Appear(action27)为 `{start:224,count:10,skip:-10,interval:100,effect_*=0,reverse:false,blend:true}`。原图头依 `MLibrary.cs:880–901` 读取，图像载荷是 GZip 包的 BGRA，符合 `:1062–1092`，本轮未调用 DirectX 或来源程序。

Lib224–233对Mon4 2860–2869逐帧实测：width、offset完全相同；Lib较高的底部行全部透明0；裁去新增底行后alpha逐字节相同。RGB不是完全相等：可见通道最大差5–7，均差0.79–1.92；不得声称原RGBA相同，也不应把观察到的四倍数补高直接解释为当前格式是DXT（当前载荷实测GZip）。完整8动作与10对统计见 `supplement-audit.json`。

## 声音：已找到原索引对应文件

`PlayerObject.cs:1726` 用 `20000+Spell*10`，Spell65得到20650；`MonsterObject.cs:217,3836–3839` 用 BoneFamiliar78×10+5得到785。`SoundManager.cs:75–95` 先查SoundList覆盖，否则分别映射到 `M65-0`、`078-5`；`CachedSound.cs:16–37` 按声音目录和支持扩展名加载。

这两文件不在当前primary_180/Wav中。已在 `D:/123DownLoad/CrystalLocal/downloads/client-resources/Sound/` 找到：

| 文件 | 大小 / SHA256 | 格式 |
| --- | --- | --- |
| M65-0.wav | 200452 / `575d30ac3f7dec94ed6bb8407607970dd725b6d387ba263edd6843b1c49e51f3` | PCM16、2声道、44100Hz、50102样本帧、1.13610秒 |
| 078-5.wav | 265440 / `13e6015011d1bfdcd5ff0b094dec72c2eeec05da9eafae43cbbfe31080a0e48a` | PCM16、2声道、44100Hz、66349样本帧、1.50451秒 |
| SoundList.lst | 10317 / `0250a0ce5790e0f72c64be2f28f423749d9669c242f158c3afaec3de7ad9d52f` | 此表没有20650/785覆盖，故适用源码默认命名。 |

`PList.json` 按Name/Length匹配以上三件；`client-resources-verification.json` 记载 ManifestUrl=`https://ftp.mirfiles.co.uk/resources/mir2/crystal/patch/PList.gz`，2026-10-15T12:07:45+08:00核4681/4681、missing0；另一份本地 `miniMiro2/Miro2_GPT/资料参考/.../Sound/` 三文件SHA也相同。这里只是既有本地下载记录和当前字节核对；PList不含内容签名，本轮没有联网重新验签。声音的完整播放时长与600/1000ms图像动画不同，不应为凑动画时长擅自裁剪。

primary中的 M17-1/M17-3 仅是现存未认证声音，不能凭本作技能ID17替代Crystal65原声。当前项目 `client/assets` 中没有wav/ogg/mp3资源，现WorldAssets/GameWorldView也没有音频播放链。

## 当前正式资源与最小缺项

审查时 `development/manifest.json` 有3332帧。`pet_skeleton` 仍是candidate、development_only、official_180_animation_acceptance=false；Mon4 base2520，只有standing/walking/attack/struck/die。`effects['17']` 不存在，Magic1500–1509和Mon4 2860–2869均未导出。

`WorldAssets.ts:147–155` 预载所选profile动作；`:158–164` 对公开且active的17会要求effects17，缺失会拒prepare。当前主22技能不含17，所以此缺项此前未触发。隔离UI public必须从同源17候选投影并明确隔离用途，不能让主22资源加载成功冒充已验17。

建议后续受控实施：

1. 数值/帧映射若新增维护，放唯一Excel视觉合同，再由同一个资源生成器读取；起手、出场、音频角色分别命名，勿把500ms技能延迟、2000ms就绪和1000ms出场重复当同一字段。
2. 仅增20张原格和effects17、pet_skeleton.appear。Appear方向stride0/非循环/100ms/additive；保留站走等原格字节。起手跟随施法者，600ms/additive；没有源码依据的额外落点Effect不补。
3. 一次权威born事实匹配World/scene/baseline/handle/Life与时点后消费。先到baseline/迟到event/健康重入/Recall/已死body都要有明确行为，不能节点创建即appear。源1000ms可以在迟到时按实际born时间采样或过期跳过，不能重置成完整新一轮。
4. 若本阶段接声音，只导入上述两件来源文件并建立可释放的AudioClip持有与播放门禁；语义同起手/真实born，不因健康重观重播。若暂不接，验收报告明确无声即可，不能把缺声写成已忠实完整。

## 本轮证据与范围

- `audit_sources.py` / `audit.json`：17个当前输入摘要、40帧完整只读decode统计、正式manifest覆盖、主skillIDs、音频空集；源WIL/WIX和已读正式文件前后摘要不变。
- `contact_sheets.py` / `contact-sheets.json` / 两张 `*-native-contact.png`：只在.work原尺寸排格，已人工查看骨骼云团新生与起手；原源文件前后摘要不变。
- `audit_supplement.py` / `supplement-audit.json`：真实Lib FrameSet及像素对拍、声音元数据/下载记录、另一本副本SHA、负向枚举。
- 精确负向范围：Python Path.rglob不受gitignore影响，CrystalLatest/Client没有.Lib；GoMir2/client/assets/world仅maps/players，6个library JSON全是CArmour/CWeapon/CHair，没有monster78。后来在DS隐藏.local找到正确库，不能将早期局部“没找到”扩大为硬盘缺件。

本轮只证明来源参数、可解码/素材匹配和结构缺项；未证明Creator渲染、播放、真实UI操作、新生业务或容量已经通过。
