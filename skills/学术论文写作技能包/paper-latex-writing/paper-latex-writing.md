---
name: paper-latex-writing
description: "学术论文LaTeX撰写与编译。当用户说'写论文'、'写LaTeX'、'撰写论文'、'paper write'、'draft LaTeX'、'编译论文'、'生成PDF'、'paper compile'时使用。涵盖逐节撰写、5遍科学写作审计、参考文献验证、PDF编译与格式检查。"
---

# 学术论文 LaTeX 撰写与编译 (Paper LaTeX Writing)

## 触发条件

用户提到以下任意场景时使用本 Skill：
- "写论文"、"撰写论文"、"写LaTeX"、"paper write"、"draft"
- "编译论文"、"生成PDF"、"paper compile"、"build PDF"
- "论文润色"、"改写论文"、"科学写作"

## 概述

将论文大纲（PAPER_PLAN.md）转为完整的 LaTeX 源文件，经过 5 遍科学写作审计 + 内置独立审稿 + 编译修复，产出可直接投稿的 PDF。

## 前置条件

- **必需**: PAPER_PLAN.md（由 `paper-planning` 产出）
- **推荐**: 已完成图表（由 `paper-figures` 产出）

## 操作步骤

### Step 1: 创建项目结构

```
paper/
├── main.tex                    # 主文件
├── math_commands.tex           # 共享数学宏
├── references.bib              # 参考文献（仅含被引用条目）
├── sections/
│   ├── 0_abstract.tex
│   ├── 1_introduction.tex
│   ├── 2_related_work.tex
│   ├── 3_method.tex
│   ├── 4_experiments.tex
│   ├── 5_conclusion.tex
│   └── A_appendix.tex
└── figures/                    # 符号链接或复制
```

### Step 2: 逐节撰写

#### Abstract
- 5 部分结构：what, why hard, how, evidence, strongest result
- 150-250 词，必须自包含
- 含一个具体量化结果
- 无引用、无未定义缩写

#### Introduction
- 引人入胜的开头 hook（1-2 句）
- 明确差距陈述（"However, ..."）
- 方法概述（在读者迷失细节前给出全貌）
- 2-4 个具体可证伪的贡献
- 最强结果预览
- 路线图: "The rest of this paper is organized as..."
- 目标 1-1.5 页

#### Related Work
- **至少 1 整页**（3-4 个实质性段落）
- 按方法论族/假设类别/研究问题组织，**不按 paper-by-paper 罗列**
- 使用 `\paragraph{Category Name.}` 组织
- 每段以本文与相关工作的关系/区别结尾

#### Method
- 尽早定义符号（参考 `math_commands.tex`）
- 使用正式数学环境: `\begin{definition}`, `\begin{theorem}`
- 理论论文：正文含关键结果证明草图，完整证明放附录
- 如适用：伪代码（`algorithm2e` 或 `algorithmic`）
- 目标 1.5-2 页

#### Experiments
- 从实验设置开始（数据集、baselines、指标、实现细节）
- 先放主要结果表/图
- 随后是消融和分析
- Introduction 中的每个论断必须有支持证据
- 目标 2.5-3 页

#### Conclusion
- 总结贡献（重新表述，不复制 intro）
- 诚恳的局限性说明
- 1-2 个具体未来方向
- 伦理声明和可复现声明（如 venue 要求）
- 目标 0.5 页

### Step 3: 参考文献构建

**三步验证链条**：
1. **DBLP**（最佳质量）— 通过标题+第一作者搜索获取真实 BibTeX
2. **CrossRef DOI**（备选）— 通过 DOI 获取 BibTeX
3. **标记 [VERIFY]**（最后手段）— 两者都失败时标记

**引用清洁规则**：
- `references.bib` 仅含实际被 `\cite{}` 的条目
- 每条目必须有 author, title, year, venue/journal
- 优先使用已发表 venue 版本而非 arXiv 预印本
- 键位格式: `{第一作者}{年份}{关键词}`
- **永不从 LLM 记忆生成 BibTeX**，必须通过 DBLP/CrossRef 验证

### Step 4: 5 遍科学写作审计

**Pass 1: 去冗余 (Clutter Extraction)**

| 冗余短语 | 替换为 |
|---------|--------|
| Due to the fact that | Because |
| In order to | To |
| A number of | Several |
| It is worth noting that | (删除) |
| It is important to note that | (删除) |

删除 AI-痕迹词: delve, pivotal, landscape, tapestry, underscore, noteworthy, intriguingly

**Pass 2: 主动语态 (Active Voice)**
- 识别被动语态，转换为主动
- 复活被名词化的动词: "made an investigation" → "investigated"

**Pass 3: 句子架构 (Sentence Architecture)**
- 标记 >40 词的句子，拆分
- 主语和动词靠近
- 每段只承担一个任务
- 检查段落过渡是否自然

**Pass 4: 关键词一致性 (Banana Rule)**
- "Banana" 不要变成 "elongated yellow fruit" 来避免重复
- Methods 中的术语必须在 Results/Discussion/Tables/Captions 中保持一致
- 缩写节俭：仅为方便而创建的非标准缩写在全文首次出现后统一使用

**Pass 5: 数字与引用完整性**
- 样本量 (N) 在 Abstract 和 Table 1 中一致吗？
- Results 中的百分比和 Tables 中的原始数字匹配吗？
- 有效数字一致且适当吗？
- 图的数据和表的值匹配吗？

### Step 5: 内置独立审稿

使用内置审稿能力对完整草稿进行独立审阅（审阅过程在本地完成，草稿内容不外传）：
1. Intro 中的每个论断都有支持证据？
2. 写作清晰、简洁、无 AI 痕迹？
3. 有逻辑缺口或不清晰的解释？
4. 在页面限制内？
5. Related Work 足够全面（≥1 页）？
6. 理论论文：证明草图足够？
7. 图/表描述清晰且引用正确？
8. 略读读者能从 title + abstract + intro + Figure 1 理解贡献？

### Step 6: 反向大纲测试

1. 提取每段主题句
2. 按顺序阅读 — 应形成连贯叙述
3. 检查论断覆盖 — 每个 Claims-Evidence Matrix 中的论断必须出现
4. 检查证据映射 — 每个实验/图必须支持一个陈述的论断

### Step 7: 编译

- 使用 `latexmk -pdf` 自动多遍编译
- 自动修复常见错误（缺失包、未定义引用、BibTeX 语法）
- 最多 3 次编译尝试
- 编译后运行格式检查：
  - 主文 overfull hbox > 0pt：阻止完成
  - 附录 overfull hbox > 10pt：阻止完成
  - 参考文献 overfull hbox > 20pt：阻止完成
  - 重复标签：硬阻止
  - 超页面限制：建议削减

## 输出

```
paper/
├── main.pdf                      # 编译后的 PDF
├── main.tex                      # 主 LaTeX 文件
├── math_commands.tex             # 数学宏
├── references.bib                # 验证过的参考文献
├── sections/                     # 各节 .tex 文件
├── figures/                      # 图文件
└── SCIWRITE_AUDIT.md             # 5 遍审计检查清单
```

## 与其他 Skill 的关系

- 配合 `paper-planning`：接收 PAPER_PLAN.md 和 Claims-Evidence Matrix
- 配合 `paper-figures`：接收 `latex_includes.tex` 和图表文件
- 产出完整 PDF 后可交付 `paper-improvement` 进行多轮审稿

## 模板与示例

参考 `templates/` 目录下的 LaTeX 模板文件：

```
templates/
├── main.txt             # 主文件模板
├── math_commands.txt    # 数学宏模板
├── abstract.txt         # Abstract 模板
├── introduction.txt     # Introduction 模板
├── related_work.txt   # Related Work 模板
├── method.txt            # Method 模板
├── experiments.txt      # Experiments 模板
└── conclusion.txt       # Conclusion 模板
```

## 注意事项

- 引用不编造：使用 DBLP/CrossRef 验证，永远不从 LLM 记忆生成 BibTeX
- `references.bib` 仅含被引用条目：自动清理死条目
- 编译必须成功才能进入审稿改进阶段
- 写大文件时如果 Write 工具失败，用 Bash 分块写入，不询问用户
- 创建前先检查是否已有 paper/ 目录，如有则备份
