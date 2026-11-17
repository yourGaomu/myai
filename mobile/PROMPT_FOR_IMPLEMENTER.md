# 提示词（复制以下全部内容，交给执行者 / 执行型 AI）

---

你是本项目的移动端 UI 改造执行者。任务是把这个 React Native (Expo SDK 56) App 的界面从
「浅色纸感 + 2px 黑描边 + 硬阴影」改造为「深色画布 + 毛玻璃浮岛 + 暖黄强调」的
**方向 B · 午夜玻璃** 设计。

## 第一步：读文档（不要跳过）

在 `mobile/` 目录下有两份必读文件：

1. **`UI_REDESIGN_MIDNIGHT_GLASS.md`** —— 完整施工规格。请**通读一遍再动手**，
   特别注意：
   - 第 3 章（同一个颜色作为不同属性要映射到不同 token，这是最容易做错的地方）
   - 第 9 章（8 个步骤的执行顺序，不可跳步）
   - 附录 A（常见陷阱，能帮你避开几个必踩的坑）
   - 第 10 章（验收标准，最后必须逐条通过）

2. **`ui-redesign-prototype.html`** —— 视觉基准。用浏览器打开，看「方向 B · 午夜玻璃」
   的两个手机屏幕。你实现的最终效果应该和它一致。**遇到任何视觉判断，以它为准。**

## 第二步：按第 9 章的 8 个步骤执行

**每一步都必须能独立编译通过，并单独提交一个 commit。** 顺序是：

1. **基础设施** —— 重写 `src/constants/theme.ts`（文档第 2 章有完整代码，直接整份复制），
   安装 5 个依赖，改 `app.json` 版本号，改 `utils/buttonFeedback.ts`
2. **骨架** —— `MobileScreenShell.tsx`、`MobileAppScreen.tsx`、`useAndroidNavigationBar.ts`、`BottomDock.tsx`
3. **共享样式** —— `chat/styles.ts`（这个文件被 8 个组件 import，改了它等于改了 8 个组件）
4. **核心结构** —— `AppHeader.tsx`、`BottomTabs.tsx`、`Composer.tsx`（视觉主体在这里成型）
5. **消息组件** —— `MessageBubble.tsx`、`MarkdownText.tsx`、`ThinkingReasoning.tsx` 等
6. **面板类** —— Files / Changes / Knowledge / Plan / Sessions / Subagents / Plugins / Permissions 等
7. **最大文件** —— `SettingsPanel.tsx`（4496 行、251 处颜色，单独做）
8. **收尾** —— emoji 图标替换、头像资源、清理待办清单

## 第三步：用现成的脚本，不要手工硬改

仓库里已经有两份**实测过**的迁移脚本：

```bash
cd mobile

# 第 3 步之后、第 4 步之前运行：
node scripts/migrate-colors.mjs        # 颜色 → token，自动完成 71.9%
node scripts/migrate-typography.mjs    # 字号/字重/描边/圆角 → token ，自动完成 95%+
```

**跑完脚本后必须人工补两件事**：

1. **补 import**。每个被改动的文件顶部要加：
   ```ts
   import { palette, radius, font, weight, shadow } from "../../constants/theme";
   ```
   （`components/xxx/` 下是 `../../constants/theme`，`screens/` 下是 `../constants/theme`）

2. **处理 `migration-todo.txt`**。里面约 550 项，按文档第 3.4 节的兜底规则判断。
   其中两类有现成答案，直接查表：
   - `DiffViewer.tsx` 的深色值 → 文档 4.3.1 节有完整对照表
   - `AgentTurnCard.tsx` 的状态色 → 文档 4.3.2 节有语义归类表

**注意 `migrate-typography.mjs` 的副作用**：它会把所有 `fontWeight: "900"` 降为 `weight.bold`。
跑完你要人工把「页面主标题 / 数字徽章 / 进度百分比」这三类改回 `weight.heavy`。
拿不准就保持 `bold` —— 轻一点远好过回到"全屏都最响"。

## 硬性约束（违反即返工）

1. **禁止任何颜色字面量**。`src/` 下除 `theme.ts` 外不得出现 `"#xxxxxx"` 或 `"rgba(...)"`。
   验收脚本会查，必须输出 `PASS`。
2. **禁止 `borderWidth: 2`**。全部改 `1`。原代码有 100 处。
3. **禁止硬阴影**（`shadowRadius: 0` + `shadowOpacity: 1`）。原代码 45 处，改用
   `...shadow.float` / `...shadow.card` / `...shadow.brandGlow`。
4. **禁止非法字重**。`"450"` `"550"` `"650"` 在 RN 里是非法的，会被静默忽略。
5. **禁止 emoji 当图标**。全部换成 `@expo/vector-icons` 的 Feather 图标，
   文档第 6.3 节有完整的字符 → 图标名对照表。
6. **不要改业务逻辑**。hooks、协议、storage、滚动逻辑、props 签名一律不动
   （除文档明确列出的新增可选 props）。这次只改视觉层。

## 必须注意的三件事

1. **同一颜色按属性分流**。`#12100e` 作为文字色要变亮（`palette.ink1`），
   作为描边色要变淡（`palette.hairline`），作为阴影色要删除。**绝对不能全局查找替换。**

2. **`app.json` 的 splash 背景色有两处**（顶层 `splash.backgroundColor` 和
   `expo-splash-screen` 插件配置里各一处），只改一处会导致部分 Android 版本启动时闪浅色。

3. **这是原生改动，OTA 更新无效**。新增了 4 个含原生代码的包，必须重新出包
   （`npx expo run:android` 或 `npm run build:android:apk`）。

## 第四步：验收

按文档第 10 章逐条过：

- **10.1 自动化检查**（7 项，必须全部输出 PASS）
- **10.2 视觉验收**（真机截图，与原型 HTML 逐屏对比）
- **10.3 回归测试**（14 项交互必须与改造前完全一致）
- **10.4 交付物**（8 个 commit + 检查输出 + 真机截图 + 遗留问题说明）

## 卡住了怎么办

- **视觉拿不准** → 看原型 HTML 的方向 B 部分
- **某个色值不知道映射到哪** → 按文档 3.4 节的「饱和度 + 明度」兜底规则判断，不要猜
- **文档没说清** → **停下来记录问题**，不要自行发挥。把问题汇总给需求方一起确认
- **原生编译失败** → 先解决依赖链接，不要继续改代码

现在开始执行。第一步是读 `mobile/UI_REDESIGN_MIDNIGHT_GLASS.md`。
