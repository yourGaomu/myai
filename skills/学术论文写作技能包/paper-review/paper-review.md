---
name: paper-review
description: "学术论文多维度审稿与审计。当用户说'审稿'、'论文审稿'、'review'、'paper audit'、'数字核对'、'引用审核'、'证明验证'、'对抗审稿'时使用。涵盖数字核对、引用三重验证、数学证明验证、对抗性审查和综合审稿五大审计维度。"
---

# 学术论文多维度审稿与审计 (Paper Review & Audit)

## 触发条件

用户提到以下任意场景时使用本 Skill：
- "审稿"、"论文审稿"、"review my paper"、"帮我审稿"
- "数字核对"、"核查数字"、"paper claim audit"、"数字是否准确"
- "引用审核"、"citation audit"、"检查引用"、"参考文献核对"
- "证明验证"、"proof checker"、"验证证明"、"proof audit"
- "对抗审稿"、"kill argument"、"拒稿理由"

## 概述

对论文进行五个维度的独立审计，模拟真实审稿人的严苛审查。每个审计维度使用**零上下文**的独立内置审稿能力，确保审查结果不受原始研究意图的影响。

## 五大审计维度

```
Audit 1: 数字核对 (Paper Claim Audit)
  → 论文中每个数字与原始数据是否一致？
  
Audit 2: 引用审核 (Citation Audit)  
  → 每条引用是否真实存在？元数据是否正确？上下文是否恰当？
  
Audit 3: 证明验证 (Proof Checker)
  → 数学定理的每个证明步骤是否严密？假设是否都被使用？
  
Audit 4: 对抗性审查 (Kill Argument)
  → 双线程攻防：攻击者构造最强制拒稿理由 → 辩护者逐条回应
  
Audit 5: 综合审稿 (Full Review)
  → 模拟会议审稿人的完整结构化评审
```

---

## Audit 1: 数字核对 (Paper Claim Audit)

### 核心原则：零上下文

审稿人仅提供论文 .tex 文件和原始数据文件，**不提供任何执行者摘要或解释**。这确保审稿人像真实读者一样独立判断。

### 检查的 7 种失败模式

| # | 失败模式 | 示例 |
|---|---------|------|
| 1 | **数字夸大** | 论文说 85.3%，原始数据是 84.7% |
| 2 | **最佳种子选择** | 报告最佳种子而非平均值 |
| 3 | **配置不匹配** | 对比的方法使用了不同超参数 |
| 4 | **聚合不匹配** | 声称"5 个种子平均值"但只有 3 次运行 |
| 5 | **增量错误** | "提高 15%"但实际增量是 16.7% |
| 6 | **标题-表格不匹配** | 图标题描述与实际内容不符 |
| 7 | **范围过度声称** | "持续优于"但只在 2 个数据集上测试 |

### 操作步骤

1. 将论文 .tex 文件和原始数据 JSON/CSV 提交给内置审稿能力进行本地审阅（数据不外传）
2. 审稿提示中**不包含**任何关于论文意图的解释
3. 审稿人独立提取论文中所有数字声明，逐一与原始数据对比
4. 输出裁决: `PASS | WARN | FAIL | NOT_APPLICABLE`
5. 涉及论文原文和原始数据时，需获得用户确认后方可进行审稿

### 输出

- `PAPER_CLAIM_AUDIT.md` — 逐条检查报告
- `PAPER_CLAIM_AUDIT.json` — 结构化裁决数据

---

## Audit 2: 引用审核 (Citation Audit)

### 三层独立验证

| 层 | 验证内容 | 方法 |
|----|---------|------|
| **L1: 存在性** | 引用论文在声称的 arXiv ID/DOI/venue 处确实存在 | arXiv API + DBLP + CrossRef |
| **L2: 元数据正确性** | 作者名、年份、venue、标题与权威来源匹配 | 字段级对比 |
| **L3: 上下文适当性** | 引用的论文确实支持它在文中被用来支持的声称 | 内置审稿能力（本地验证） |

### 每条目裁决

```
KEEP    — 引用存在、元数据正确、上下文恰当
FIX     — 元数据需修正（如年份/作者拼写错误）
REPLACE — 上下文不匹配，需替换为更合适的引用
REMOVE  — 引用不存在（幻觉引用），必须删除
```

### 操作步骤

1. 提取论文中所有 `\cite{}` 命令，去重后得引用列表
2. 逐条通过 DBLP + CrossRef API 验证存在性和元数据
3. 使用内置审稿能力在本地验证引用条目及其上下文的 L3 适当性
4. 生成修复建议

### 输出

- `CITATION_AUDIT.md` — 逐条引用审计报告
- `CITATION_AUDIT.json` — 结构化审计数据

---

## Audit 3: 证明验证 (Proof Checker)

仅适用于包含 ≥5 个 `\begin{theorem/lemma/proposition/corollary}` 环境的理论论文。

### 检查内容

1. **假设使用检查**: 每个假设在证明中是否被实际使用？
2. **量词错误**: `∀` 和 `∃` 的顺序是否正确？
3. **缺失的支配条件**: 是否有未声明的隐含条件？
4. **关键引理的反例尝试**: 尝试构造反例
5. **主文与附录一致性**: 定理陈述在正文和附录中是否一致？

### 输出

- `PROOF_AUDIT.md` — 逐定理检查报告
- `PROOF_AUDIT.json` — 结构化检查数据
- 严重性分级: `FATAL > CRITICAL > MAJOR > MINOR`
- FATAL/CRITICAL 问题必须在继续前修复

---

## Audit 4: 对抗性审查 (Kill Argument)

### 双线程对抗审查

**Thread 1 (攻击者)**: 使用全新内置审稿上下文构造最强约 200 词拒稿备忘录（本地处理，不外传），聚焦：
1. 中心定理是否按陈述真正被证明？
2. 假设-声称不匹配：正文是否退回比标题/摘要更窄的对象？
3. 缺失的证明义务：标题依赖于未证明的基础引理？
4. 极限顺序模糊：K/n/d/eps 的极限组合是否论文未承诺？
5. 声称-证据缺口：经验/数值证据是否太窄？
6. 范围过度声称：标题/摘要是否推销比正文更广的结果？

**Thread 2 (辩护者)**: 第二个新鲜审稿人逐条辩护，分类为：
- `answered_by_current_text` — 正文已回应
- `partially_answered` — 部分回应
- `still_unresolved` — 仍未解决

### 裁决

```
PASS — 0 unresolved, 所有 partially_answered 都是 minor
FAIL — ≥1 still_unresolved at critical level
```

### 输出

- `KILL_ARGUMENT.md` — 攻击/辩护逐条记录
- `KILL_ARGUMENT.json` — 结构化裁决

---

## Audit 5: 综合审稿 (Full Review)

### 审稿人指令

使用内置审稿能力对完整论文 .tex 和 PDF 进行本地结构化评审（论文内容不外传）：

```markdown
1. Overall Score (1-10, 6=weak accept, 7=accept)
2. Summary (2-3 sentences)
3. Strengths (ranked list)
4. Weaknesses (ranked by severity: CRITICAL > MAJOR > MINOR)
5. For each CRITICAL/MAJOR weakness: specific actionable fix
6. Missing references
7. Visual audit (from PDF):
   - Figure quality: readable? labels clear? distinguishable in grayscale?
   - Figure-caption alignment: each caption matches its figure?
   - Layout: orphan captions, strange page breaks, figures far from citation?
   - Table formatting: aligned columns, consistent decimals, bolded best?
   - Visual consistency: same color scheme across all figures?
8. Verdict: Ready for submission? Yes / Almost / No
```

### 输出

- `AUTO_REVIEW.md` — 完整审稿报告
- `AUTO_REVIEW.json` — 结构化评审数据

---

## 审计独立性协议（贯穿所有 Audit）

1. **每项 Audit 使用全新内置审稿上下文线程**（本地处理，不外传），不共享上下文
2. **审稿人不对先前版本或修复有任何了解**
3. **审稿提示中不包含"自上一轮以来"等时间顺序暗示**
4. **唯一可接受的改进证据是当前的 .tex 文件和 PDF**
5. **裁决由技能代码计算**，不由审稿人自评（避免 self-judgment bias）

## 与其他 Skill 的关系

- 接收 `paper-latex-writing` 产出的 PDF + .tex 文件作为输入
- 产出的审计报告供 `paper-improvement` 驱动修改
- Audit 2（引用审核）需要 `paper-latex-writing` 的 `references.bib`
- 可独立使用：无需依赖其他 Skill，只需提供论文 PDF/.tex

## 注意事项

- 零上下文审计是核心原则：审稿人看不到你的意图，只看到你的文字
- 每项 Audit 产生的修复建议需记录在案，便于 `paper-improvement` 追踪
- FATAL/CRITICAL 问题必须立即修复才能继续
- 引用审核需要外部 API（DBLP/CrossRef），如 API 不可用则标记为 [VERIFY]
- 证明验证仅适用于理论论文，纯实验论文跳过 Audit 3
