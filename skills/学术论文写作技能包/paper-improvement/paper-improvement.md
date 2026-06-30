---
name: paper-improvement
description: "学术论文多轮审稿改稿循环。当用户说'改论文'、'论文改进'、'improve paper'、'论文润色循环'、'auto improve'、'根据审稿意见修改'时使用。接收paper-review的审计报告，按CRITICAL→MAJOR→MINOR优先级逐条修复，每轮重新编译并独立再审，直至达标。"
---

# 学术论文多轮审稿改稿循环 (Paper Improvement Loop)

## 触发条件

用户提到以下任意场景时使用本 Skill：
- "改论文"、"论文改进"、"improve paper"、"论文润色循环"
- "auto improve"、"根据审稿意见修改"、"按审稿意见改"
- "论文多轮改进"、"迭代改进论文"

## 概述

接收 `paper-review` 产出的审计报告，按照优先级（CRITICAL → MAJOR → MINOR）逐条实现修复，每轮修复后重新编译并提交独立再审。默认执行 2 轮，确保论文质量持续提升。

## 前置条件

- **必需**: 论文 PDF + .tex 源文件（由 `paper-latex-writing` 产出）
- **必需**: 至少一份审计报告（由 `paper-review` 产出）：
  - `AUTO_REVIEW.md` — 综合审稿
  - `PAPER_CLAIM_AUDIT.md` — 数字核对
  - `CITATION_AUDIT.md` — 引用审核
  - `PROOF_AUDIT.md` — 证明验证（如适用）
  - `KILL_ARGUMENT.md` — 对抗性审查

## 操作步骤

### Step 1: 汇总所有审计问题

从所有审计报告中提取问题列表，按严重性排序：

```
🔴 CRITICAL — 必须修复，否则论文不可投稿
🟠 MAJOR    — 审稿人几乎肯定会要求修改
🟡 MINOR    — 改进后显著提升质量
```

生成 `IMPROVEMENT_PLAN.md`，每个问题包含：
- 来源审计报告
- 问题描述
- 严重性等级
- 建议修复方案
- 涉及的文件/章节

### Step 2: 按优先级实现修复

#### 常见修复模式

| 问题类型 | 修复模式 |
|---------|---------|
| 假设-模型不匹配 | 重写假设以匹配模型，添加连接两者的正式命题 |
| 过度声称 | 软化语言: "validate" → "demonstrate practical relevance" |
| 缺失指标 | 添加带诚实参数计数的量化表 |
| 定理不自包含 | 添加"Interpretation"段落列出所有依赖 |
| 符号混淆 | 全局重命名冲突符号，添加 Notation 段落 |
| 缺失引用 | 添加到 references.bib，在适当位置引用 |
| 理论-实践差距 | 明确将理论框架为理想化的；添加合成验证小节 |
| 证明缺口 | 运行 Audit 3（证明验证）修复 FATAL/CRITICAL 问题 |
| 写作冗余/被动语态 | 应用 5 遍科学写作审计（见 `paper-latex-writing`） |
| 数字不匹配 | 运行 Audit 1（数字核对）修复不匹配 |
| 关键词不一致 | Banana Rule: Methods 中的 "obese group" → Results 必须也是 "obese group" |
| 可视化问题 | 重新生成图表，确保灰度可区分、标签清晰 |
| 引用幻觉 | 删除不存在条目，通过 DBLP/CrossRef 查找替换 |

### Step 3: 重新编译 + 一致性测试

每轮修复完成后：
1. 重新编译 PDF（`latexmk -pdf`）
2. 运行定理重述回归测试（如有定理环境）：比较主文和附录中的定理/引理/命题/推论陈述是否一致
3. 运行格式检查：overfull hbox、重复标签、页面限制

### Step 4: 下一轮独立审稿

**关键原则：审稿人独立性**

- 每轮使用**全新**内置审稿上下文线程（本地处理，不外传）
- **绝不在审稿提示中包含"自上一轮以来"或"已修复了X"**
- 审稿人只能看到当前的 .tex 文件和 PDF
- 审稿人必须对之前的版本和修复**一无所知**

### Step 5: 对比评分

每轮结束后记录评分变化：

```markdown
| Round | Score | 关键改进 | 状态 |
|-------|-------|---------|------|
| Round 0 (初始) | 5.5/10 | — | 基线 |
| Round 1 | 6.5/10 | 修复数字不匹配、补充缺失引用 | ✅ |
| Round 2 | 7.5/10 | 重写过度声称、改善图表可读性 | ✅ |
```

### Step 6: 终止判据

以下条件满足其一即停止迭代：
1. 评分 ≥ 7.0（Accept）且无 CRITICAL/MAJOR 问题
2. 已完成最大轮数（默认 2 轮，最多 3 轮）
3. 两轮间评分提升 < 0.5（改进收益递减）

## 输出

```
paper/
├── main_round0_original.pdf      # 改进前原始版本
├── main_round1.pdf               # 第 1 轮改进后 PDF
├── main_round2.pdf               # 第 2 轮改进后 PDF（最终）
├── main.tex                      # 当前最新版 .tex
├── IMPROVEMENT_PLAN.md           # 改进计划
├── PAPER_IMPROVEMENT_LOG.md      # 完整改进日志
│   ├── Round 0 baseline
│   ├── Round 1: 问题列表 + 修复记录 + 再审结果
│   └── Round 2: 问题列表 + 修复记录 + 再审结果
└── FINAL_REPORT.md               # 终审报告
```

## 终审报告模板

```markdown
# Paper Improvement Pipeline Report

**Input**: [原始 paper/ 目录]
**Rounds**: 2
**Assurance**: submission

## Score Progression

| Round | Score | Verdict |
|-------|-------|---------|
| Round 0 | X/10 | Weak Reject |
| Round 1 | Y/10 | Borderline Accept |
| Round 2 | Z/10 | Accept |

## Changes Summary

### Round 1
- [X] CRITICAL: ...
- [X] MAJOR: ...
- [X] MINOR: ...

### Round 2
- [X] MAJOR: ...
- [X] MINOR: ...

## Final Audit Status

| Audit | Status |
|-------|--------|
| Paper Claim Audit | ✅ PASS |
| Citation Audit | ✅ PASS |
| Proof Checker | ✅ PASS / N/A |
| Kill Argument | ✅ PASS |
| Full Review | ✅ Score Z/10 |

## Submission Checklist
- [ ] All audits passed
- [ ] No overfull hboxes in main text
- [ ] All citations verified by DBLP/CrossRef
- [ ] PDF within page limit
- [ ] All figures clear in grayscale

**Submission-ready: ✅ YES**
```

## 与其他 Skill 的关系

- 接收 `paper-review` 的全部审计报告作为输入
- 修复过程中可能需要重新执行 `paper-figures`（改图）或 `paper-latex-writing`（重新编译）
- 是论文写作管道的最后一个阶段

## 注意事项

- **绝不摧毁原始版本**：保留 round0/round1/round2 的所有 PDF
- **审稿人独立性是核心**：每轮审稿人不知晓之前的问题和修复
- **修复而非重写**：保持最小修改原则，每次只修复审稿人指出的问题
- **优先级严格执行**：CRITICAL > MAJOR > MINOR，不可跳级
- **无法修复的问题**（如缺少实验数据）应诚实标注为 [PENDING_DATA]，不伪装修复
