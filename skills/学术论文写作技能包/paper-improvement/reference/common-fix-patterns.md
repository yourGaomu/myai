# 常见修复模式速查表

本文档列出审稿→改稿循环中的常见问题及对应修复模式，供 `paper-improvement` Skill 参考。

## 方法论问题

| 问题 | 典型审稿人意见 | 修复模式 |
|------|-------------|---------|
| 假设-模型不匹配 | "The assumptions in Theorem 1 do not match the practical setting" | 重写假设以匹配实际模型；或添加 connecting proposition 连接理想化假设和实际条件 |
| 理论-实践差距 | "The theory assumes idealized conditions that never hold" | 明确框架为理想化分析；如果可能，添加合成验证实验展示差距大小 |
| 缺失基线 | "Missing comparison against [BASELINE]" | 如数据可获取，补充实验；如不可获取，在 Limitations 中诚实说明 |
| 过度声称 | "The claims exceed what the experiments support" | 软化语言，添加范围限定：如 "on our benchmark suite" 而非 "universally" |
| 缺失指标 | "No parameter count / FLOPs / runtime comparison" | 添加带诚实参数的量化对比表 |

## 写作问题

| 问题 | 典型审稿人意见 | 修复模式 |
|------|-------------|---------|
| 写作冗余 | "The paper is hard to follow" | 5遍审计：去冗余 → 主动语态 → 拆分长句 → Banana Rule → 数字检查 |
| 符号混淆 | "Notation is inconsistent" | 全局重命名冲突符号；添加 Notation 段落 |
| 关键词不一致 | "Same concept called different names" | Banana Rule: 全文统一术语 |
| AI-痕迹词 | 出现 delve, pivotal, landscape... | 全文搜索替换 |
| 路标缺失 | "Unclear how sections connect" | 在每节开头添加 1 句路标 |

## 证据问题

| 问题 | 典型审稿人意见 | 修复模式 |
|------|-------------|---------|
| 数字不匹配 | "85.3% claimed but raw data shows 84.7%" | 用原始数据重新计算，修正论文数字 |
| 最佳种子选择 | "Results appear cherry-picked" | 报告 mean ± std 而非最佳种子 |
| 聚合不匹配 | "Claims 5 seeds but only 3 runs" | 补充实验或修正声明 |
| 增量计算错误 | "'15% improvement' is actually 16.7%" | 重新计算精确增量，修正论文 |
| 范围过度声称 | "'Consistently outperforms' on only 2 datasets" | 限定范围："on both tested benchmarks" |

## 引用问题

| 问题 | 典型审稿人意见 | 修复模式 |
|------|-------------|---------|
| 引用幻觉 | "Cannot find reference [X]" | 通过 DBLP/CrossRef 验证，删除不存在条目 |
| 元数据错误 | "Wrong year/author for [X]" | 用权威来源 (DBLP/CrossRef) 的 BibTeX 替换 |
| 上下文不当 | "[X] does not actually support the claim" | 替换为确实支持该声称的引用 |
| 缺失关键引用 | "Missing citation to [KNOWN WORK]" | 在适当位置添加引用 |

## 可视化问题

| 问题 | 典型审稿人意见 | 修复模式 |
|------|-------------|---------|
| 灰度不可区分 | "Cannot distinguish curves in grayscale" | 添加不同线型 (solid, dashed, dotted) + 标记形状 |
| 标签太小 | "Axis labels unreadable" | 字号 ≥ 7pt |
| 标题-图不匹配 | "Caption describes something not in the figure" | 修正标题以匹配图内容 |
| 配色不一致 | "Different color scheme across figures" | 统一使用 IEEE 配色方案 |
