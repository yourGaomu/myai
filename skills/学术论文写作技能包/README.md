---
name: SKILL
description: "覆盖学术论文从规划到投稿的全流程。包含一系列独立 Skill，每个 Skill 都符合 SkillHub / Agent Skills 开放标准。可被兼容的 AI 客户端独立加载使用。"
---
# 学术论文写作全流程技能包

本技能包包含 **5 个独立 Skill**，覆盖学术论文从规划到投稿的全流程。每个 Skill 符合 [SkillHub / Agent Skills 开放标准](https://agentskills.io)，可被兼容的 AI 客户端独立加载使用。

## 📦 技能清单

| # | Skill | 名称 | 功能 | 阶段 |
|---|-------|------|------|------|
| 1 | `paper-planning` | 论文大纲规划 | 论断-证据矩阵、逐节规划、图表规划、引文脚手架 | 规划 |
| 2 | `paper-figures` | 论文配图与表格 | 数据图、架构图(JSON→SVG)、AI插图、Mermaid流程图 | 配图 |
| 3 | `paper-latex-writing` | LaTeX撰写与编译 | 逐节撰写、5遍科学写作审计、参考文献验证、PDF编译 | 写作 |
| 4 | `paper-review` | 多维度审稿与审计 | 数字核对、引用审核、证明验证、对抗审查、综合审稿 | 审稿 |
| 5 | `paper-improvement` | 审稿改稿循环 | 汇总审计问题 → 逐条修复 → 重编译 → 独立再审 (×2) | 改进 |

## 🔄 推荐协作流程

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

## 🚀 快速开始

### 方式 A: 全部加载

将整个 `学术论文写作技能包/` 目录导入你的 AI 客户端，5 个 Skill 将按需自动触发。

### 方式 B: 按需使用

按当前所处阶段加载对应 Skill：

```
# 规划阶段
只需加载 paper-planning/

# 写作阶段  
需要 paper-planning/ → paper-figures/ → paper-latex-writing/

# 审稿改稿阶段
需要 paper-review/ + paper-improvement/
```

## 📋 各 Skill 触发关键词

| Skill | 触发词 |
|-------|--------|
| paper-planning | "写大纲"、"论文规划"、"paper plan"、"论文结构" |
| paper-figures | "画图"、"论文配图"、"生成图表"、"paper figure"、"作图" |
| paper-latex-writing | "写论文"、"写LaTeX"、"draft LaTeX"、"编译论文"、"生成PDF" |
| paper-review | "审稿"、"review"、"数字核对"、"引用审核"、"证明验证" |
| paper-improvement | "改论文"、"论文改进"、"improve paper"、"根据审稿意见修改" |

## 🛠 设计理念

1. **单一职责**: 每个 Skill 只做一件事，做好一件事
2. **可独立使用**: 不强制依赖其他 Skill，可按需组合
3. **以审稿人为中心**: 所有审计使用零上下文独立审稿人，模拟真实peer review
4. **可追溯**: 保留 round0/round1/round2 全部版本，每步改进有据可查
5. **符合标准**: 每个 SKILL.md 符合 SkillHub 规范（YAML frontmatter + slug命名 + 触发关键词）

## 📖 参考来源

- 论断-证据矩阵方法: [Research-Paper-Writing-Skills](https://github.com/Master-cai/Research-Paper-Writing-Skills)
- 引用验证方法: 基于 DBLP/CrossRef 的文献存在性-元数据正确性-上下文适当性三重验证链条
- 科学写作审计: Sainani's "Writing in the Sciences" (Stanford)
- 审稿人独立性协议: ARIS `/research-review`, `/kill-argument`
- IEEE 配色方案: IEEE Transactions visualization guidelines
- 零上下文审计: ARIS `/paper-claim-audit`, `/citation-audit`
