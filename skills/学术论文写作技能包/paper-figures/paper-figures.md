---
name: paper-figures
description: "学术论文配图与表格生成。当用户说'画图'、'论文配图'、'生成图表'、'paper figure'、'作图'、'数据可视化'、'论文插图'时使用。支持数据图(matplotlib)、架构图(JSON→SVG)、AI插图(内置生成)和流程图(Mermaid)四种模式。"
---

# 学术论文配图与表格 (Paper Figures)

## 触发条件

用户提到以下任意场景时使用本 Skill：
- "画图"、"作图"、"生成图表"、"paper figure"
- "论文配图"、"论文插图"、"数据可视化"
- "做表格"、"对比表"、"latex表格"

## 概述

为学术论文生成符合期刊标准的配图与表格。支持四种生成模式，遵循 IEEE 配色和学术出版规范。

## 四种生成模式

| 模式 | 适用场景 | 输出格式 | 依赖 |
|------|---------|---------|------|
| **数据图 (matplotlib)** | 折线图、柱状图、散点图、热力图 | `.pdf` + `.png` | matplotlib, seaborn |
| **架构图 (FigureSpec)** | 系统架构、管道流程、级联图 | `.svg` + `.json` | 无外部依赖 |
| **AI插图** | 定性方法示意、自然风格插图 | `.png` | 平台内置图像生成 |
| **流程图 (Mermaid)** | 轻量流程图、状态机、序列图 | `.mmd` + `.png` | mermaid-cli |

## 操作步骤

### Step 1: 读取图表规划

如有 PAPER_PLAN.md 中的图表规划表，按优先级处理：
- 🔴 HIGH 优先级：首先生成
- 🟡 MED 优先级：后续生成
- 🟢 LOW 优先级：时间允许时生成

### Step 2: 按模式生成

#### 模式 A: 数据图 (matplotlib)

```python
# 设计原则 (参考 top-journal-scientific-figures)
- IEEE 配色方案：蓝 #0072BD, 橙 #D95319, 黄 #EDB120, 紫 #7E2F8E, 绿 #77AC30, 青 #4DBEEE, 红 #A2142F
- 无箭头、无装饰、无3D效果
- 干净简洁：最少刻度线、直接数据标签
- 颜色在灰度下可区分
- 一致的字号(8-10pt)和线宽(1-1.5pt)
- 输出矢量 PDF（300dpi 光栅化位图元素）
```

#### 模式 B: 架构图 (FigureSpec → SVG)

1. 用 JSON 精确描述：节点位置(x,y)、尺寸(w,h)、颜色、标签
2. deterministic 渲染为 SVG
3. 输出到 `figures/*.svg` 和 `figures/specs/*.json`
4. 优点：可版本控制、可精确审查、无 API 依赖

#### 模式 C: AI插图

1. 使用平台内置图像生成能力规划构图并生成初稿
2. 内置审稿能力审查质量（评分≥9/10 才通过）
3. 迭代最多 3 轮
4. 适用于定性方法插图、自然场景示意

#### 模式 D: Mermaid 流程图

1. 编写 `.mmd` 文件（Mermaid 语法）
2. 使用 mermaid-cli 渲染为 PNG/SVG
3. 验证语法无误
4. 适用于简单流程图、状态机

### Step 3: 生成 LaTeX 表格

```latex
% 对比表模板
\begin{table}[t]
\centering
\caption{Main results comparison. Best results in \textbf{bold}, second-best \underline{underlined}.}
\label{tab:main}
\begin{tabular}{lcccc}
\toprule
Method & Metric1 & Metric2 & Metric3 & Avg \\
\midrule
Baseline A & 72.3 & 68.1 & 75.4 & 71.9 \\
Baseline B & 74.5 & 70.2 & 77.1 & 73.9 \\
\textbf{Ours} & \textbf{85.3} & \textbf{81.7} & \textbf{86.2} & \textbf{84.4} \\
\bottomrule
\end{tabular}
\end{table}
```

**表格规范**：
- 使用 `booktabs` 包：`\toprule`, `\midrule`, `\bottomrule`
- 加粗最佳结果，下划线次佳
- 对齐小数点（使用 `siunitx` 包的 `S` 列类型）
- 每一列含义明确的表头

### Step 4: 输出集成

生成 `figures/latex_includes.tex`，包含所有图和表的 `\includegraphics` / `\input` 语句，可直接在主 LaTeX 文件中引用。

## 输出目录结构

```
figures/
├── fig1_hero.svg / .pdf
├── fig2_training_curves.pdf
├── fig3_ablation.pdf
├── table1_main.tex
├── specs/
│   └── fig1_hero.json
├── mmd/
│   └── flowchart.mmd
└── latex_includes.tex
```

## 与其他 Skill 的关系

- 接收 `paper-planning` 的图表规划作为输入
- 输出 `latex_includes.tex` 供 `paper-latex-writing` 直接引用
- 可独立使用（不依赖其他 Skill），只需提供数据文件

## 注意事项

- 所有图应能在灰度打印下区分（使用不同的线型或标记形状辅助）
- 图标题应自包含：读者不读正文也能理解图的内容
- 字号不小于 7pt（出版印刷的最小可读字号）
- 矢量图优先（PDF/SVG），避免像素化
