---
name: 学术论文写作技能包
description: "学术论文从规划到投稿的全流程技能。当用户说'写论文'、'论文写作'、'写学术论文'、'paper writing'、'投稿论文'、'撰写论文全流程'时使用。调度5个子技能：paper-planning → paper-figures → paper-latex-writing → paper-review → paper-improvement，覆盖大纲规划、配图、LaTeX撰写、多维度审稿、改稿循环全流程。"
---

# 学术论文写作技能包

## 触发条件

用户提到以下任意场景时使用本技能：
- "写论文"、"论文写作"、"写学术论文"、"paper writing"
- "投稿论文"、"撰写论文全流程"、"发论文"
- "帮我写一篇完整的论文"、"从零开始写论文"

## 概述

本技能是一个**总调度器**，按顺序调用 5 个独立子技能，覆盖学术论文从规划到投稿的全流程：

```
paper-planning ──→ paper-figures
       │                  │
       └──────┬───────────┘
              ↓
     paper-latex-writing
              │
              ↓
        paper-review ──→ paper-improvement
                              │
                              ↓ (迭代 2 轮)
                     paper-review (独立再审)
                              │
                              ↓
                         📄 终稿 PDF
```

## 五个子技能

| # | 子技能文件夹 | 名称 | 功能 | 阶段 |
|---|-------------|------|------|------|
| 1 | `paper-planning/` | 论文大纲规划 | 论断-证据矩阵、逐节规划、图表规划、引文脚手架 | 规划 |
| 2 | `paper-figures/` | 论文配图与表格 | 数据图、架构图(JSON→SVG)、AI插图、Mermaid流程图 | 配图 |
| 3 | `paper-latex-writing/` | LaTeX撰写与编译 | 逐节撰写、5遍科学写作审计、参考文献验证、PDF编译 | 写作 |
| 4 | `paper-review/` | 多维度审稿与审计 | 数字核对、引用审核、证明验证、对抗审查、综合审稿 | 审稿 |
| 5 | `paper-improvement/` | 审稿改稿循环 | 汇总审计问题 → 逐条修复 → 重编译 → 独立再审 (×2) | 改进 |

## 操作步骤

### 阶段 1: 论文规划（调用 `paper-planning/paper-planning.md`）

**调用条件**: 用户有研究方向+实验结论，需要规划论文结构。

**操作**:
1. 阅读 `paper-planning/paper-planning.md` 中的完整指令
2. 按指令构建 Claims-Evidence Matrix（论断-证据矩阵）
3. 确定论文类型和结构（实证/理论+实验/方法论）
4. 逐节规划：Abstract五部分 → Introduction → Related Work → Method → Experiments → Conclusion
5. 图表规划：列出所有需要的图和表，标注优先级(HIGH/MED/LOW)
6. 引文脚手架：为每个 section 列出需要的引文类别
7. 使用内置审稿能力对完整大纲进行独立审阅（审阅过程在本地完成，大纲内容不外传）

**产出**: `PAPER_PLAN.md`

### 阶段 2: 论文配图（调用 `paper-figures/paper-figures.md`）

**调用条件**: PAPER_PLAN.md 已生成，需要制作图表。

**操作**:
1. 阅读 `paper-figures/paper-figures.md` 中的完整指令
2. 按 PAPER_PLAN.md 中的图表规划，按优先级(HIGH→MED→LOW)生成
3. 选择合适的生成模式：
   - **数据图**：使用 matplotlib（IEEE配色、矢量PDF输出）
   - **架构图**：使用 FigureSpec JSON→SVG
   - **AI插图**：使用平台内置图像生成能力
   - **流程图**：使用 Mermaid
4. 生成 LaTeX 表格（`booktabs` 规范）
5. 输出 `figures/latex_includes.tex` 供后续引用

**产出**: `figures/` 目录 + `latex_includes.tex`

### 阶段 3: LaTeX 撰写（调用 `paper-latex-writing/paper-latex-writing.md`）

**调用条件**: PAPER_PLAN.md + 图表已就绪，开始撰写论文正文。

**操作**:
1. 阅读 `paper-latex-writing/paper-latex-writing.md` 中的完整指令
2. 创建项目结构（可参考 `paper-latex-writing/templates/` 下的模板）：
   - `main.tex`、`math_commands.tex`、`references.bib`
   - `sections/`（0_abstract ~ A_appendix）
3. 逐节撰写：Abstract → Introduction → Related Work → Method → Experiments → Conclusion
4. 构建参考文献：使用 DBLP/CrossRef 三步验证链条
5. 执行 **5 遍科学写作审计**：
   - Pass 1: 去冗余 (Clutter Extraction)
   - Pass 2: 主动语态 (Active Voice)
   - Pass 3: 句子架构 (Sentence Architecture)
   - Pass 4: 关键词一致性 (Banana Rule)
   - Pass 5: 数字与引用完整性
6. 内置独立审稿（全新上下文线程，审稿在本地完成）
7. 反向大纲测试
8. 编译 PDF：`latexmk -pdf`，修复 overfull hbox、缺失包等问题

**产出**: `paper/` 目录 + `main.pdf`

### 阶段 4: 多维度审稿（调用 `paper-review/paper-review.md`）

**调用条件**: 论文 PDF 已编译成功，需要进行审稿。

**操作**:
1. 阅读 `paper-review/paper-review.md` 中的完整指令
2. 执行五大审计维度：
   - **Audit 1: 数字核对** — 每个数字与原始数据是否一致？
   - **Audit 2: 引用审核** — 引用存在性、元数据正确性、上下文适当性（参考 `paper-review/reference/citation-discipline.md`）
   - **Audit 3: 证明验证** — 仅理论论文，数学证明的严密性检查
   - **Audit 4: 对抗性审查** — 双线程攻防：攻击者→最强拒稿理由，辩护者→逐条回应
   - **Audit 5: 综合审稿** — 完整结构化评审（评分 1-10）
3. **严格遵循审计独立性协议**（参考 `paper-review/reference/reviewer-independence.md`）：
   - 每项 Audit 使用全新内置审稿上下文线程（本地处理，不外传）
   - 审稿人仅接收 .tex 和原始数据，不接收任何意图说明
   - 裁决由代码计算，不由审稿人自评
   - 涉及论文原文和原始数据时，需获得用户确认后方可进行审稿

**产出**: `AUTO_REVIEW.md` + `CITATION_AUDIT.md` + `PAPER_CLAIM_AUDIT.md` + `PROOF_AUDIT.md` + `KILL_ARGUMENT.md`

### 阶段 5: 改稿循环（调用 `paper-improvement/paper-improvement.md`）

**调用条件**: 审计报告已生成，需要根据审稿意见修改论文。

**操作**:
1. 阅读 `paper-improvement/paper-improvement.md` 中的完整指令
2. 汇总所有审计问题，按严重性排序：CRITICAL > MAJOR > MINOR
3. 按优先级逐条实现修复（参考 `paper-improvement/reference/common-fix-patterns.md`）
4. 每轮修复后重新编译 PDF
5. 用全新内置审稿上下文线程进行下一轮独立审稿（本地处理，不外传）
6. 对比评分变化，满足终止判据时停止

**终止判据**:
- 评分 ≥ 7.0（Accept）且无 CRITICAL/MAJOR 问题
- 已完成最大轮数（默认 2 轮）
- 两轮间评分提升 < 0.5

**产出**: `main_round0/1/2.pdf` + `FINAL_REPORT.md`

## 使用方式

### 全流程执行
当用户说"写论文"且提供研究方向+实验结论时，依次执行阶段1→2→3→4→5。

### 部分执行
根据用户当前阶段选择性执行：
```
# 只有想法，需要规划
→ 阶段 1: paper-planning

# 已有 PAPER_PLAN.md，需要生成图表
→ 阶段 2: paper-figures

# 大纲+图表就绪，开始写作
→ 阶段 3: paper-latex-writing

# 论文写完，需要审稿
→ 阶段 4: paper-review

# 审稿意见来了，需要修改
→ 阶段 5: paper-improvement
```

### 单独子技能触发词

| 子技能 | 直接触发词 |
|--------|----------|
| paper-planning | "写大纲"、"论文规划"、"paper plan"、"论文结构" |
| paper-figures | "画图"、"论文配图"、"生成图表"、"paper figure"、"作图" |
| paper-latex-writing | "写论文"、"写LaTeX"、"draft LaTeX"、"编译论文"、"生成PDF" |
| paper-review | "审稿"、"review"、"数字核对"、"引用审核"、"证明验证" |
| paper-improvement | "改论文"、"论文改进"、"improve paper"、"根据审稿意见修改" |

## 设计原则

1. **单一职责**: 每个子技能只做一件事，做好一件事
2. **可独立使用**: 不强制依赖其他子技能，可按需组合
3. **以审稿人为中心**: 所有审计使用零上下文独立审稿人，模拟真实 peer review
4. **可追溯**: 保留 round0/round1/round2 全部版本，每步改进有据可查
5. **符合标准**: 每个子技能符合 SkillHub 规范（YAML frontmatter + slug命名 + 触发关键词）

## 注意事项

- 执行阶段 3（LaTeX撰写）时，引用**永不编造**：必须通过 DBLP/CrossRef 验证
- 执行阶段 4（审稿）时，**严格遵循零上下文原则**：审稿人只看到文字，不知道意图
- 执行阶段 5（改稿）时，**绝不摧毁原始版本**：保留所有轮次的 PDF
- 如果用户只提供了模糊想法而非完整实验结论，应先执行阶段 1（规划），在 PAPER_PLAN.md 中标注需要补充的实验
- 编写大文件时如果 Write 工具失败，用 Bash 分块写入，不询问用户

## 参考来源

- 论断-证据矩阵方法: [Research-Paper-Writing-Skills](https://github.com/Master-cai/Research-Paper-Writing-Skills)
- 引用验证方法: 基于 DBLP/CrossRef 的文献存在性-元数据正确性-上下文适当性三重验证链条
- 科学写作审计: Sainani's "Writing in the Sciences" (Stanford)
- 审稿人独立性协议: ARIS `/research-review`, `/kill-argument`
- IEEE 配色方案: IEEE Transactions visualization guidelines
- 零上下文审计: ARIS `/paper-claim-audit`, `/citation-audit`
