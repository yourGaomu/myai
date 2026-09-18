# MyAI Mobile Android UI 原型系统架构与规范指南

本目录（`mobile/remote_ui`）专门存放 **MyAI Android 客户端** 的高保真产品原型系统。该系统基于当前 React Native / Expo 客户端工程源码深度复刻构建，真实还原了所有核心交互逻辑、状态流转与 Neo-Brutalism 粗黑线条视觉风格。

---

## 1. 架构设计概览 (Architecture Design)

为了保证**“零环境门槛、高解耦、高扩展性”**，本项目采用了模块化现代 Web 架构，分为 4 大核心层次：

```text
mobile/remote_ui/
├── index.html                    # 原型展示台入口（包含三栏工作台与真机视窗）
├── preview.bat                   # Windows 专属双击免配置启动器
├── package.json                  # 项目元数据与便捷命令
├── assets/                       # 图标、头像动效帧、静态资源
├── css/                          # 模块化样式系统
│   ├── tokens.css                # 设计变量（颜色、字体、粗边框、硬投影、圆角）
│   ├── reset.css                 # 基础样式统一与重置
│   ├── device.css                # Android 真机模拟器外壳、打孔摄像头、状态栏与底栏
│   ├── components.css            # 通用原子组件（按钮、输入框、气泡、标签、模态框）
│   ├── screens.css               # 8 大核心屏幕专有样式
│   └── workbench.css             # 演示台控制面板样式（快速切换栏、场景模拟器）
└── js/                           # JavaScript 逻辑分层
    ├── data/                     # 真实复原自 protocol.ts 的 Mock 数据库
    │   ├── mockChat.js           # 对话流、Thinking 深度思考、工具调用与时间线
    │   ├── mockFiles.js          # 工作区文件目录树、会话共享资产、代码预览
    │   ├── mockChanges.js        # 代码改动列表、SQLite 历史快照、Diff 补丁
    │   ├── mockKnowledge.js      # 向量知识库、文档索引、AI 记忆与 Dream 提取
    │   ├── mockPlan.js           # 结构化任务规划进度与步骤
    │   ├── mockSessions.js       # 多会话列表与回收站
    │   └── mockSettings.js       # 中继配置、大模型列表、技能插件与子智能体
    ├── store/                    # 响应式状态管理
    │   └── store.js              # 单向数据流、事件总线、状态订阅与模拟器触发器
    ├── components/               # 通用业务组件
    │   ├── StatusBar.js          # 动态时间与安卓系统状态栏
    │   ├── AppHeader.js          # 呼吸动效小人、标题、连接药丸、设置入口
    │   ├── BottomDock.js         # 底部停靠栏（输入框 Composer + 5Tab 导航栏）
    │   ├── PermissionModal.js    # 危险操作拦截审批弹窗
    │   └── Toast.js              # 全局轻提示组件
    ├── screens/                  # 8 大核心屏幕渲染器
    │   ├── ChatScreen.js         # 对话主屏幕（气泡、Thinking 折叠块、工具组）
    │   ├── FilesScreen.js        # 工作区文件树与共享资源
    │   ├── ChangesScreen.js      # 文件变动与版本快照
    │   ├── ChangeDetailScreen.js # 代码差异对比器 (Diff Viewer)
    │   ├── KnowledgeScreen.js    # RAG 知识库与 AI 记忆中心
    │   ├── PlanScreen.js         # 规划执行进度面板
    │   ├── SessionsScreen.js     # 多会话管理与回收站
    │   └── SettingsScreen.js     # 全局设置与子智能体控制台
    └── app.js                    # 主程序入口驱动
```

---

## 2. 如何运行与预览 (How to Run)

### 方法一：Windows 一键双击运行（最推荐）
直接在资源管理器中双击打开：
```text
mobile/remote_ui/preview.bat
```
脚本会自动探测本地的 Python 或 Node 环境启动微型本地服务器，并自动弹出浏览器打开展示台。

### 方法二：使用 Node / NPX
在终端执行：
```powershell
cd D:\Go_All\myai\mobile\remote_ui
npx serve . -p 3000
```
浏览器访问：`http://localhost:3000/index.html`。

### 方法三：使用 Python
```powershell
cd D:\Go_All\myai\mobile\remote_ui
python -m http.server 8099
```
浏览器访问：`http://localhost:8099/index.html`。

---

## 3. 复原的 8 大核心业务屏幕

1. **💬 对话主面板 (ChatScreen)**：
   - 顶部提供 [对话] / [计划] 模式切换胶囊；
   - 包含 Agent 任务时间线、用户黄色气泡、助手白色卡片气泡；
   - 包含 **Thinking 深度思考折叠块**（点击展开/收起）；
   - 包含 **工具调用卡片组 (ToolActivityGroup)**，展示参数及返回结果；
   - 底部停靠 Composer 输入框（含文件附件卡片、发送/暂停按钮）。
2. **📂 工作区文件 (FilesScreen)**：
   - 会话共享资源卡片（直接外链打开）；
   - PC Agent 工作区文件树目录导航、面包屑与上一级返回；
   - 选中文件的代码预览卡片与“附加至对话”联动。
3. **📝 代码变更 (ChangesScreen)**：
   - 工作区改动列表：带有 `[M]` (修改/黄)、`[A]` (新增/绿)、`[D]` (删除/红) 状态徽章；
   - SQLite 历史快照检查点卡片，支持版本对比与回滚。
4. **🔀 差异对比 (ChangeDetailScreen)**：
   - 高保真统一差异对比器 (Unified Diff Viewer)，红绿行级高亮；
   - 顶部快捷支持“返回变更”与“一键还原文件”。
5. **📚 知识库与 AI 记忆 (KnowledgeScreen)**：
   - 双 Tab 切换：**资料知识库**（RAG 知识库、分块索引、文档列表）与 **AI 记忆**（长期记忆卡片、来源溯源、Dream 记忆整合）。
6. **🎯 任务规划执行 (PlanScreen)**：
   - 规划执行总目标概览；
   - 进度条与完成度百分比；
   - 步骤清单（就绪、运行中、已完成状态机）；
   - 底部“返回对话”与“执行计划”控制。
7. **🗂️ 会话中心 (SessionsScreen)**：
   - 活跃会话卡片列表（模型标识、权限模式、最后时间）；
   - 顶部“+ 新建会话”浮动动作；
   - 底部“回收站”与已删除会话一键恢复。
8. **⚙️ 全局设置与子智能体 (SettingsScreen)**：
   - 涵盖常规、中继连接（Relay URL / 6位配对码）、模型中心、子智能体 Subagents、技能中心、插件扩展。
9. **⚠️ 危险操作权限拦截 (PermissionModal)**：
   - 当 Agent 调用本地 Shell、文件删除等危险操作时，弹出醒目的黄色全屏权限审批卡片，支持确认允许与拒绝。

---

## 4. 演示台场景模拟器 (Live Simulation)

右侧控制台集成了多种一键业务场景触发器：
- **⚡ 模拟思考与打字机输出**：真机顶部头像开始呼吸动效，状态药丸转为“正在思考...”，随后生成 Thinking 思考卡并打字流式输出消息；
- **🛠️ 模拟系统工具调用过程**：模拟调用 `git_diff_summary` 工具并在消息流中插入工具请求与返回卡片；
- **⚠️ 触发危险操作权限弹窗**：弹出高保真权限确认卡片，点击允许或拒绝直接驱动后续对话；
- **🔄 切换对话 / 规划模式**：在普通对话执行与只读结构化规划间切换；
- **📡 模拟在线 / 离线切换**：观察顶部状态药丸的颜色与小人状态变化；
- **🖥️ 切为真机外壳 / 平铺画布**：支持带手机物理机身的演示模式与适合截图嵌入文档的平铺模式。

---

## 5. 设计令牌规范 (Design Tokens)

- **核心配色**：
  - 主强调色（明黄）：`#ffd84f`
  - 次强调色（高光青）：`#4fd7ee`
  - 成功/就绪（嫩绿）：`#b9e9b0`
  - 危险/删除/离线（珊瑚红）：`#ff7f68`
  - 背景底色：`#f4f5f7`
  - 卡片底色：`#fffdf7`
  - 墨黑线条：`#12100e` / `#25231f`
- **边框与阴影**：
  - 标准边框：`2px solid #12100e`
  - 强调边框：`4px solid #12100e`
  - 硬投影：`0 4px 0 #12100e`
