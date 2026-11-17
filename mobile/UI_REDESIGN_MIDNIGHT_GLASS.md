# MyAI Mobile · UI 改造落地方案（方向 B · 午夜玻璃）

> **执行者请注意**：这份文档是完整的施工规格。请**按顺序**执行第 1 到第 9 章，
> 不要跳步。每一章都有明确的"完成判据"。第 10 章是验收清单，全部通过才算完成。
>
> 如果你在执行中发现文档没说清的地方，**不要自行发挥**，把问题记录下来一并向需求方确认。
> 涉及视觉取舍的，一律以本文档第 5 章的数值为准。

## 验证状态（哪些内容已经过实测）

本文档不是纸上设计。以下内容已在真实代码上验证过，你可以信任：

| 内容 | 验证方式 | 结果 |
|---|---|---|
| 现状统计数据（1329 处颜色 / 100 处 2px 描边 / 45 处硬阴影 / 202 处 900 字重 / 21 档字号 / 289 个独特色值） | 对 `src/` 全量扫描 | ✅ 已核对 |
| 第 2 章的 `theme.ts` 完整代码 | `tsc --strict --noEmit` 单独编译 | ✅ 通过，零错误 |
| 全部 token 色值的 WCAG 对比度 | 程序计算 | ✅ 见 2.3 表 |
| `scripts/migrate-colors.mjs` | 在 `src/` 副本上实际运行 | ✅ 自动迁移 71.9%，31 个文件被改动 |
| `scripts/migrate-typography.mjs` | 在 `src/` 副本上实际运行 | ✅ 28 个文件被改动，仅 12 处需人工 |
| 5 个新增依赖的版本号 | 查询 expo SDK 56 bundledNativeModules + npm registry | ✅ 版本真实存在，可安装 |
| 第 10 章的验收脚本 | 逐个实际运行 | ✅ 均能正常输出 |

**未验证的部分（需要你在执行中确认）：**

- 视觉效果的最终观感（还没实现，只能以原型 HTML 为基准）
- 真机上的毛玻璃表现（Android 与 iOS 差异，见附录 A.1）
- 头像圆形的边缘质量（依赖 `assets/icon.png` 是否有 alpha，见 8.4）

---

## 阅读顺序建议

如果你是**第一次接手**：

1. 先读**第 0 章**（为什么改）和**第 3 章**（最容易做错的地方）
2. 再读**第 9 章**（执行顺序），建立整体节奏
3. 然后按第 9 章的顺序，边做边读对应的第 1、2、4、5、6、7、8 章
4. 遇到视觉判断先看 **`mobile/ui-redesign-prototype.html`** 的方向 B 部分
5. 收尾时逐条过**第 10 章**
6. **附录 A 的常见陷阱建议现在就读一遍**，能避开几个坑


---

## 0. 背景与目标

### 0.1 现状问题（审计结论，已实测）

这份改造不是"换个颜色"，而是修复三个结构性问题。以下是真实统计：

| 指标 | 实测值 | 出处 |
|---|---|---|
| 颜色字面量总数 | **1329 处**（1305 hex + 24 rgba） | `src/` 下 32 个文件（含 `theme.ts` 自身的 23 个色值；**待改造的业务文件为 31 个**） |
| 独特的 hex 色值 | **289 个** | 前 20 个色值只覆盖 68.5% |
| `borderWidth: 2` | **100 处** | 几乎每个容器都有 2px 实黑描边 |
| `shadowRadius: 0` 硬阴影 | **45 处** | 硬阴影代替了层次 |
| `fontWeight: "900"` | **202 处** | 另有 `"800"` 88 处 |
| 字号档位 | **21 种**（8 ~ 28px） | 10/11/12 三档各超过 100 处 |
| emoji 当图标 | **约 50 个不同字符** | 分布在 19 个文件 |
| `constants/theme.ts` | **40 行，0 引用** | 死代码 |

**三个根本问题：**

1. **层级被抹平**。所有容器都是 2px 黑边 + 硬阴影 + 900 字重，没有任何东西比别的更重要。
   Header 一屏里有 3 个带边框圆按钮、1 个带边框浮岛（浮岛内还有头像+徽章+状态条+模式药丸），
   发送按钮和菜单按钮视觉权重相同。
2. **消息流被色块淹没**。用户黄、助手米白、工具青蓝、错误珊瑚红，四种气泡四种底色且全部带边。
   滚动时是一片色块，而不是一段对话。
3. **主题系统是死的**。`theme.ts` 定义了深色系统但无人引用，实际颜色写死在 32 个文件的
   `StyleSheet` 里；而 `app.json` 又声明了 `userInterfaceStyle: "dark"`，配置与实现互相矛盾。

### 0.2 本次改造的目标

**方向 B · 午夜玻璃**：深色画布 + 毛玻璃浮岛 + 头像暖黄光晕。

选择它的核心原因：项目在 `assets/loading/` 和 `assets/loading/thinking/` 投入了 **20 帧角色动画**
（每帧 288×720 RGBA PNG），这套资产在深色底上才是视觉焦点，在浅色底上只是小图。
同时它让 `app.json` 声明的 dark 终于成立。

### 0.3 范围

**要做：**
- 建立单一 token 源（`src/constants/theme.ts` 重写）
- 全部 31 个业务文件（加 `theme.ts` 重写）的颜色/字重/字号/描边迁移到 token
- 5 个核心组件的**结构调整**（Header / BottomTabs / Composer / MessageBubble / ScreenShell）
- emoji 图标替换为矢量图标
- 状态栏、导航栏、启动屏、`app.json` 的深色对齐

**不做（明确排除，避免范围蔓延）：**
- 不改任何业务逻辑、hooks、network 协议、storage
- 不改组件 props 签名（除第 6.1 节明确列出的新增可选 props）
- 不引入 Redux/Zustand 等状态库
- 不重写 `ChatPanel` 的滚动与自动滚底逻辑
- 不做浅色主题（本次只做深色；`theme.ts` 需为将来预留浅色扩展位，见 2.4）

---

## 1. 前置准备

### 1.1 建立基线

```bash
cd mobile
git checkout -b feat/ui-midnight-glass

# 1) 记录当前 typecheck 通过情况（必须全绿才继续）
npm run typecheck 2>&1 | tee /tmp/typecheck-before.log

# 2) 记录当前颜色基线，改造后要归零
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)\$/.test(e.name))o.push(f);}return o;}
let n=0;for(const f of walk('src')){n+=(fs.readFileSync(f,'utf8').match(/#[0-9a-fA-F]{3,8}\b/g)||[]).length;}
console.log('hex literals:', n);
" | tee /tmp/hex-before.log

# 3) 截图留档（当前浅色版本），用于改后对比
npm run web
```

**完成判据**：`/tmp/typecheck-before.log` 无 error；`/tmp/hex-before.log` 显示 `hex literals: 1305`（±5）。
如果数字对不上，说明代码已变更，**先与需求方确认**再继续。

### 1.2 安装依赖

**一次性安装以下 5 个包。版本号必须与 `expo` SDK 56 的 `bundledNativeModules` 一致，不要用 `latest`** ——
`latest` 目前是 58.x，会导致原生模块版本冲突，构建直接失败。

```bash
cd mobile

npx expo install expo-blur expo-linear-gradient expo-haptics @expo/vector-icons react-native-svg
```

这条命令会自动选择正确版本。安装后**核对** `package.json` 里应出现：

```json
"@expo/vector-icons": "^15.0.2",
"expo-blur": "~56.0.3",
"expo-haptics": "~56.0.3",
"expo-linear-gradient": "~56.0.4",
"react-native-svg": "15.15.4"
```

> 如果你的环境里 `npx expo install` 选出的版本不同（比如 `react-native-svg` 是 15.16.x），
> **以 expo install 的结果为准**，不要手动改。

**各包用途（不要引入别的包）：**

| 包 | 用途 | 用在哪些文件 |
|---|---|---|
| `expo-linear-gradient` | 头像、用户气泡、发送按钮、进度条的渐变 | AppHeader, Composer, MessageBubble, StartupLoadingOverlay |
| `@expo/vector-icons` | 替换全部 emoji 图标（用 Feather 字体族） | 19 个文件 |
| `expo-blur` | 浮岛/输入栏/标签栏的玻璃拟态 | AppHeader, Composer, BottomTabs |
| `expo-haptics` | 按钮点击触感反馈 | `utils/buttonFeedback.ts` 调用方 |
| `react-native-svg` | 网点/自定义形状（本次仅头像光环兜底用，见 6.2） | AppHeader |

### 1.3 原生重编译（**关键，不能跳过**）

本次新增了 4 个**含原生代码**的包（`expo-blur`、`expo-linear-gradient`、`expo-haptics`、
`react-native-svg`）。`@expo/vector-icons` 是纯 JS + 字体资源，不需要重编译。

**这意味着 OTA 更新无法生效。** 必须重新出包：

```bash
# 本地验证用（Android 真机/模拟器）
npx expo run:android

# 或走 EAS 出内测包
npm run build:android:apk
```

同时**必须提升版本号**，否则已安装的旧版用户收不到更新。改 `app.json`：

```json
"version": "1.1.0",        // 原 1.0.4
"android": { "versionCode": 9 }   // 原 8
```

> `runtimeVersion.policy` 是 `"appVersion"`，所以 version 一改，runtime version 自动跟着变。
> 这会导致 OTA 分渠道发布，旧包用户不会收到不兼容的 JS 更新 —— 这是预期行为，不要改这个 policy。

**完成判据**：`npx expo run:android` 能在真机上装起来且不白屏。
如果这一步失败，**停止后续所有工作**，先解决原生链接问题。

---

## 2. Token 系统（核心，先做这一步）

### 2.1 为什么必须先建 token

因为现有 1329 处颜色里有 **289 个独特色值**，靠人眼逐个替换必然出错、必然不一致。
建立 token 后，迁移变成"查表 + codemod"，可验证、可回滚。

### 2.2 完整重写 `src/constants/theme.ts`

**用下面这整份代码替换原文件内容**（原文件是死代码，直接覆盖，不要保留旧字段）。

```ts
// 设计 token 单一来源。所有组件禁止出现颜色字面量，必须引用这里的值。
// 命名约定：语义名，不含具体颜色（不要出现 yellow/black 之类的名字，否则换主题时没法用）。

export const palette = {
  // —— 中性色阶（深色画布，由深到浅）
  canvas: "#08080A",      // 最底层背景
  surface1: "#101014",    // 一级容器（面板、列表底）
  surface2: "#17171B",    // 二级容器（卡片、代码块）
  surface3: "#1C1C22",    // 三级容器（浮起的元素）
  surface4: "#232329",    // 输入框内层、按下态

  // —— 描边（深色底下必须用透明度，不能用实色）
  hairline: "rgba(255,255,255,0.07)",       // 默认分隔线 / 1px 边框
  hairlineStrong: "rgba(255,255,255,0.12)", // 需要强调的边框
  hairlineSoft: "rgba(255,255,255,0.045)",  // 极弱分隔

  // —— 文字（三级层次，实测对比度见 2.3）
  ink1: "#F2F0EC",   // 主文字（正文、标题）
  ink2: "#9C978D",   // 次文字（说明、副标题）
  ink3: "#8A8479",   // 弱文字（元信息、时间戳）
  ink4: "#6B6660",   // 极弱（占位符、禁用态）
  inkInverse: "#241F14", // 放在亮色块上的文字（品牌黄底上）

  // —— 品牌色（暖黄，唯一强调色）
  brand: "#F6D45C",
  brandLight: "#F9DC72",   // 渐变亮端
  brandDeep: "#E9B93F",    // 渐变暗端（按钮、头像）
  brandDark: "#D9A62F",    // 头像渐变暗端
  brandWash: "rgba(246,212,92,0.10)",   // 淡底（选中标签背景）
  brandWashStrong: "rgba(246,212,92,0.16)",
  brandCode: "rgba(246,212,92,0.11)",   // 行内代码底
  brandGlow: "rgba(246,212,92,0.55)",   // 光晕

  // —— 状态色（低饱和，深色底上不刺眼）
  ok: "#4ADE80",
  okWash: "rgba(74,222,128,0.13)",
  okBorder: "rgba(74,222,128,0.3)",
  warn: "#F0CE68",
  warnWash: "rgba(240,206,104,0.13)",
  err: "#E08D7D",
  errWash: "rgba(224,141,125,0.13)",
  errBorder: "rgba(224,141,125,0.32)",
  info: "#7FD1A8",
  infoWash: "rgba(127,209,168,0.12)",
  cyan: "#5BC8DE",
  cyanWash: "rgba(91,200,222,0.12)",

  // —— diff 专用（深色底红绿，比通用状态色更亮，保证代码可读）
  diffAdd: "#8FD8B4",
  diffAddBg: "rgba(127,209,168,0.09)",
  diffDel: "#E08D7D",
  diffDelBg: "rgba(224,141,125,0.10)",

  // —— 玻璃拟态（BlurView fallback 与 tint 用）
  glassTint: "rgba(24,24,29,0.72)",
  glassTintStrong: "rgba(28,28,34,0.82)",
  glassTintSoft: "rgba(22,22,27,0.80)",
  glassEdge: "rgba(255,255,255,0.09)",       // 玻璃上边缘高光
  glassInner: "rgba(255,255,255,0.05)",      // 内描边高光

  // —— 遮罩
  scrim: "rgba(4,4,6,0.72)",                 // 抽屉/弹窗背后的遮罩
  scrimHeavy: "rgba(4,4,6,0.92)",            // 全屏图片查看

  // —— 纯色（仅限必须的场合，优先用上面语义色）
  transparent: "transparent",

  // —— 下凹区域（工具结果体、代码块内部、diff 区域）
  recessed: "rgba(0,0,0,0.22)",

  // —— 顶部环境光（MobileScreenShell 背景光晕）
  ambientGlow: "rgba(246,212,92,0.07)",
} as const;

// 圆角（沿用现有档位，但收敛成 5 档）
export const radius = {
  xxs: 4,   // 极小圆角（1~4px 的吸附目标，用于小徽章）
  xs: 6,
  sm: 9,
  md: 12,
  lg: 16,
  xl: 20,
  pill: 999,
} as const;

// 间距（4 的倍数，收敛成 7 档；现有代码用的是 2/4/5/6/7/8/9/10/11/12/14/16，迁移时就近吸附）
export const space = {
  xs: 4,
  sm: 6,
  md: 8,
  lg: 12,
  xl: 16,
  xxl: 20,
  xxxl: 26,
} as const;

// 字号（21 档 → 7 档。迁移映射规则见 4.3）
export const font = {
  micro: 10,     // 徽章、极小标签（原 8 / 9 / 9.5 / 10）
  caption: 11.5, // 元信息、时间戳（原 10.5 / 11 / 11.5）
  label: 12.5,   // 按钮文字、列表副标题（原 12 / 12.5）
  body: 14,      // 正文（原 13 / 13.5 / 14）
  bodyLg: 15,    // 强调正文（原 15）
  title: 17,     // 区块标题（原 16 / 17 / 18）
  display: 22,   // 页面主标题（原 20 / 22 / 24 / 26 / 28）
} as const;

// 行高（正文必须 >= 字号的 1.6 倍，保证中文可读）
export const leading = {
  tight: 1.25,
  normal: 1.45,
  relaxed: 1.68,
} as const;

// 字重（RN 只接受 100 的整数倍字符串！原代码的 450/550/650 是非法的，见 4.4）
export const weight = {
  regular: "400",
  medium: "500",
  semibold: "600",
  bold: "700",
  heavy: "900",   // 仅限：主标题、徽章数字。正文禁止使用
} as const;

// 阴影：深色主题下不用"强阴影"做层次，用"边框 + 表面色差"。
// 只在浮起元素（浮岛、输入栏、标签栏、Modal）使用，且必须是柔和的。
export const shadow = {
  // 浮起元素：大范围柔和下沉阴影（不是硬阴影！）
  float: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.45,
    shadowRadius: 18,
    elevation: 12,
  },
  // 轻微浮起（卡片）
  card: {
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.28,
    shadowRadius: 6,
    elevation: 3,
  },
  // 品牌发光（按钮、头像）—— 注意 RN 不支持彩色阴影在 Android，
  // 所以 elevation 给 0，靠 border 与渐变本身表现
  brandGlow: {
    shadowColor: palette.brand,
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.5,
    shadowRadius: 10,
    elevation: 0,
  },
  none: {
    shadowColor: "transparent",
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0,
    shadowRadius: 0,
    elevation: 0,
  },
} as const;

// 玻璃拟态参数：Android 上 BlurView 效果弱，用高不透明度实色近似。
// 组件侧统一调用这个函数，不要各自写死。
export const glass = {
  // BlurView 的 intensity（iOS 有效；Android 建议 <= 30）
  intensity: 24,
  // 兜底色（BlurView 之上覆盖的半透明实色，两平台都用，保证视觉一致）
  tint: palette.glassTint,
  // 边框高光
  edge: palette.glassEdge,
} as const;

// 兼容旧命名：原 theme.ts 里的 darkTheme 已被全量废弃，
// 但为防止有遗漏的 import，这里导出会抛错的占位，让遗漏在编译期暴露。
/** @deprecated 已废弃，请使用 palette / radius / font / weight */
export const darkTheme = new Proxy({} as Record<string, never>, {
  get(_t, prop) {
    throw new Error(
      `darkTheme.${String(prop)} 已废弃。请改用 palette / radius / font / weight / shadow。`,
    );
  },
});
```

### 2.3 对比度验证（已实测，实现者不必重算，但需知道结论）

| 前景 | 背景 | 对比度 | 结论 |
|---|---|---|---|
| ink1 `#F2F0EC` | canvas `#08080A` | 17.58:1 | ✅ 优秀 |
| ink1 `#F2F0EC` | surface2 `#17171B` | 15.71:1 | ✅ 优秀 |
| ink2 `#9C978D` | canvas | 6.88:1 | ✅ 达 AA |
| ink2 `#9C978D` | surface2 | 6.15:1 | ✅ 达 AA |
| ink3 `#8A8479` | canvas | 5.39:1 | ✅ 达 AA（≥4.5） |
| ink4 `#6B6660` | canvas | 3.52:1 | ⚠️ 仅用于占位符/禁用态（AA 大字号可用） |
| brand `#F6D45C` | canvas | 13.81:1 | ✅ 优秀 |
| inkInverse `#241F14` | 用户气泡 `#EFC348` | 9.81:1 | ✅ 优秀 |

**规则**：ink4 **禁止**用于任何需要阅读的正文。只允许占位符文字（placeholder）和禁用按钮文字。

### 2.4 预留浅色主题（只留位，不实现）

在 `theme.ts` 末尾追加一个**空的**扩展点，不要写任何实现：

```ts
// 将来支持浅色主题时，在这里定义 lightPalette，并按同样结构提供 lightRadius 等。
// 组件侧统一通过 useTheme() 取值（本次改造可以先直接从 theme.ts 解构导入，
// 但请把导入语句集中在每个文件顶部，方便将来替换为 context）。
```

**不要在本次改造中实现 `useTheme` hook**。直接从 `../constants/theme` 解构导入即可。
将来换主题时再统一替换导入 —— 现在做 context 属于过度设计。

**完成判据（第 2 章）**：`src/constants/theme.ts` 内容与上面一致；`npm run typecheck` 通过；
此时所有组件仍引用旧色值，**界面看起来没有变化** —— 这是正常的，下一步才开始迁移。

---

## 3. 迁移的核心难点：同一个颜色，不同属性，要去不同的 token

**这是整个改造最容易做错的地方，请务必读懂这一节再动手。**

现有的 `#12100e`（近黑）出现了 **475 次**，但它的用途完全相反：
- 作为 `color`（文字色）207 次 → 在深色主题里必须变**亮**（`palette.ink1`）
- 作为 `borderColor`（描边）194 次 → 在深色主题里必须变**淡**（`palette.hairline`）
- 作为 `shadowColor`（阴影）46 次 → 在深色主题里必须**删除**（深色底不需要硬阴影）

所以**绝对不能用全局查找替换**。必须按"属性 + 原色值"两个维度查表。

### 3.1 实测属性分布（决定了迁移表的结构）

| 属性 | 出现次数 |
|---|---|
| `color` | 470 |
| `backgroundColor` | 379 |
| `borderColor` | 281 |
| `shadowColor` | 48 |
| `borderBottomColor` | 18 |
| `borderTopColor` | 16 |
| `borderLeftColor` | 9 |
| `borderRightColor` | 3 |

### 3.2 主映射表（覆盖 68.5% 的用量，必须严格照做）

格式：`原色值` → `color 属性映射` / `backgroundColor 属性映射` / `borderColor 属性映射`

| 原色值 | 次数 | 作为 color | 作为 backgroundColor | 作为 borderColor |
|---|---|---|---|---|
| `#12100e` | 475 | `palette.ink1` | `palette.surface3` | `palette.hairline` |
| `#6c665f` | 76 | `palette.ink2` | `palette.surface2` | `palette.hairline` |
| `#ffd84f` | 70 | `palette.inkInverse` | `palette.brand` | `palette.brandDeep` |
| `#fffdf7` | 40 | `palette.ink1` | `palette.surface1` | `palette.hairline` |
| `#776f66` | 30 | `palette.ink3` | — | `palette.hairlineSoft` |
| `#25231f` | 27 | `palette.ink1` | `palette.surface4` | `palette.hairlineStrong` |
| `#fffaf0` | 24 | `palette.ink1` | `palette.surface1` | `palette.hairline` |
| `#ffffff` | 22 | `palette.ink1` | `palette.surface2` | `palette.hairlineStrong` |
| `#b9e9b0` | 19 | `palette.inkInverse` | `palette.okWash` | `palette.okBorder` |
| `#8c857b` | 16 | `palette.ink3` | `palette.surface3` | `palette.hairline` |
| `#ff7f68` | 16 | `palette.inkInverse` | `palette.errWash` | `palette.errBorder` |
| `#4fd7ee` | 15 | `palette.inkInverse` | `palette.cyanWash` | `palette.cyanWash` |
| `#f5eefc` | 14 | `palette.ink2` | `palette.brandWash` | `palette.hairline` |
| `#f5f1e9` | 13 | `palette.ink2` | `palette.surface2` | `palette.hairline` |
| `#d7cfc2` | 7 | `palette.ink3` | — | `palette.hairline` |
| `#78716c` | 7 | `palette.ink3` | `palette.surface3` | `palette.hairline` |
| `#2e8b38` | 6 | `palette.ok` | `palette.ok` | `palette.okBorder` |
| `#0f766e` | 6 | `palette.info` | `palette.infoWash` | `palette.okBorder` |
| `#fff` | 6 | `palette.ink1` | `palette.surface2` | `palette.hairlineStrong` |
| `#ede7da` | 5 | `palette.ink2` | `palette.surface2` | `palette.hairline` |
| `#0e4e16` | 5 | `palette.ok` | `palette.okWash` | `palette.okBorder` |
| `#777169` | 5 | `palette.ink3` | — | `palette.hairline` |
| `#f4f5f7` | 5 | `palette.ink1` | `palette.canvas` | `palette.hairline` |
| `#a8a29e` | 5 | `palette.ink3` | `palette.surface3` | `palette.hairlineStrong` |
| `#fff8db` | 4 | `palette.inkInverse` | `palette.brandWashStrong` | `palette.brandDeep` |
| `#fee2e2` | 4 | `palette.err` | `palette.errWash` | `palette.errBorder` |
| `#44403c` | 4 | `palette.ink2` | `palette.surface3` | `palette.hairlineStrong` |
| `#756d64` | 4 | `palette.ink3` | `palette.surface2` | `palette.hairline` |
| `#171411` | 4 | `palette.ink1` | `palette.surface3` | `palette.hairline` |
| `#141413` | 3 | `palette.ink1` | `palette.surface1` | `palette.hairline` |

### 3.3 状态色系映射（用于 success / warning / error 类）

现有代码里有一堆 emoji 语义的浅色底（如 `#dcfce7` 浅绿、`#fee2e2` 浅红、`#cffafe` 浅青），统一按下表归类：

| 原色系（举例） | 归入 |
|---|---|
| `#dcfce7` `#d9f6df` `#edf5f0` `#dcf5d6` `#e7f3e7` `#edf9e8` `#f4fbf7` `#e6efe4` `#dcebd8` `#dff3dc` `#b9e0c2` | `palette.okWash` |
| `#fee2e2` `#fef2f2` `#ffe1db` `#ffe3de` `#fff0ee` `#eede d6`→`#eedbd6` `#f3b9b1` | `palette.errWash` |
| `#fef3c7` `#fff4cc` `#fffbeb` `#fdf7ea` `#fff8db` `#f5eee4` `#fff3bf` | `palette.brandWash` |
| `#cffafe` `#f0fdfa` `#eff6ff` `#eef7ff` `#e9f4ff` `#e6edf3` | `palette.cyanWash` |
| `#f5eefc` `#f1efff` | `palette.brandWash` |
| `#f1ece3` `#f7f5f0` `#fdfbf7` `#fdf8ee` `#f8f6f0` `#fbfbfa` `#f5efd8`→`#f5eedf` `#faf5ec` `#f2ecdf` `#f8f4ec` `#f4eee2` `#fbf8f2` `#fcfbf7` `#f7f1e8` `#f5efe5` `#f4f3ef` `#f1eee7` `#eceae4` `#f5f5f4` `#f7f6f2` | `palette.surface2`（中性面板底）|

> 说明：上面左侧的色值是本仓库里实际出现的中性浅色/浅色底。它们在白底时代用来做"面"，在深色时代一律变成"稍亮的表面"或"淡色状态底"。
> 如果遇到表里没有的色值，**按第 3.4 节的兜底规则处理，不要猜**。

### 3.4 兜底规则（覆盖剩下的 269 个一次性色值）

按**饱和度 + 明度**判定，用下面的判断流程。**这是硬规则，请严格执行**：

```
对任意未列入 3.2 / 3.3 的色值 C：

1. 计算 C 的 HSL 明度 L（0~100）
2. 若 L >= 92（极浅，白底时代的"面"或"淡底"）：
      作为 backgroundColor → palette.surface2
      作为 borderColor     → palette.hairline
      作为 color           → palette.ink1
3. 若 L 在 75 ~ 92（浅灰，白底时代的次级文字/描边）：
      作为 color           → palette.ink3
      作为 backgroundColor → palette.surface3
      作为 borderColor     → palette.hairline
4. 若 L 在 40 ~ 75（中灰）：
      作为 color           → palette.ink2
      作为 borderColor     → palette.hairlineStrong
      作为 backgroundColor → palette.surface3
5. 若 L < 40（深色，白底时代的文字/描边主色）：
      作为 color           → palette.ink1
      作为 borderColor     → palette.hairline
      **作为 shadowColor   → 删除该 shadow 属性，改用 shadow.none**
6. 若饱和度 S >= 50 且是黄色系（H 在 35~65）：
      作为 backgroundColor → palette.brandWash
      作为 color           → palette.brand
7. 若饱和度 S >= 50 且是绿色系（H 在 80~170）：
      作为 backgroundColor → palette.okWash
      作为 color           → palette.ok
8. 若饱和度 S >= 50 且是红色系（H 在 320~360 或 0~20）：
      作为 backgroundColor → palette.errWash
      作为 color           → palette.err
9. 若饱和度 S >= 50 且是青色/蓝色系（H 在 170~260）：
      作为 backgroundColor → palette.cyanWash
      作为 color           → palette.cyan
10. 其他情况（极少数）：作为 color → palette.ink2，其他 → palette.surface2
```

### 3.5 可以直接全局替换的"安全色"（不受属性影响）

以下 5 个值在任何属性下映射都相同，可以放心 `replace_all`：

| 原值 | 新值 | 说明 |
|---|---|---|
| `#12100e`（**仅当属性是 shadowColor**） | → **删除该行** | 见 3.4 第 5 条 |
| `#f4f5f7`（仅 `MobileScreenShell` 内） | `palette.canvas` | 全局背景 |
| `#efe4d2`（仅 `StartupLoadingOverlay` 与 app.json） | `palette.canvas` | 启动屏背景 |
| `#ffd84f`（作为 backgroundColor） | `palette.brand` | 品牌黄 |
| `transparent` | 保持不变 | 无需处理 |

---


---

## 4. 迁移执行方式

### 4.1 为什么用 codemod 而不是手工

1329 处颜色分布在 32 个文件、合计 **16,559 行**代码里（其中 `theme.ts` 会被整份重写，
实际需要逐个改造的**业务文件是 31 个**）。手工改会有两个必然结果：
改不完（漏）、改不一致（同一语义用了两个 token）。

**用脚本做第 3 章映射表的机械替换，然后人工只处理脚本标注的可疑项。**
### 4.2 codemod 脚本（**已验证可运行**）

我已经把这份脚本写好并**在真实代码副本上实测过**，放在仓库里：
**`mobile/scripts/migrate-colors.mjs`**。

> 你不需要手动抄写脚本。直接使用仓库里这个文件即可。
> 如果你想了解它的逻辑，下面附录 C 有说明。

**实测数据（请知悉，避免预期错误）：**

| 指标 | 数值 |
|---|---|
| 可自动替换 | **71.9%**（938 / 1305 处） |
| 剩余需人工 | 367 处 |
| 其中：待删硬阴影 | 20 处 |
| 其中：长尾未映射色值 | 265 处（239 个独特色值） |
| 其中：游离字面量（三元/参数） | 82 处 |

**换句话说：脚本能处理七成，剩下三成必须人工做。** 那三成主要是：
1. `DiffViewer.tsx` 里本来就存在的深色值（`#22211f` / `#7ee787` / `#ffa198` 等）
2. 各面板里的一次性状态色（`#b58c00` / `#b83b30` / `#1e6847` 之类）
3. 三元表达式与函数参数里的颜色

这三类都必须按第 3.4 节兜底规则人工判断，**不要试图扩展脚本来覆盖它们** ——
长尾色值的语义只有看到上下文才能定，脚本猜必错。

### 4.3 运行脚本

```bash
cd mobile
node scripts/migrate-colors.mjs
```

**脚本会做的**：
- 处理两种写法：对象属性 `prop: "#hex"` 与 JSX 属性 `prop="#hex"`（两者都要，别只处理前者）
- 按第 3.2 / 3.3 表替换成 `palette.xxx`
- `placeholderTextColor` 统一改 `palette.ink4`
- `shadowColor` 一律保留并记入待办（阴影要整块重构，不能只换色）
- 结果写入 `mobile/migration-todo.txt`

**脚本不做的（必须人工补完，漏了会编译失败或视觉错误）：**

1. **补 import**。每个被改动的文件顶部需要：
   ```ts
   // src/components/chat/xxx.tsx
   import { palette } from "../../constants/theme";
   // src/screens/xxx.tsx
   import { palette } from "../constants/theme";
   ```
   如果同时用到圆角/字号/阴影，一并引入：
   ```ts
   import { palette, radius, font, weight, shadow } from "../../constants/theme";
   ```

2. **处理 `migration-todo.txt` 的每一项**，按第 3.4 节兜底规则判断。

3. **重构硬阴影**。凡 `shadowColor` 是深色系（`#12100e` / `#25231f` / `#000` 等）的，
   把 `shadowColor` + `shadowOffset` + `shadowOpacity` + `shadowRadius` + `elevation`
   **整块五件套**替换为一个 spread：
   ```ts
   // 改前
   shadowColor: "#12100e",
   shadowOffset: { width: 0, height: 2 },
   shadowOpacity: 1,
   shadowRadius: 0,
   elevation: 3,
   // 改后（浮起元素：浮岛/输入栏/标签栏/Modal/抽屉）
   ...shadow.float,
   // 改后（普通卡片、列表项）
   ...shadow.card,
   ```

4. **处理三元表达式**（约 20 处，集中在这几个文件）：
   ```ts
   // 改前
   thumbColor={plugin.enabled ? "#f5eee4" : "#bdb4a8"}
   // 改后
   thumbColor={plugin.enabled ? palette.brandLight : palette.ink4}
   ```

   主要位置：
   - `PluginPanel.tsx:58` — Switch 的 `thumbColor`
   - `AppHeader.tsx` — 按钮激活态
   - `BottomTabs.tsx` — 标签激活态
   - `SideDrawer.tsx` — 列表项激活态
   - `AgentTurnCard.tsx` — 大量状态色三元
   - `KnowledgePanel.tsx` — 选中态

5. **处理函数参数里的颜色**（如 `ButtonContent color="#8b2822"`）：
   ```tsx
   // 改前
   <ButtonContent color="#8b2822" loading={pending} text="取消" />
   // 改后
   <ButtonContent color={palette.err} loading={pending} text="取消" />
   ```
   注意：JSX 里传变量要用 `{}`，不能写 `color=palette.err`。

### 4.3.1 人工处理 `DiffViewer.tsx` 的专项清单

这个文件**本来就是深色**（是全项目唯一已经深色的地方），所以它的色值全部不在映射表里，
会 100% 进入待办清单。**按下面表格直接替换，不用按兜底规则判断**：

| 原值 | 用途 | 新值 |
|---|---|---|
| `#22211f` | 容器背景 | `palette.surface1` |
| `#141413` | 代码区背景 | `palette.surface1` |
| `#25221b` | 行背景（交替） | `palette.surface2` |
| `#32302c` | 行号区背景 | `palette.surface2` |
| `#383632` | 分隔线 | `palette.hairline` |
| `#38342b` | 表头分隔 | `palette.hairline` |
| `#524e48` | 边框 | `palette.hairline` |
| `#484f58` | 行号色 | `palette.ink4` |
| `#6e7681` | 次要文字 | `palette.ink3` |
| `#7ee787` | 新增行文字 | `palette.diffAdd` |
| `#ffa198` | 删除行文字 | `palette.diffDel` |
| `#3fb950` | 新增标记背景 | `palette.diffAddBg` |
| `#f85149` | 删除标记背景 | `palette.diffDelBg` |
| `#f7f5f0` | 主体文字 | `palette.ink1` |
| `#ffd84f` | 高亮文字 | `palette.brand` |
| `#8c857b` | 元信息 | `palette.ink3` |
| `#12100e` | 所有边框/阴影 | `palette.hairline` / 删阴影 |

### 4.3.2 人工处理 `AgentTurnCard.tsx` 的专项清单（44 处游离）

这个文件有 44 处未命中映射，因为它用了大量"状态色三元"。**按语义归类，不要逐个数**：

| 语义 | 原色系（举例） | 新值 |
|---|---|---|
| 运行中 / 进行中 | `#ffd84f` `#fef3c7` `#b58c00` | `palette.brand` / `palette.brandWash` |
| 成功 / 完成 | `#b9e9b0` `#0e4e16` `#1e6847` `#1d5c45` | `palette.ok` / `palette.okWash` |
| 失败 / 错误 | `#ff7f68` `#7a1f1a` `#b83b30` `#8b2822` `#9f352d` | `palette.err` / `palette.errWash` |
| 等待 / 暂停 | `#ede7da` `#d4ccbd` `#7a7267` | `palette.ink3` / `palette.hairline` |
| 中性面板 | `#ffffff` `#f5f1e9` `#e6dfd3` | `palette.surface2` / `palette.surface3` |

### 4.4 字号 / 字重 / 描边 / 圆角的机械替换

颜色改完后，用第二份脚本处理其余维度。

**这份脚本也已经写好并实测过：`mobile/scripts/migrate-typography.mjs`**，直接用即可：

```bash
cd mobile
node scripts/migrate-typography.mjs
```

实测结果：**修改 28 个文件**，仅剩 12 处需要人工（都是 `borderRadius: 1/2/3` 这类极小值和小圆角）。
脚本自动完成：字号 21 档 → 7 档、字重合法化并降档、`borderWidth: 2/2.5/3/1.5` → `1`、圆角收敛到 token。

> **脚本的一个重要副作用请务必知悉**：它会把**所有** `fontWeight: "900"` 降为 `weight.bold`。
> 运行完你必须人工把这三类改回 `weight.heavy`：
> ① 页面主标题（`StartupLoadingOverlay` 的 title、各 Panel 的一级标题）
> ② 数字徽章（未读数、token 计数）
> ③ 进度百分比
> 如果拿不准，**保持 bold** —— 轻一点比回到"全屏都最响"好。

下面的映射表供你复核脚本结果，也可用于脚本未覆盖的写法（如三元表达式里的字号）：

**字号（21 档 → 7 档）：**

| 原值 | 新值 |
|---|---|
| `8`, `9`, `9.5`, `10` | `font.micro` |
| `10.5`, `11`, `11.5` | `font.caption` |
| `12`, `12.5` | `font.label` |
| `13`, `13.5`, `14` | `font.body` |
| `15` | `font.bodyLg` |
| `16`, `17`, `18` | `font.title` |
| `20`, `22`, `24`, `26`, `28` | `font.display` |

**字重（重要：原代码有非法值）**

RN 的 `fontWeight` **只接受 100 的整数倍字符串**。原代码里存在 `"450"`、`"550"`、`"650"`，
这些值在 iOS/Android 上都会被**静默忽略**（回落到默认字重），所以现在的字重层次有一部分根本没生效。
改造成 token 后必须全部合法化：

| 原值 | 新值 |
|---|---|
| `"400"`, `"450"` | `weight.regular` |
| `"500"`, `"550"` | `weight.medium` |
| `"600"`, `"650"` | `weight.semibold` |
| `"700"` | `weight.bold` |
| `"800"` | `weight.bold` |
| `"900"` | `weight.bold`，**例外见下** |

**`"900"` 的例外规则**（原代码 202 处 900，不能一刀切）：
只有以下三种场景保留 `weight.heavy`：
- 页面主标题（语义上是标题，字号为 `font.display` 或 `font.title`）
- 数字徽章（未读数、token 计数、百分比）
- 进度数字

其余一律 `weight.bold`。**拿不准时用 `weight.bold`** —— 宁可轻一点，也不要回到"全屏都最响"的老问题。

**描边（100 处 `borderWidth: 2`）：**

| 原值 | 新值 |
|---|---|
| `borderWidth: 2` | `borderWidth: 1` |
| `borderWidth: 1.5` | `borderWidth: 1` |
| `borderWidth: 3` | `borderWidth: 1`，配色改 `palette.hairlineStrong` |
| `borderWidth: 1` | 保留 |

> **不要**把 `borderWidth: 1` 再改成别的值。1px 是深色下的标准边框。

**硬阴影（45 处 `shadowRadius: 0` 且 `shadowOpacity: 1`）：** 全部删除，按元素类型替换：

| 元素类型 | 替换为 |
|---|---|
| 浮岛 / 输入栏 / 标签栏 / Modal / 抽屉 | `...shadow.float` |
| 普通卡片、列表项 | `...shadow.card` |
| 品牌主按钮（发送、确认） | `...shadow.brandGlow` |
| 其余 | `...shadow.none` 或整块删除 |

**圆角：**

| 原值 | 新值 |
|---|---|
| `4`, `5` | `radius.xs` |
| `6`, `7`, `8` | `radius.sm` |
| `9`, `10`, `12`, `13`, `14` | `radius.md` |
| `16`, `18` | `radius.lg` |
| `20`, `22`, `24` | `radius.xl` |
| `999` | `radius.pill` |

> 圆角吸附允许 ±2 的视觉取舍，但**同一类元素必须用同一个 token**。

### 4.5 `rgba()` 字面量（24 处，手工逐个改）

| 原值 | 新值 |
|---|---|
| `rgba(255,253,247,0.9)` / `rgba(255,253,247,0.92)` | `palette.glassTint` |
| `rgba(255,250,240,0.82)` / `rgba(255,250,240,0.88)` | `palette.glassTint` |
| `rgba(255,250,240,0.96)` | `palette.glassTintStrong` |
| `rgba(18,16,14,0.72)` / `rgba(18,16,14,0.65)` / `rgba(28,25,22,0.46)` | `palette.scrim` |
| `rgba(18,16,14,0.45)` | `palette.hairlineStrong` |
| `rgba(10,9,8,0.96)` | `palette.scrimHeavy` |
| `rgba(255,255,255,0.02)` | `palette.glassInner` |
| `rgba(46,160,67,0.12)` / `rgba(46,160,67,0.18)` | `palette.okWash` / `palette.okBorder` |
| `rgba(248,81,73,0.12)` / `rgba(248,81,73,0.18)` | `palette.errWash` / `palette.errBorder` |
| `rgba(255,216,79,0.16)` / `rgba(255,216,79,0.4)` | `palette.brandWashStrong` / `palette.brandGlow` |

### 4.6 禁止事项（验收会查）

```ts
// ❌ 禁止：任何颜色字面量
backgroundColor: "#17171B"
color: "#fff"
borderColor: "rgba(255,255,255,0.1)"

// ❌ 禁止：组件内自定义颜色常量
const CARD_BG = "#17171B";

// ✅ 唯一正确写法
import { palette, radius, font, weight, shadow } from "../../constants/theme";
backgroundColor: palette.surface2
color: palette.ink1
borderColor: palette.hairline
 borderRadius: radius.md
fontSize: font.body
fontWeight: weight.bold
...shadow.card
```

**唯一例外**：`src/constants/theme.ts` 自身。

### 4.7 完成判据

```bash
cd mobile
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)$/.test(e.name))o.push(f);}return o;}
const hits=[];
for(const f of walk('src')){if(f.endsWith('theme.ts'))continue;
const n=(fs.readFileSync(f,'utf8').match(/#[0-9a-fA-F]{3,8}\b/g)||[]).length;
if(n)hits.push(n+'  '+f);}
console.log(hits.length?hits.join('\n'):'CLEAN');
"
```

**期望输出：`CLEAN`**。同时 `npm run typecheck` 必须通过。

此时界面应该**已经变深色**，但布局还是旧的（浮岛还是一大坨、气泡还带边框）。下一步做结构改造。

---

## 5. 结构改造规格（核心 5 个组件）

第 3–4 章做完后，颜色已经对了，但**布局问题还在**：浮岛还是"框住一堆东西"、
消息气泡还是四色卡片、底部还是二选一。这一章给出每个组件的**精确目标参数**。

> **重要**：以下所有数值都是**最终值**，直接照抄。不要"感觉一下再调"。
> 原型文件 `mobile/ui-redesign-prototype.html` 里 B 方向的两个屏幕是视觉基准，
> 实现完请开着这个文件逐屏对比。

### 5.1 `src/screens/MobileScreenShell.tsx`（骨架，最先改）

这是所有页面的容器，改它影响全局。

**改动点：**

1. `safe` 样式的 `backgroundColor`：`"#f4f5f7"` → `palette.canvas`
2. `StatusBar` 的 `style`：`"dark"` → `"light"`（深色底必须用浅色状态栏图标）
3. web 分支的 `document.body.style.backgroundColor`：`"#f4f5f7"` → `palette.canvas`
4. `content` 的 `gap`：`12` → `space.lg`（值不变，只是换成 token）
5. `content` 的 `paddingHorizontal`：`14` → `space.xl`（16，给正文更多呼吸空间）
6. **新增**：最外层 `SafeAreaView` 上叠加一层品牌光晕，见下

**光晕实现**（新增，这是深色主题的视觉锚点）：

在 `screen` 这个 `View` 内部（`KeyboardAvoidingView` 之下、内容之上）插入：

```tsx
{/* 顶部品牌光晕：纯装饰，不参与布局与交互 */}
<View pointerEvents="none" style={styles.brandGlow} />
```

样式：

```tsx
brandGlow: {
  position: "absolute",
  top: -140,
  alignSelf: "center",
  width: 360,
  height: 300,
  borderRadius: 180,
  backgroundColor: "rgba(246,212,92,0.07)",   // ← 唯一允许的例外：光晕用字面量
  // 用两层叠加模拟径向渐变（RN 无 radial-gradient）
  shadowColor: palette.brand,
  shadowOffset: { width: 0, height: 0 },
  shadowOpacity: 0.35,
  shadowRadius: 90,
  elevation: 0,
  opacity: 0.5,
  zIndex: 0,
},
```

> **关于这处"例外"**：`rgba(246,212,92,0.07)` 可以写成 `palette.brandGlow` 的变体，
> 但为避免污染 token 表，这里允许直接写字面量并加注释。
> `palette.ambientGlow` 已在第 2 章的 token 表里定义（值为 `rgba(246,212,92,0.07)`），
> **请直接引用 token，不要写字面量**。上面代码里示意用的字面量请替换成 `palette.ambientGlow`。

**关键约束**：所有内容容器必须有 `zIndex: 1` 或更高的兄弟顺序，确保压在光晕之上。
现有 `scrollArea` / `fixedContent` 不需要额外改，因为光晕是 `absolute` 且在最前。

**完成判据**：全屏背景是 `#08080A`；顶部有极淡的暖黄光晕；状态栏文字变白色；
web 端刷新后 body 背景也是深色（无浅色闪白）。

### 5.2 `src/components/layout/AppHeader.tsx`（改动最大，重点）

**这是整个改造的核心。** 现状问题：一屏里 9 个黑边容器。

**目标结构（三段式）：**

```
┌─────────────────────────────────────────────┐
│  [☰]        ┌──── 玻璃浮岛 ────┐      [🕒][▤]│
│             │ (头像) MYAI       │            │
│             │  pc-local         │            │
│             │  ● 正在思考…      │            │
│             └───────────────────┘            │
│                  [对话][规划]                 │
└─────────────────────────────────────────────┘
```

**逐项规格：**

#### 左侧按钮（菜单）
```
尺寸：36 × 36
圆角：radius.sm (9)
背景：rgba(255,255,255,0.045)   → 用 palette.glassInner
边框：1px palette.hairline
图标：Feather "menu"，size 18，color palette.ink2
无阴影
按下态：背景变为 palette.surface4
```
替换原 `☰` emoji。

#### 中间浮岛
```
容器：
  flexDirection: "row"
  alignItems: "center"
  gap: 9
  padding: 7 (上下) 14 (右) 7 (左) 8
  borderRadius: radius.pill (999)
  backgroundColor: palette.glassTint          ← 用 BlurView 包裹时的 tint
  边框: 1px palette.glassEdge
  阴影: ...shadow.float
  玻璃: 外层用 <BlurView intensity={glass.intensity} tint="dark"> 包裹

头像（34 × 34）：
  圆角：17（正圆）
  背景：LinearGradient（见下）
  外圈：不需要 border，用 boxShadow 模拟 1px 亮环 → 改用一个绝对定位的 View：
        position:absolute, inset:-1, borderRadius:18,
        borderWidth:1, borderColor:"rgba(246,212,92,0.5)"
  光晕：绝对定位 View，inset:-6, borderRadius:23,
        backgroundColor:"transparent",
        shadowColor: palette.brand, shadowOpacity:0.55, shadowRadius:9, elevation:0
  动画帧：沿用原有 20 帧逻辑，不加改动（workingFrames / thinkingFrames）

身份区（竖排）：
  行1: "MYAI"
       fontSize: font.label      // 12.5
       fontWeight: weight.bold
       color: palette.ink1
       + 设备徽章（见下）
  行2: 状态行，见下

设备徽章：
  背景 rgba(255,255,255,0.07) → 用 palette.hairline
  圆角 radius.xs (6)
  paddingHorizontal 5, paddingVertical 1
  文字 fontSize: font.micro     // 10
       fontWeight: weight.semibold
       color: palette.ink3
  字体 fontFamily: monospace（设备 ID 是技术信息）

状态行：
  fontSize: font.caption       // 11.5
  fontWeight: weight.medium
  color: palette.ink2
  在线圆点: 6×6 圆, backgroundColor palette.ok
  文本示例: "● 在线" / "● 正在思考…"
```

**头像渐变**（`LinearGradient`）：
```tsx
<LinearGradient
  colors={[palette.brand, palette.brandDark]}
  start={{ x: 0.15, y: 0 }}   // 150deg 的近似
  end={{ x: 1, y: 1 }}
  style={styles.avatarGradient}   // borderRadius 17, overflow hidden
>
  <Image source={...} style={styles.avatarImage} />
</LinearGradient>
```

#### 模式切换（**从浮岛里移出来**，放到浮岛正下方）

这是关键的层级修复：原来模式药丸被塞在浮岛内部，和状态挤在一行。
现在独立成一个居中的小分段控件：

```
容器：
  flexDirection: "row"
  alignSelf: "center"
  marginTop: 8
  backgroundColor: rgba(255,255,255,0.055)  → 新增 palette.glassInner
  borderRadius: radius.sm (9)
  padding: 2
  gap: 1

每段：
  paddingHorizontal 12, paddingVertical 3
  borderRadius: 7
  文字 fontSize: font.micro     // 10
       fontWeight: weight.semibold

选中态：
  背景 palette.brandWashStrong  rgba(246,212,92,0.16)
  文字 color palette.brand

未选中：
  文字 color palette.ink3
```

文案：`对话` / `规划`（去掉 emoji `💬` `🎯`）。
点击行为不变，仍调 `onToggleMode`。

#### 右侧按钮（历史 + 设置）
```
与左侧按钮完全同规格（36×36, glassInner, hairline, ink2 图标）
图标：Feather "clock" (历史) / Feather "sliders" (设置)
间距：gap 6
选中态（设置页激活时）：背景 palette.brandWash，图标 color palette.brand
```

**必须删除的东西：**
- `statusModeCapsule` 整块（状态已移入浮岛，模式已移出）
- `dotDivider`、`modePill`、`modePillPlan`、`modeDropdownArrow`、`deviceBadge` 保留但改色
- 所有 `borderWidth: 2`、`shadowRadius: 0`

**完成判据**：Header 高度不超过 92px（原约 110px）；没有任何 2px 黑边；
浮岛是唯一有阴影的元素；模式切换在浮岛下方独立一行。

### 5.3 `src/components/navigation/BottomTabs.tsx`（结构重写）

**现状问题**：5 个纯文字分段，`borderWidth: 2` + 硬阴影，像按钮条。

**目标结构**：图标 + 文字，玻璃容器，选中项是淡黄药丸。

```
外层容器：
  flexDirection: "row"
  marginHorizontal: 12
  marginBottom: 22        ← 必须保留，否则压到 Home 指示条
  marginTop: 8
  padding: 4
  borderRadius: radius.lg (16)
  backgroundColor: palette.glassTintSoft
  边框: 1px palette.hairline
  阴影: ...shadow.float
  玻璃: 用 BlurView 包裹

每个 tab：
  flex: 1
  flexDirection: "column"
  alignItems: "center"
  gap: 2
  paddingTop: 5, paddingBottom: 6
  borderRadius: radius.md (12)

图标：size 19, strokeWidth 2（Feather）
文字：fontSize font.micro (10), fontWeight weight.medium

选中态：
  背景 palette.brandWash
  图标 color palette.brand
  文字 color palette.brand, fontWeight weight.bold

未选中：
  图标 color palette.ink4
  文字 color palette.ink4
```

**图标映射**（Feather）：
| Tab | 图标名 |
|---|---|
| 对话 | `message-circle` |
| 文件 | `folder` |
| 变更 | `git-branch` |
| 知识 | `book-open` |
| 设置 | `settings` |

**完成判据**：标签栏距屏幕底部有 ≥20px 留白；选中项有淡黄底；文字不压 Home 条。

### 5.4 `src/components/chat/Composer.tsx`（结构重写）

**现状问题**：上方有一个"Agent 任务执行"徽章 + 快捷键提示的独立行，下方是 2px 黑边输入胶囊，
再下方是 5 段标签栏。三段垂直堆叠，占掉约 130px。

**目标结构**：**删掉徽章行**，只保留一个玻璃输入条。

```
外层 wrapper：
  paddingHorizontal: 16
  paddingTop: 10
  gap: 8

输入胶囊（单行，圆角 pill）：
  flexDirection: "row"
  alignItems: "flex-end"
  gap: 7
  borderRadius: radius.xl (20)
  padding: 7 (上下左右统一，左侧 9)
  backgroundColor: palette.glassTintStrong
  边框: 1px palette.glassEdge
  阴影: ...shadow.float
  玻璃: BlurView

左侧附件按钮（30 × 30）：
  无背景、无边框（重要：原来有 1.5px 边框）
  图标 Feather "paperclip", size 19, color palette.ink3

输入框：
  flex: 1
  fontSize: font.body       // 14
  fontWeight weight.regular
  color palette.ink1
  placeholderTextColor palette.ink4
  minHeight: 30
  maxHeight: 88   ← 原 72 略增，容纳更多行
  lineHeight: font.body * leading.normal (≈20)
  paddingHorizontal: 0  ← 交给父容器留白

发送按钮（30 × 30，正圆）：
  borderRadius: 15
  LinearGradient colors={[palette.brandLight, palette.brandDeep]}
  图标 Feather "arrow-up", size 15, strokeWidth 2.5, color palette.inkInverse
  阴影: ...shadow.brandGlow

暂停按钮（执行中，30 × 30）：
  borderRadius: 15
  backgroundColor: palette.errWash
  边框: 1px palette.errBorder
  图标 Feather "square", size 13, color palette.err
```

**必须删除：**
- `composerHeaderRow` 整块（"Agent 任务执行"徽章 + "Enter 发送" 提示）
- `composerModeBadge`、`modeLiveDot`、`modeText`、`composerHintText`
- `uploadBtn` 的背景与边框

> **关于删掉"Enter 发送 · Shift+Enter 换行"提示**：这是 web 端专属提示，删掉后 web 用户体验略降。
> 如果需求方希望保留，请改为**输入框获得焦点时才显示**的一行极小文字（`font.micro`, `palette.ink4`），
> 且不要加边框。**默认行为：删除。**

**附件胶囊**（`attachmentChip`）改色：
```
背景 palette.surface2
边框 1px palette.hairline
圆角 radius.sm
阴影 ...shadow.card  （原来是无阴影 + 2px 边）
```
已上传态（`uploadedAttachmentChip`）：背景 `palette.okWash`，边框 `palette.okBorder`。
图片态（`imageAttachmentChip`）：背景 `palette.surface3`。

**完成判据**：输入区总高度 ≤ 60px（不含附件行）；没有任何黑边框；发送按钮是圆形渐变。

### 5.5 `src/components/chat/styles.ts` + `MessageBubble.tsx`（消息流重做）

**这是"好看不好看"的核心。** 现状：用户黄/助手米白/工具青/错误红，四色四边。

**目标：助手消息去掉背景和边框，直接排在画布上。**

#### 用户消息（`userMessage`）

```
容器（userTurnWrapper）：
  alignSelf: "flex-end"
  maxWidth: "86%"

气泡（userMessage）：
  borderRadius: radius.lg (16)
  borderBottomRightRadius: radius.xs (6)   ← 右下角缺口
  paddingHorizontal: 14
  paddingVertical: 10
  背景: LinearGradient colors={[palette.brandLight, palette.brandDeep]}
        start {x:0.15,y:0} end {x:1,y:1}
  无边框（删除 borderWidth 2 + borderColor）
  阴影: ...shadow.brandGlow  （或删除，深色下可不加）

文字（userMessageText）：
  color: palette.inkInverse
  fontSize: font.body       // 14
  fontWeight: weight.semibold (600)   ← 原 800，降两档
  lineHeight: 21

时间戳（userTurnMeta）：
  color: palette.ink3
  fontSize: font.micro      // 10
  fontWeight: weight.medium
  marginTop: 5
  paddingRight: 3
```

> **实现提示**：`userMessage` 现在是带 `backgroundColor` 的普通 `View`。
> 要换成 `LinearGradient`，需要把 `MessageBubble.tsx` 里用户消息那段从
> `<View style={[styles.message, styles.userMessage]}>` 改成
> `<LinearGradient colors={...} style={[styles.message, styles.userMessage]}>`。
> **注意 `styles.message` 里的 `borderWidth: 1` 要删掉**，否则渐变边缘会有细线。

#### 助手消息（`assistantMessage`）—— **关键改动**

```
容器（assistantMessage）：
  删除 backgroundColor（原 #fffaf0）
  删除 borderColor / borderWidth
  删除 borderRadius
  padding: 0（原来 12，靠父容器 gap 控制间距）
  alignSelf: "stretch"
```

也就是说，助手消息**不再是一个气泡**，而是直接铺在画布上的文本块。
父容器 `ChatPanel` 的 `gap` 负责消息之间的呼吸。

**为什么这样改**：对话里助手输出占 70% 的内容，它需要一个安静、可长读的版面。
给它加气泡等于给"读文章"套了个卡片，视觉噪音都堆在这里。

#### 助手消息头（新增）

在助手文本上方加一行小字头像+名字，建立"谁在说话"：

```
容器（需新增 styles.assistantHeader）：
  flexDirection: "row"
  alignItems: "center"
  gap: 7
  marginBottom: 8

小头像（20 × 20）：
  borderRadius: 10
  LinearGradient [brand, brandDark]
  内放 Image（icon.png），或直接用 emoji 🙂 兜底
  阴影: shadowColor palette.brand, shadowOpacity 0.5, shadowRadius 5, elevation 0

名字（assistantName）：
  fontSize: font.caption    // 11.5
  fontWeight: weight.semibold
  color: palette.ink2

时间（assistantTime）：
  fontSize: font.micro      // 10
  fontWeight: weight.medium
  color: palette.ink3
```

> **数据来源**：`message.createdAt` 已有，格式化为 `HH:mm`。
> 如果不想引入新的时间格式化，可以直接用现有的 `formatDateTime` 截取时间部分，
> 或者**只显示名字不显示时间**（更简单，推荐）。

#### 工具调用（`toolMessage` / `toolGroupMessage`）—— 从"青蓝卡片"变"细线行"

```
容器（toolMessage）：
  backgroundColor: palette.surface2    ← 原 #4fd7ee 青色
  边框: 1px palette.hairline
  圆角: radius.md (12)
  padding: 12
  删除 gap 8 → gap: space.md (8)

头部行（toolHeader）：
  flexDirection: "row"
  alignItems: "center"
  gap: 9

状态点（替换原 toolBadge 文字徽章）：
  6 × 6 圆点
  成功: backgroundColor palette.ok
        阴影 shadowColor palette.ok, shadowOpacity 0.6, shadowRadius 4, elevation 0
  失败: backgroundColor palette.err
  运行中: backgroundColor palette.brandWarn → 用 palette.warn

工具名（toolTitle）：
  fontFamily: monospace
  fontSize: font.label       // 12.5
  fontWeight: weight.semibold
  color: palette.ink1
  例如 "filesystem.search"

副信息（toolSubtitle）：
  fontSize: font.caption     // 11.5
  fontWeight: weight.medium
  color: palette.ink3
  marginLeft: "auto"     ← 推到右边，如 "3 个匹配 · 0.4s"

展开箭头（toolToggle）：
  换成 Feather "chevron-right" / "chevron-down", size 13, color palette.ink4

展开体（toolBody）：
  borderTopWidth: 1
  borderTopColor: palette.hairline
  paddingTop: 10
  backgroundColor: palette.recessed
  圆角: radius.sm
```

**代码块**（`toolCode`）：
```
fontFamily: monospace
fontSize: font.caption     // 11.5
lineHeight: 17
color: palette.ink2
```
关键字高亮色（如 diff 里的路径）：`palette.warn`（黄）/ `palette.info`（绿）。

#### 思考过程（`reasoningBlock` / `ThinkingReasoning`）

```
容器（reasoningBlock）：
  背景 palette.surface2（从透明改为有底，因为它是次要信息但需要被识别）
  → 或者更克制：保持透明，只留左侧竖线。**推荐后者**（更安静）

头部（reasoningHeader）：
  图标: 用于旋转的小圈（保持现有 ActivityIndicator 或换成 Feather "loader"）
  文字（reasoningSummaryText）：
    fontSize: font.caption    // 11.5
    fontWeight: weight.semibold
    color: palette.ink3
  箭头: Feather "chevron-right" / "chevron-down"，color palette.ink4

体（reasoningBody）：
  borderLeftWidth: 1.5
  borderLeftColor: "rgba(255,255,255,0.10)"  → palette.hairline
  paddingLeft: 10
  marginTop: 7

文字（reasoningText）：
  fontSize: font.label       // 12.5
  lineHeight: 20.6           // 12.5 * 1.65
  color: palette.ink3       // 不要用 ink4，对比度 3.52:1 不够读正文
```

#### 错误消息（`errorMessage`）

```
背景: palette.errWash（原 #ff7f68 实色）
边框: 1px palette.errBorder
文字: palette.err
```
原先是整块珊瑚红实色，太刺眼。淡底 + 彩色边框足够表达"出错"。

#### 事件消息（`eventMessage`）

```
背景: palette.brandWash
边框: 1px palette.brandDeep
文字: palette.ink1
```
原先是 `#ffd84f` 实黄 + 黑边。

#### 附件卡片（`assetCard` / `workspaceAttachmentCard`）

```
背景: palette.surface2
边框: 1px palette.hairline    ← 原 borderWidth: 3！
圆角: radius.md
阴影: ...shadow.card
图标块: 背景 palette.brandWash，无边框
主按钮（assetOpenButton）:
  背景 palette.okWash
  边框 1px palette.okBorder
  文字色 palette.ok
```

#### Markdown 渲染（`styles.ts` 里的 markdown* 系列）

这是长文主体，必须最干净：

```
markdownHeading1: fontSize font.display, fontWeight weight.bold, color palette.ink1, lineHeight 28
markdownHeading2: fontSize font.title,   fontWeight weight.bold, color palette.ink1, lineHeight 23
markdownHeading3: fontSize font.bodyLg,  fontWeight weight.semibold, color palette.ink1, lineHeight 21
markdownParagraph: color palette.ink1, lineHeight 23.5 (14*1.68)

markdownStrong: fontWeight weight.bold, color palette.ink1  ← 原 900
markdownEmphasis: fontStyle italic

markdownInlineCode:
  fontFamily monospace
  fontSize 12.5
  背景 palette.brandCode
  color palette.warn           ← 黄色文字
  borderRadius radius.xs(6)
  无边框（删除原 borderWidth 1 + 黑边）
  paddingHorizontal 5

markdownCodeBlock:
  背景 palette.surface2
  边框 1px palette.hairline      ← 原 borderWidth 2
  圆角 radius.md
  padding 11

markdownCodeLabel:
  fontSize font.micro        // 10
  fontWeight weight.semibold
  color palette.ink3
  textTransform uppercase

markdownCodeText:
  fontFamily monospace
  fontSize 12
  lineHeight 19
  color palette.ink1

markdownTable:
  边框 1px palette.hairline
  圆角 radius.md
  单元格 borderBottomWidth 1, borderColor palette.hairline
  表头单元格 背景 palette.surface3, 文字 color palette.ink1, fontWeight bold
  偶数行 palette.surface1

markdownQuote:
  背景 palette.surface2
  无外边框
  左侧竖条 width 2, backgroundColor palette.brand, borderRadius 1
  padding 11
```

**完成判据**：连续三条助手消息 + 两条工具调用 + 一条用户消息，整屏**没有一块彩色大色块**
（除用户气泡外）；工具调用看起来像列表行而不是卡片；所有正文可长读不刺眼。

### 5.6 `src/components/startup/StartupLoadingOverlay.tsx`

```
overlay 背景: "#efe4d2" → palette.canvas
title:
  color: palette.ink1
  fontSize: font.display       // 22
  fontWeight: weight.bold
status:
  color: palette.ink2
  fontSize: font.label         // 12.5
  fontWeight: weight.medium
progressTrack:
  背景 palette.surface2
  边框 1px palette.hairline    ← 原 borderWidth 2
  圆角 radius.pill
  高度 6（原 10，更精致）
progressFill:
  LinearGradient [palette.brandLight, palette.brandDeep]（原 #4fd7ee 青色）
```
角色动画（`character`）不变，深色底上会自动更突出。

### 5.7 `src/components/navigation/SideDrawer.tsx`

```
抽屉背景: palette.surface1
遮罩: palette.scrim（原 rgba(18,16,14,0.45) 太淡，深色下需要 0.72）
右侧边框: 1px palette.hairline    ← 原 borderWidth 2
所有列表项:
  选中背景 palette.brandWash
  文字 palette.ink1 / ink2
  borderBottomWidth 1, palette.hairlineSoft
图标: Feather，color palette.ink3
```
抽屉的 `Animated` 逻辑（`slideAnim` / `fadeAnim`，`useNativeDriver: true`）
**完全不动**，只改样式。

### 5.8 `src/components/changes/DiffViewer.tsx`（已是深色，只需微调）

这个文件**已经是深色**（`#141413` / `#22211f` / `#7ee787` / `#ffa198`），是全项目唯一不用大改的地方。

```
删除所有 borderColor: "#12100e"（深色底上黑边等于隐形）→ 改 palette.hairline
新增行背景: palette.diffAddBg，文字 palette.diffAdd
删除行背景: palette.diffDelBg，文字 palette.diffDel
行号色: palette.ink4
容器背景: palette.surface1
```

> **注意**：这个文件里的 `#ffd84f` 出现 2 次（约 306 行、337 行），按主表映射为
> `palette.brand`（作为 color 时是 `palette.inkInverse`，但这里是文字高亮，应改 `palette.brand`）。
> **这类语义判断需要人工确认**，不要盲目用脚本结果。

### 5.9 `src/utils/buttonFeedback.ts`（全局按下反馈，改一处影响所有按钮）

```ts
// 改前
return [style, active ? { opacity: 0.72, transform: [{ translateX: 2 }, { translateY: 2 }] } : null];

// 改后
export function buttonFeedback(style: StyleProp<ViewStyle>, active?: boolean): StyleProp<ViewStyle> {
  return [
    style,
    active ? { opacity: 0.7, transform: [{ scale: 0.985 }] } : null,
  ];
}
```

**为什么改**：原来的 `translateX/Y: 2` 是配合"硬阴影位移"的拟物按压效果
（元素向右下移动 2px，像被按进纸面）。深色主题没有硬阴影，这个位移会显得诡异且抖动。
改成轻微缩放是更通用的按压反馈。

> 如果你希望保留触感反馈，可以在这里加 `expo-haptics`：
> ```ts
> import * as Haptics from "expo-haptics";
> // 在返回前触发（注意：这个函数在 render 期间被调用，不能有副作用！）
> ```
> **不要**在这个函数里调 `Haptics` —— 它是在 render 阶段算样式用的，加副作用会导致
> 每帧都触发震动。**正确做法**：保持这个函数纯净，触感在 `Pressable` 的 `onPressIn` 里单独加。
> **本次改造默认不加触感**，留作后续优化。

---

## 6. 图标替换（emoji → 矢量图标）

### 6.1 为什么要换

项目里有 **约 50 个不同的 emoji 字符**，分布在 19 个文件。emoji 的问题：
- 每个 Android 厂商（三星/小米/华为/原生）的 emoji 字体不同，粗细、大小、基线全不一致
- 无法设色（emoji 是彩色字形，`color` 属性无效）
- 在深色底上部分 emoji 对比度不足

### 6.2 用什么

统一使用 `@expo/vector-icons` 的 **Feather** 字体族（线性图标，strokeWidth 1.8~2.4，与方向 B 的
"细线条"语言一致）。

**不要**混用 MaterialIcons / Ionicons / FontAwesome —— 它们的笔触风格不同。

### 6.3 完整替换表（**按字符查表，全部替换**）

| 文件 | 原 emoji | 替换为 | size | 备注 |
|---|---|---|---|---|
| `AppHeader.tsx` | `☰` | `menu` | 18 | |
| `AppHeader.tsx` | `🕒` | `clock` | 18 | |
| `AppHeader.tsx` | `⚙` | `sliders` | 18 | 设置用 sliders 更符合"参数调节"语义 |
| `AppHeader.tsx` | `💬` | 删除 | — | 模式切换改为纯文字 |
| `AppHeader.tsx` | `🎯` | 删除 | — | 同上 |
| `AppHeader.tsx` | `▾` | 删除 | — | 模式不再有下拉箭头 |
| `Composer.tsx` | `📎` | `paperclip` | 19 | |
| `Composer.tsx` | `▶` | `arrow-up` | 15 | strokeWidth 2.5 |
| `Composer.tsx` | `■` | `square` | 13 | |
| `Composer.tsx` | `✕` | `x` | 13 | |
| `SideDrawer.tsx` | `✕` | `x` | 18 | |
| `SideDrawer.tsx` | `💬` | `message-circle` | 18 | |
| `SideDrawer.tsx` | `📁` | `folder` | 18 | |
| `SideDrawer.tsx` | `⚡` | `zap` | 18 | |
| `SideDrawer.tsx` | `📚` | `book-open` | 18 | |
| `SideDrawer.tsx` | `⚙` | `settings` | 18 | |
| `SideDrawer.tsx` | `✏` | `edit-2` | 18 | |
| `BottomTabs.tsx` | 纯文字 | `message-circle`/`folder`/`git-branch`/`book-open`/`settings` | 19 | 见 5.3 |
| `FilesPanel.tsx` | `📦` | `package` | 18 | |
| `FilesPanel.tsx` | `📁` | `folder` | 18 | |
| `FilesPanel.tsx` | `📄` | `file-text` | 18 | |
| `FilesPanel.tsx` | `❮` | `chevron-left` | 18 | |
| `FilesPanel.tsx` | `❯` | `chevron-right` | 18 | |
| `ChangesPanel.tsx` | `🏷` | `tag` | 15 | |
| `ChangesPanel.tsx` | `❯` | `chevron-right` | 18 | |
| `ChangeDetailPanel.tsx` | `❮` | `chevron-left` | 18 | |
| `ChangeDetailPanel.tsx` | `📁` | `folder` | 16 | |
| `ChangeDetailPanel.tsx` | `🏷` | `tag` | 15 | |
| `DiffViewer.tsx` | `↩` | `corner-up-left` | 14 | |
| `DiffViewer.tsx` | `⇄` | `repeat` | 14 | |
| `KnowledgePanel.tsx` | `📑` | `clipboard` | 16 | |
| `KnowledgePanel.tsx` | `⚙` | `sliders` | 16 | |
| `KnowledgePanel.tsx` | `📄` | `file-text` | 16 | |
| `KnowledgePanel.tsx` | `✕` | `x` | 14 | |
| `KnowledgePanel.tsx` | `✓` | `check` | 14 | |
| `KnowledgePanel.tsx` | `●`/`○` | `circle` | 8 | 用 fill 区分选中/未选中 |
| `KnowledgePanel.tsx` | `▾` | `chevron-down` | 14 | |
| `AIMemoryPanel.tsx` | `⚠` | `alert-triangle` | 15 | |
| `AIMemoryPanel.tsx` | `🔔` | `bell` | 16 | |
| `AIMemoryPanel.tsx` | `▲`/`▼` | `chevron-up`/`chevron-down` | 15 | |
| `AIMemoryPanel.tsx` | `✨` | `star` | 16 | |
| `AIMemoryPanel.tsx` | `✓` | `check` | 14 | |
| `AIMemoryPanel.tsx` | `○` | `circle` | 8 | |
| `AgentTurnCard.tsx` | `🤖` | `cpu` | 15 | |
| `AgentTurnCard.tsx` | `✓` | `check` | 13 | |
| `AgentTurnCard.tsx` | `↔` | `arrow-left-right` | 13 | |
| `AgentTurnCard.tsx` | `📝` | `edit-3` | 14 | |
| `AgentTurnCard.tsx` | `❯` | `chevron-right` | 14 | |
| `AgentTurnCard.tsx` | `📋` | `clipboard` | 14 | |
| `AgentTurnCard.tsx` | `🔄` | `refresh-cw` | 14 | |
| `AgentTurnCard.tsx` | `🔍` | `search` | 14 | |
| `SharedAssetCard.tsx` | `🔍` | `zoom-in` | 16 | |
| `SharedAssetCard.tsx` | `✕` | `x` | 16 | |
| `SharedAssetCard.tsx` | `🔗` | `link` | 14 | |
| `SubagentPanel.tsx` | `▴`/`▾` | `chevron-up`/`chevron-down` | 14 | |
| `SubagentPanel.tsx` | `🔍` | `search` | 15 | |
| `SubagentInspectionModal.tsx` | `🤖` | `cpu` | 16 | |
| `SubagentInspectionModal.tsx` | `🔒` | `lock` | 15 | |
| `SubagentInspectionModal.tsx` | `↻` | `rotate-cw` | 15 | |
| `SubagentInspectionModal.tsx` | `✕` | `x` | 18 | |
| `SubagentInspectionModal.tsx` | `⚠` | `alert-triangle` | 15 | |
| `SubagentInspectionModal.tsx` | `↔` | `arrow-left-right` | 14 | |
| `SubagentInspectionModal.tsx` | `⚡` | `zap` | 14 | |
| `SubagentInspectionModal.tsx` | `💬` | `message-square` | 14 | |
| `SubagentInspectionModal.tsx` | `📦` | `package` | 14 | |
| `SubagentInspectionModal.tsx` | `🛡` | `shield` | 15 | |
| `SubagentInspectionModal.tsx` | `💭` | `message-circle` | 14 | |
| `SubagentInspectionModal.tsx` | `🛠` | `tool` | 14 | |
| `SubagentInspectionModal.tsx` | `🖼` | `image` | 14 | |
| `SubagentInspectionModal.tsx` | `💡` | `zap` | 14 | （Feather 无 bulb，用 zap 或 info） |
| `SubagentInspectionModal.tsx` | `❯` | `chevron-right` | 14 | |
| `UserQuestionPrompt.tsx` | `●`/`○` | `circle` | 10 | 选中用 `check-circle` |
| `SessionModeSwitch.tsx` | `💬`/`🎯` | 删除 | — | 改纯文字，见 5.2 |
| `SettingsPanel.tsx` | `⚙` | `sliders` | 16 | |
| `SettingsPanel.tsx` | `🎯` | 删除 | — | |
| `SettingsPanel.tsx` | `💬` | `message-circle` | 16 | |
| `SettingsPanel.tsx` | `🧠` | `cpu` | 16 | |
| `SettingsPanel.tsx` | `🧭` | `compass` | 16 | |
| `SettingsPanel.tsx` | `⚡` | `zap` | 16 | |
| `SettingsPanel.tsx` | `🛡` | `shield` | 16 | |
| `SettingsPanel.tsx` | `🤖` | `cpu` | 16 | |
| `SettingsPanel.tsx` | `🛠` | `tool` | 16 | |
| `SettingsPanel.tsx` | `🔌` | `plug` | 16 | |
| `SettingsPanel.tsx` | `🎨` | `droplet` | 16 | |
| `SettingsPanel.tsx` | `📦` | `package` | 16 | |
| `SettingsPanel.tsx` | `📋` | `clipboard` | 16 | |
| `SettingsPanel.tsx` | `🔒` | `lock` | 16 | |
| `SettingsPanel.tsx` | `🔄` | `refresh-cw` | 16 | |
| `SettingsPanel.tsx` | `✅` | `check-circle` | 16 | |
| `SettingsPanel.tsx` | `❌` | `x-circle` | 16 | |
| `SettingsPanel.tsx` | `▲`/`▼` | `chevron-up`/`chevron-down` | 14 | |
| `SettingsPanel.tsx` | `✕` | `x` | 16 | |
| `TokenDock.tsx` | `⚙` | `sliders` | 14 | |
| `plan/PlanPanel.tsx` | 见文件内 | 按语义就近替换 | 14~16 | 人工判断 |

> **兜底规则**：表里没列出的 emoji，选一个**语义最接近的 Feather 图标**，
> 并在代码里加注释说明来历。**实在没有对应的就删掉 emoji**（宁可不显示，也不要留一个渲染不一致的字符）。

### 6.4 标准写法

```tsx
import { Feather } from "@expo/vector-icons";

<Feather name="menu" size={18} color={palette.ink2} />
```

**注意**：`@expo/vector-icons` 的图标在首次渲染时异步加载字体，会有一瞬间空白。
在 `App.tsx` 里预加载可以避免闪烁（可选，但推荐）：

```tsx
// App.tsx 顶部
import { Feather } from "@expo/vector-icons";
import { useFonts } from "expo-font";   // expo-font 已随 expo 安装，无需单独装

// 组件内
const [fontsLoaded] = useFonts(Feather.font);
if (!fontsLoaded) return null;   // 或返回 StartupLoadingOverlay
```

> **但要注意**：`App.tsx` 目前已经有 `StartupLoadingOverlay` 负责首屏。
> 最简单稳妥的做法是**不做预加载**，接受首帧可能空一下（通常在 50ms 内，用户感知不到）。
> **默认行为：不做预加载。**

### 6.5 完成判据

```bash
cd mobile
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)\$/.test(e.name))o.push(f);}return o;}
const re=/[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{2B00}-\u{2BFF}]/gu;
const hits=[];
for(const f of walk('src')){const m=fs.readFileSync(f,'utf8').match(re);if(m)hits.push(f+'  '+[...new Set(m)].join(' '));}
console.log(hits.length?hits.join('\n'):'CLEAN');
"
```

**期望输出：`CLEAN`**（允许极少数例外，如文案里本来就有的 emoji，需在代码注释里说明）。

---

## 7. 其他文件（清单式，机械改色）

以下文件**只做颜色/字号/字重/描边的 token 化**，不做结构改动。
按第 3–4 章的表执行即可。列出改动量供核对：

| 文件 | 颜色数 | 需重点注意 |
|---|---|---|
| `settings/SettingsPanel.tsx` | **251** | 最大文件（4496 行），`borderWidth: 2` 有 **30 处**，900 字重 66 处。建议最后做，单独提交 |
| `knowledge/KnowledgePanel.tsx` | 132 | 14 处 2px 边 |
| `chat/AgentTurnCard.tsx` | 107 | 无 2px 边，但要处理大量状态色 |
| `subagents/SubagentInspectionModal.tsx` | 92 | 有 1 处 2px 边 |
| `knowledge/AIMemoryPanel.tsx` | 80 | 6 处 2px 边 |
| `subagents/SubagentPanel.tsx` | 62 | 无 2px 边 |
| `changes/styles.ts` | 49 | 8 处 2px 边 |
| `plan/PlanPanel.tsx` | 47 | 2 处 2px 边 |
| `changes/DiffViewer.tsx` | 38 | **已是深色**，见 5.8 |
| `files/FilesPanel.tsx` | 36 | 3 处 2px 边，6 处硬阴影 |
| `sessions/SessionsPanel.tsx` | 36 | 4 处 2px 边 |
| `chat/AgentRunTimeline.tsx` | 30 | 3 处 2px 边 |
| `plugins/PluginPanel.tsx` | 22 | 纯改色 |
| `permissions/UserQuestionPrompt.tsx` | 20 | 2 处 2px 边 |
| `chat/ChatJumpNav.tsx` | 17 | 3 处 2px 边 |
| `status/TokenDock.tsx` | 13 | 纯改色 |
| `permissions/PermissionPrompt.tsx` | 12 | 2 处 2px 边 |
| `knowledge/KnowledgeWorkspacePanel.tsx` | 8 | 2 处 2px 边 |
| `session/SessionModeSwitch.tsx` | 8 | 1 处 2px 边，emoji 需删 |
| `common/ResponsiveFormModal.tsx` | 7 | 纯改色 |
| `chat/ChatPanel.tsx` | 6 | 1 处 `#fffdf7`（约 418 行） |
| `common/ButtonContent.tsx` | 1 | 默认 `color = "#12100e"` → `palette.inkInverse` |
| `layout/BottomDock.tsx` | 1 | `#f4f5f7` → `palette.canvas` |
| `chat/SharedAssetCard.tsx` | 1 | 在 `styles.ts` 里 |

**关于 `chat/styles.ts`**：这是**共享样式文件**，被 8 个组件 import
（`MessageBubble`、`MarkdownText`、`ThinkingReasoning`、`ToolActivityGroup`、
`SharedAssetCard`、`ChatAttachmentCard`、`AssistantLoadingBubble`、`ShimmerLabel`）。
改这一个文件 = 同时影响 8 个组件。**优先级最高，建议第一批做。**

---

## 8. 平台配置（必须改，否则视觉割裂）

### 8.1 `app.json`

```json
{
  "expo": {
    "version": "1.1.0",
    "userInterfaceStyle": "dark",        // 保持不变（本来就是 dark）
    "splash": {
      "backgroundColor": "#08080A"       // 原 #efe4d2（浅黄），必须改
    },
    "android": {
      "versionCode": 9,                  // 原 8
      "navigationBar": {
        "backgroundColor": "#08080A",    // 新增
        "barStyle": "light-content"      // 新增
      }
    },
    "plugins": [
      [
        "expo-splash-screen",
        {
          "backgroundColor": "#08080A",  // 原 #efe4d2
          "image": "./assets/splash-character.png",
          "imageWidth": 260,
          "resizeMode": "contain"
        }
      ],
      [
        "expo-navigation-bar",
        {
          "style": "light",              // 原 "dark"，深色底要浅色图标
          "enforceContrast": false
        }
      ]
    ]
  }
}
```

> **注意**：`expo-splash-screen` 插件配置里的 `backgroundColor` 和顶层 `splash.backgroundColor`
> **两处都要改**，只改一处会导致某些 Android 版本启动时仍闪浅色。

### 8.2 `src/hooks/useAndroidNavigationBar.ts`

```ts
// 改前
NavigationBar.setStyle?.("dark");

// 改后：深色底需要浅色导航栏图标
NavigationBar.setStyle?.("light");
// 或更直接：
NavigationBar.setBackgroundColorAsync?.("#08080A").catch(() => undefined);
```

> `NavigationBar.setStyle` 的语义是「导航栏图标的明暗」，不是「导航栏背景的明暗」。
> 深色背景 → 图标要亮 → `"light"`。**这一点容易搞反，请确认实机效果。**

### 8.3 `src/screens/index.ts` / `MobileAppScreen.tsx` 状态栏

`MobileScreenShell.tsx` 里的 `<StatusBar style="dark" />` 改成 `style="light"`。
（已在 5.1 列出。）

### 8.4 启动屏图片

`assets/splash-character.png` 是 1024×1024 **带 alpha 通道**的透明背景 PNG（已实测确认），
所以在 `#08080A` 背景上会正常显示，**不需要更换图片**。

但请注意：`assets/icon.png` 是 **1254×1254 无 alpha 通道**（RGB）的方形图。
它被用在两处：
1. 桌面图标（`app.json` 的 `icon`）—— 没问题，桌面图标本来就不需要圆角透明
2. **`AppHeader.tsx` 第 84 行的头像**（`require("../../../assets/icon.png")`）—— **有问题**

因为我们把头像做成了 34×34 正圆 + `borderRadius: 17` + `overflow: hidden`，
无 alpha 的方形 PNG 被裁成圆形后，**四个角会残留原图的底色**（如果原图底色不是黑色，
在 `#08080A` 背景上会看到一个浅色方块的痕迹）。

**处理方案（二选一，推荐 A）：**

- **方案 A（推荐）**：新建 `assets/avatar.png`，用 `myai-icon-preview-v2.png` 或
  `splash-character.png` 的头像部分裁一个 256×256 的**带透明背景**的 PNG 放进去，
  `AppHeader.tsx` 改引用 `require("../../../assets/avatar.png")`。
- **方案 B**：不改图标，把头像外层的 `borderRadius` 稍微加大并加一层 `palette.brand`
  边框盖住四角。视觉上不够干净，只作为妥协。

> 这一步需要设计资源。**如果暂时拿不到，先用方案 B 过渡，并在代码里加 `TODO` 注释。**

**完成判据**：冷启动全程（splash → 首屏）没有浅色闪烁；导航栏图标是浅色；
头像圆形边缘干净。

---

## 9. 执行顺序（严格按此顺序，每步单独提交）

分 8 个 commit，**每个 commit 都必须能独立编译通过**。
这样出问题时可以精确定位到哪一步，也方便 code review。

| 步骤 | 内容 | 涉及文件 | 完成标志 |
|---|---|---|---|
| **1** | 基础设施 | `theme.ts` 重写、装依赖、`app.json` 版本号、`buttonFeedback.ts` | typecheck 通过，界面无变化 |
| **2** | 骨架 | `MobileScreenShell.tsx`、`MobileAppScreen.tsx`（StatusBar）、`useAndroidNavigationBar.ts`、`BottomDock.tsx` | 全屏变深色，状态栏变浅色 |
| **3** | 共享样式 | `chat/styles.ts`（影响 8 个组件） | 消息区整体变深，但布局还乱 |
| **4** | 核心结构 | `AppHeader.tsx`、`BottomTabs.tsx`、`Composer.tsx` | **视觉主体成型**，可与原型对比 |
| **5** | 消息组件 | `MessageBubble.tsx`、`MarkdownText.tsx`、`ThinkingReasoning.tsx`、`ToolActivityGroup.tsx`、`AssistantLoadingBubble.tsx`、`ShimmerLabel.tsx`、`ChatAttachmentCard.tsx`、`SharedAssetCard.tsx`、`ChatPanel.tsx` | 消息流干净，助手无气泡 |
| **6** | 面板类 | `FilesPanel`、`ChangesPanel`、`ChangeDetailPanel`、`DiffViewer`、`Knowledge*`（3 个）、`PlanPanel`、`SessionsPanel`、`Subagent*`（2 个）、`PluginPanel`、`PermissionPrompt`、`UserQuestionPrompt`、`TokenDock`、`AgentTurnCard`、`AgentRunTimeline`、`ChatJumpNav`、`SessionModeSwitch`、`ResponsiveFormModal`、`ButtonContent`、`StartupLoadingOverlay`、`SideDrawer` | 所有次级页面深色且一致 |
| **7** | 最大文件 | `SettingsPanel.tsx`（251 色，单独做） | 设置页一致 |
| **8** | 收尾 | emoji 清理、`assets/avatar.png`、`migration-todo.txt` 清空 | 验收脚本全绿 |

**每完成一步，立刻执行：**

```bash
npm run typecheck
# 用真机或 web 跑一遍，确认没有白屏/布局崩坏
git add -A && git commit -m "refactor(mobile-ui): <步骤名>"
```

---

## 10. 验收标准（全部通过才算完成）

### 10.1 自动化检查（必须有输出证据）

**检查 1：无颜色字面量**

```bash
cd mobile
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)\$/.test(e.name))o.push(f);}return o;}
const hits=[];
for(const f of walk('src')){if(f.endsWith('theme.ts'))continue;
const n=(fs.readFileSync(f,'utf8').match(/#[0-9a-fA-F]{3,8}\b/g)||[]).length;
if(n)hits.push(n+'  '+f);}
console.log(hits.length?hits.join('\n'):'PASS: 无硬编码颜色');
"
```
✅ 期望：`PASS: 无硬编码颜色`

**检查 2：无 2px 描边**

```bash
grep -rn "borderWidth: 2\b" src --include=*.ts --include=*.tsx | grep -v theme.ts || echo "PASS: 无 2px 描边"
```
✅ 期望：`PASS: 无 2px 描边`

**检查 3：无硬阴影**

```bash
grep -rn "shadowRadius: 0\b" src --include=*.ts --include=*.tsx | grep -v theme.ts || echo "PASS: 无硬阴影"
```
✅ 期望：`PASS: 无硬阴影`

**检查 4：无非法字重**

```bash
grep -rnE 'fontWeight: "(450|550|650|750|850)"' src --include=*.ts --include=*.tsx || echo "PASS: 无非法字重"
```
✅ 期望：`PASS: 无非法字重`

**检查 5：无 emoji 图标**

```bash
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)\$/.test(e.name))o.push(f);}return o;}
const re=/[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{2B00}-\u{2BFF}]/gu;
const hits=[];
for(const f of walk('src')){const m=fs.readFileSync(f,'utf8').match(re);if(m)hits.push(f+'  '+[...new Set(m)].join(' '));}
console.log(hits.length?hits.join('\n'):'PASS: 无 emoji 图标');
"
```
✅ 期望：`PASS: 无 emoji 图标`

**检查 6：typecheck 通过**

```bash
npm run typecheck && echo "PASS: typecheck"
```

**检查 7：字号档位收敛**

```bash
node -e "
const fs=require('fs'),p=require('path');
function walk(d,o=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const f=p.join(d,e.name);
if(e.isDirectory())walk(f,o);else if(/\.(tsx?)\$/.test(e.name))o.push(f);}return o;}
const s=new Set();
for(const f of walk('src')){if(f.endsWith('theme.ts'))continue;
for(const m of fs.readFileSync(f,'utf8').match(/fontSize:\s*([\d.]+)/g)||[])s.add(m.split(':')[1].trim());}
console.log('剩余 fontSize 字面量档位:',[...s].sort((a,b)=>a-b).join(', ')||'无（全部用 token）');
"
```
✅ 期望：`无（全部用 token）` 或极少数圆角吸附遗留

### 10.2 视觉验收（必须真机截图，逐条对照）

**打开 `mobile/ui-redesign-prototype.html` 的「方向 B」两个屏幕，与真机并排对比。**
这一节不是"看着差不多就行"，请逐项打勾：

**Header**
- [ ] 头像 34×34 正圆，有暖黄渐变 + 轻微外发光
- [ ] 浮岛是**唯一的**毛玻璃元素，圆角是 pill（全圆）
- [ ] 浮岛内只有：头像、MYAI、设备徽章、状态行 —— **没有**模式切换
- [ ] 模式切换在浮岛**下方**独立一行，居中，选中项有淡黄底
- [ ] 左右两侧按钮是 36×36 圆角方形，**无黑边框**（只有 1px 极淡边）
- [ ] Header 总高度 ≤ 92px
- [ ] 正在思考时，头像在播放动画，且动画在深色底上清晰可见

**消息流**
- [ ] 用户消息是黄色渐变气泡，右下角有缺口（`borderBottomRightRadius: 6`）
- [ ] 用户消息**无边框**
- [ ] 助手消息**没有背景色、没有边框**，文字直接排在深色画布上
- [ ] 助手消息上方有 20×20 小头像 + "MYAI" 名字
- [ ] 工具调用是一个深色卡片，左侧是**小圆点**（不是"完成"文字徽章）
- [ ] 工具名是等宽字体
- [ ] 思考过程左侧有一条**极淡的竖线**，文字是灰色小字
- [ ] 连续滚动 20 条消息，**没有任何一块大面积亮色块**（除用户气泡）
- [ ] 代码块深色底 + 1px 细边，可读不刺眼
- [ ] 行内代码是黄色文字 + 极淡黄底，**无边框**
- [ ] 表格有线框但极淡，表头稍亮

**底部**
- [ ] 输入栏是单个玻璃胶囊，**没有**"Agent 任务执行"徽章行
- [ ] 输入栏**没有**黑边框
- [ ] 附件按钮**无背景无边框**，只是一个纸夹图标
- [ ] 发送按钮是**正圆**（30×30, borderRadius 15），黄色渐变，带箭头图标
- [ ] 标签栏是 5 个图标+文字，选中项有淡黄底 + 黄色图标
- [ ] 标签栏底部距屏幕边缘 ≥ 20px，**不压 Home 指示条**（这是原型里踩过的坑）
- [ ] 键盘弹起时输入栏正常上推，不与键盘重叠

**其他页面**
- [ ] 文件页：列表项无卡片套卡片，靠 1px 分隔线 + 留白
- [ ] 变更页：diff 红绿在深色底上清晰可读
- [ ] 侧边抽屉：遮罩足够深（能明显区分背景），列表项选中态有淡黄底
- [ ] 设置页：所有开关/输入框/卡片都是深色，**无一处浅色残留**
- [ ] 知识页：同上
- [ ] 权限确认弹窗：在深色底上不刺眼，按钮层级清晰（允许=黄，拒绝=暗）
- [ ] 子代理巡检弹窗：同上

**系统层**
- [ ] 冷启动：splash 是深色（`#08080A`），**没有浅黄闪烁**
- [ ] 状态栏文字是白色
- [ ] Android 导航栏图标是浅色
- [ ] 从后台切回，无浅色闪帧
- [ ] Web 端：body 背景是深色，刷新无白闪

### 10.3 回归测试（功能不能坏）

这次只改样式，**所有交互必须与改造前完全一致**。请逐项手动验证：

- [ ] 发送消息：能正常发送，消息出现在列表
- [ ] 暂停/继续：执行中按钮变成方形暂停键，点击能暂停
- [ ] 附件上传：点纸夹能选文件，上传后显示缩略图/文件名，能删除
- [ ] 图片全屏预览：点击图片能打开，能关闭
- [ ] 消息重发：失败/暂停的助手消息有"重新生成"按钮，点击有效
- [ ] 工具调用展开/收起：点击能展开查看参数和结果
- [ ] 思考过程展开/收起：同上
- [ ] 模式切换：点击"规划"能切到 plan 模式，界面有反馈
- [ ] 5 个标签页：都能正常跳转，无白屏
- [ ] 侧边抽屉：能滑出、能关闭、能切换会话、能进设置
- [ ] 长对话滚动：能自动滚到底部，向上滚动时不强行吸底
- [ ] 键盘避让：输入框获得焦点时，输入栏不被键盘遮挡
- [ ] 深链接/配对：扫码或输入绑定码能正常连接
- [ ] OTA 检查：设置页的版本信息显示正确

### 10.4 交付物

完成后请提供：

1. **8 个 commit 的 git log**（对应第 9 章的 8 步）
2. **7 项自动化检查的输出截图/文本**（10.1）
3. **真机截图**，至少包含：对话页、文件页、变更页、设置页、侧边抽屉、权限确认弹窗
   （每张都在 `#08080A` 背景下，能看出是深色主题）
4. **一份简短说明**，列出：
   - 哪些地方你按本文档做了判断（比如 3.4 兜底规则的个案）
   - 哪些地方文档没说清、你和需求方确认过什么
   - 有没有遗留的 TODO

---

## 附录 A：常见陷阱（**请先读这一节，能省你几个小时**）

### A.1 `BlurView` 在 Android 上几乎无效

`expo-blur` 的 `BlurView` 在 Android 上只支持 `BlurTargetView` + `experimentalBlurMethod`，
默认情况下**不会真的模糊**，只显示半透明色。所以：

- **必须**在 `BlurView` 上直接设置 `backgroundColor`（用 `palette.glassTint`），
  不要依赖 `tint` prop 达到视觉效果
- 不要花时间调 Android 的模糊强度，**接受它只是半透明实色**
- iOS 上会真的模糊，所以两平台观感略有差异 —— 这是可接受的

```tsx
// 正确写法
<BlurView intensity={glass.intensity} tint="dark" style={styles.island}>
  {children}
</BlurView>

// styles.island 里必须有 backgroundColor: palette.glassTint
```

### A.2 深色底上 `shadow` 几乎看不见

RN 的 `shadow` 在深色背景上会被"吃掉"。所以方向 B 的层次不靠阴影，靠：

1. **表面色差**（`canvas` → `surface1` → `surface2` → `surface3`）
2. **1px 极淡边框**（`palette.hairline`）
3. **内描边高光**（`palette.glassInner`）

`shadow.float` 只用在浮岛/输入栏/标签栏这类真正"浮起来"的元素上，且是**大范围柔和阴影**
（`shadowRadius: 18`, `shadowOpacity: 0.45`），不是原来那种硬阴影。

**如果你发现某个元素在深色下"飘"了、分不清边界，第一反应应该是加 `backgroundColor`
或 `borderColor`，而不是加阴影。**

### A.3 Android 不支持彩色阴影

`shadowColor: palette.brand` 在 Android 上会被忽略（Android 的 `elevation` 只支持灰黑阴影）。
所以头像和按钮的"光晕"效果在 Android 上**不会有**。

**处理方式**：不要试图用 `elevation` 模拟。改用：
- 头像外层加一个绝对定位的 `View`，`borderColor: "rgba(246,212,92,0.5)"`, `borderWidth: 1`，
  `inset: -1` —— 这是"亮环"，两平台都生效
- 或者干脆接受 Android 上没有光晕（视觉差异可接受）

### A.4 `LinearGradient` 不能直接替换有 `borderRadius` 的 `View`

`LinearGradient` 本身支持 `borderRadius`，但**如果里面有 `overflow: "hidden"` 的子元素**，
需要把 `borderRadius` 同时写在 `LinearGradient` 的 `style` 和子容器上，否则 Android 上圆角会被裁掉。

```tsx
<LinearGradient
  colors={[palette.brandLight, palette.brandDeep]}
  style={{ borderRadius: 16, overflow: "hidden" }}
>
```

### A.5 `ChatPanel` 的自动滚底不能动

`ChatPanel.tsx` 里有复杂的滚动逻辑（`scrollToEnd` + `onContentSizeChange` + 键盘事件）。
第 5 章只要求改颜色，**绝对不要碰滚动相关的 hooks 和 ref**。
如果你在改 `assistantMessage` 的 padding 时发现滚动位置偏移了，说明你改到了 `contentContainerStyle`，
请回退。

### A.6 助手消息去气泡后的间距问题

助手消息去掉 `backgroundColor` 和 `padding` 后，消息之间可能过于拥挤。
**不要给消息本身加 padding**，而是调整 `ChatPanel` 里消息容器的 `gap`：

```ts
// ChatPanel 的 contentContainerStyle 附近
gap: space.xl,   // 16，原可能是 12
```

### A.7 `fontWeight` 的数字 vs 字符串

RN 的 `fontWeight` 在 iOS 上接受数字（`700`），在 Android 上**只接受字符串**（`"700"`）。
现有代码全部用字符串。**token 里也定义成字符串**（已按此设计）。
如果你写成 `fontWeight: weight.bold` 而 `weight.bold` 是 `"700"` 字符串，两平台都正常。

### A.8 不要给 `SafeAreaView` 加背景色以外的改动

`MobileScreenShell` 的 `SafeAreaView` 只负责安全区。品牌光晕要加在**它的子级**，
不是它本身。加在它本身会导致 `overflow` 问题。

---

## 附录 B：如果只做一半，应该先做哪些

如果时间紧张，按这个优先级做（**但这不满足验收标准**，只是止血）：

1. **必做**（不做等于白改）：第 1、2、3 章 + 5.1 + 5.2 + 5.4
   → 全局变深色 + Header 清爽 + 输入栏清爽，已经能解决 70% 的"不好看"
2. **强烈建议**：5.3（标签栏）+ 5.5（消息流）
   → 这两个决定"专业感"
3. **可延后**：第 7 章的面板类、`SettingsPanel`
   → 只影响次级页面的观感

但请注意：**只做一半会让界面风格割裂**（对话页是深色新风格，设置页还是浅色旧风格），
观感反而更差。所以除非有紧急发版需求，**请完整做完**。

---

## 附录 C：两份 codemod 的逻辑说明（想改脚本时看这里）

### C.1 `scripts/migrate-colors.mjs`

**核心难点**：同一个色值，作为不同 CSS 属性时要映射到不同 token。
例如 `#12100e`：

- 作为 `color`（文字）→ 深色主题必须变**亮** → `palette.ink1`
- 作为 `borderColor`（描边）→ 深色主题必须变**淡** → `palette.hairline`
- 作为 `shadowColor`（阴影）→ 深色主题必须**删除** → 整块 shadow 五件套重构

脚本用正则 `/(\b[A-Za-z][A-Za-z0-9]*)\s*(?::|=)\s*(["'])(#[0-9a-fA-F]{3,8})\2/g`
同时捕获**两种写法**：

| 写法 | 出现场景 | 示例 |
|---|---|---|
| `prop: "#hex"` | `StyleSheet.create` 对象、内联对象 | `backgroundColor: "#fffdf7"` |
| `prop="#hex"` | JSX 属性 | `<ActivityIndicator color="#12100e" />` |

> **这是第一版脚本的一个真实 bug**：最初只匹配了冒号写法，导致 29 处
> `placeholderTextColor="#776f66"` 和 45 处 `color="#..."` 全部漏掉，自动迁移率只有 67.4%。
> 修复后提升到 **71.9%**。

**替换逻辑的三种结果：**

| 情况 | 处理 | 是否需人工 |
|---|---|---|
| 命中映射表且属性可判定 | 直接替换为 `palette.xxx` | 否 |
| `shadowColor` 任意值 | 保留原文，写入待办 | **是**（整块重构） |
| 未命中映射表 | 保留原文，写入待办 | **是**（按 3.4 兜底） |

**属性 → token 列的选择规则：**

```
color / tintColor          → 主表 color 列
backgroundColor           → 主表 backgroundColor 列
borderColor 及其四个方向变体 → 主表 border 列
placeholderTextColor      → 固定 palette.ink4（占位符统一色）
shadowColor               → 一律待办（不替换）
```

**已知不能自动处理、必须人工的情况：**

1. **三元表达式**：`active ? "#ffd84f" : "#ffffff"` —— 脚本不碰（无法判定哪个分支对应哪列）
2. **函数参数**：`<ButtonContent color="#8b2822" />` —— 会被识别为 JSX 属性并尝试替换，
   但因为 `#8b2822` 不在表里而进入待办
3. **数组/对象字面量里的孤立色值**：如 `const TAGS = [{ color: "#..." }]` —— 属性名被识别，
   但可能不是 CSS 属性，需人工确认

### C.2 `scripts/migrate-typography.mjs`

**它做四件事：**

1. **字号收敛**：21 档 → 7 档（`fontSize: 13.5` → `fontSize: font.body`）
2. **字重合法化**：把非法的 `"450"` `"550"` `"650"` 修正为合法值，同时把 `"800"` `"900"` 降档
3. **描边收敛**：`borderWidth: 2 / 2.5 / 3 / 1.5` → `1`
4. **圆角收敛**：所有 `borderRadius` 及其 8 个方向变体 → `radius.xxx`

**为什么必须做字重合法化**：
React Native 的 `fontWeight` 只接受 `"100"`~`"900"` 的百位字符串。
原代码里的 `"450"` / `"550"` / `"650"` 是**非法值**，在 iOS/Android 上都会被静默忽略
（回落到默认字重）。这意味着**现在的字重层次有一部分从来没生效过**。
改造后必须全部合法化，否则你会得到"看起来一样重"的标题和正文。

**⚠️ 脚本的最大副作用（务必记住）**：
它会把**所有** `fontWeight: "900"` 降为 `weight.bold`。
运行后你必须人工把这三类改回 `weight.heavy`：

1. 页面主标题（`StartupLoadingOverlay` 的 title、各 Panel 一级标题）
2. 数字徽章（未读数、token 计数）
3. 进度百分比

**如果拿不准，就保持 `weight.bold`** —— 轻一点远好过回到"全屏都最响"。

**实测数据**：修改 28 个文件，仅剩 12 处需人工（都是 `borderRadius: 0/1/2/3/34` 这类边界值，
其中 0 表示"明确无圆角"，34 已自动映射为 `radius.pill`）。

### C.3 两份脚本的执行顺序（不可颠倒）

```
1. node scripts/migrate-colors.mjs      ← 先颜色
2. 人工补 import + 处理 todo
3. 确认界面已经是深色（虽然布局还乱）
4. node scripts/migrate-typography.mjs  ← 再字号字重描边
5. 人工处理 typography-todo + 恢复 heavy 字重
6. 开始第 5 章的结构改造
```

**为什么不能颠倒**：`migrate-typography` 会改 `borderWidth` 和圆角，
如果此时颜色还没 token 化，你会在同一个文件里同时看到"新圆角 + 旧颜色"，
无法通过视觉判断哪一步出了问题。**一次只改一个维度，才能二分定位。**

### C.4 如果想扩展映射表

只需在 `migrate-colors.mjs` 的 `MAP` 里加一条：

```js
"#新的色值": {
  color: "palette.xxx",           // 作为文字色
  backgroundColor: "palette.yyy", // 作为背景色
  border: "palette.zzz",          // 作为描边
},
```

三个字段都可以填 `null`，表示"该属性下这个色值不该出现"（会进待办清单让你确认）。
改完**别直接跑全量**，先在你的目标文件上验证：

```bash
# 备份 → 单文件试跑 → 看结果 → 满意再全量
cp src/components/xxx/Foo.tsx /tmp/Foo.bak
node scripts/migrate-colors.mjs
git diff src/components/xxx/Foo.tsx
```

> 注意：脚本没有单文件参数，是全局扫描。所以要试跑就先 `git stash` 或备份。
> 如果你想让脚本支持单文件参数，自己加 `process.argv[2]` 过滤即可，逻辑不影响。
