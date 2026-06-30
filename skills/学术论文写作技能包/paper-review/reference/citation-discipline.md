# 引用规范 (Citation Discipline)

## 三步验证链条

所有引用必须经过以下验证后才可进入 references.bib：

### L1: 存在性验证
- **方法**: DBLP 搜索 (标题 + 第一作者) → 如失败 → CrossRef DOI 查询
- **检查**: arXiv ID 是否存在？DOI 是否解析？Venue 信息是否匹配？
- **失败处理**: 标记 [VERIFY]

### L2: 元数据验证
- **方法**: 字段级对比 (作者、标题、年份、venue、页码)
- **允许的差异**: 
  - 标题大小写风格差异 (允许)
  - 作者名 Unicode 差异 (如 "José" vs "Jose" — 统一使用权威来源)
- **不允许的差异**:
  - 年份不匹配
  - 完全不同的标题
  - 作者列表差异

### L3: 上下文适当性验证
- **方法**: 内置审稿能力在本地读取引用周围的句子，判断该引用是否真正支持其被用来支持的声称
- **指令**: "这篇被引用的论文是否确实支持 [CLAIM]？请读引用处的上下文"

## BibTeX 格式规范

### 键位格式
```
{第一作者}{年份}{关键词}
示例: he2024attention, vaswani2017transformer
```

### 必需字段
```
@article: author, title, journal, year
@inproceedings: author, title, booktitle, year
@misc (arXiv): author, title, year, eprint, archivePrefix
```

### 优先原则
- 已发表 venue 版本 > arXiv 预印本
- 有 DOI 的正式版本 > 无 DOI 的非正式版本
- 英文 venue 名称 > 缩写

## 禁止行为

- ❌ 从 LLM 记忆生成 BibTeX（极易产生幻觉引用）
- ❌ 包含未被 \cite{} 的条目在 references.bib
- ❌ 使用基于 arXiv ID 猜测的元数据
- ❌ 在引用链中跳过原始论文直接引用二手来源
