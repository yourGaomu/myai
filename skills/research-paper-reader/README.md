# Research Paper Reader - 论文研读总结助手

一个功能强大的学术论文研读和管理技能，支持文献库管理、上下文记忆和智能推荐。

## ✨ 核心特性

### 📄 论文研读
- **多格式支持**：PDF文件、arXiv链接、DOI、论文URL
- **智能提取**：自动识别论文结构（摘要、方法、结果、结论）
- **结构化总结**：生成清晰的论文总结报告
- **深度分析**：方法论评估、实验设计分析
- **批判性思考**：识别论文的优缺点、局限性和未来方向

### 📚 文献库管理
- **自动索引**：阅读过的论文自动添加到文献库
- **多维检索**：按标题、作者、主题、关键词搜索
- **分类管理**：按主题、作者、日期自动分类
- **标签系统**：自定义标签，灵活组织

### 🔗 上下文记忆
- **阅读历史**：追踪所有阅读记录
- **研究进展**：自动生成进展报告
- **洞察积累**：记录研究想法和发现
- **会话管理**：按会话组织阅读活动

### 🎯 智能推荐
- **相关论文**：基于关键词、主题、作者推荐
- **引用网络**：构建论文引用关系
- **研究趋势**：分析领域发展趋势

## 🚀 快速开始

### 1. 论文研读

```
用户：请帮我读一下这篇论文 [上传PDF]
用户：总结 arXiv:2301.07001
用户：分析这篇论文：10.1038/s41586-019-1234-5
```

### 2. 文献库管理

```
用户：查看我的文献库
用户：搜索关于"transformer"的论文
用户：找出"Yann LeCun"的论文
```

### 3. 研究进展

```
用户：总结我最近的研究进展
用户：查看阅读历史
用户：生成研究洞察报告
```

### 4. 相关推荐

```
用户：推荐与这篇论文相关的论文
用户：找出类似方法的论文
```

## 📦 安装

```bash
# 从 SkillHub 安装
npx clawhub install research-paper-reader --registry https://skill.xfyun.cn

# 或从本地安装
git clone https://github.com/openclaw/skills
cd skills/paper-reader
```

## 📖 文档

- [SKILL.md](./SKILL.md) - 完整技能说明
- [EXAMPLES.md](./EXAMPLES.md) - 详细使用示例
- [USAGE.md](./USAGE.md) - 使用指南

## 🗂️ 数据结构

```
~/.openclaw/workspace/papers/
├── library/
│   ├── index.json              # 主索引
│   ├── papers/                 # 论文详情
│   └── notes/                  # 研读笔记
├── by-topic/                   # 按主题分类
├── by-author/                  # 按作者分类
├── by-date/                    # 按日期分类
├── reading_history.json        # 阅读历史
├── current_session.json        # 当前会话
└── research_insights.md        # 研究洞察
```

## ⚙️ 配置

编辑 `config.json` 自定义行为：

```json
{
  "output_language": "zh-CN",
  "detail_level": "comprehensive",
  "auto_index": true,
  "max_related_papers": 10,
  "context_days": 30,
  "enable_recommendations": true
}
```

## 🛠️ 技术实现

### 核心脚本

- `fetch_arxiv.py` - arXiv 论文获取
- `parse_pdf.py` - PDF 文件解析
- `library_manager.py` - 文献库管理
- `context_manager.py` - 上下文管理

### 依赖

**必需：**
- Python 3.6+

**可选：**
- PyPDF2 - PDF解析
- pdfplumber - 更准确的PDF解析（推荐）

```bash
pip install PyPDF2 pdfplumber
```

## 💡 使用场景

### 学术研究
- 快速了解论文核心内容
- 批量研读相关文献
- 追踪研究进展

### 文献综述
- 管理大量参考文献
- 分析研究趋势
- 生成综述报告

### 方法复现
- 理解技术细节
- 对比不同方法
- 记录实现要点

### 论文写作
- 学习优秀论文结构
- 管理引用文献
- 生成BibTeX

## 🎯 特色功能

### 1. 自动索引
阅读过的论文自动添加到文献库，无需手动管理。

### 2. 智能推荐
基于关键词、主题、作者自动推荐相关论文。

### 3. 进展追踪
自动统计阅读数量、时间投入、主题分布。

### 4. 上下文记忆
记住你的阅读历史和研究洞察，提供连贯的研究支持。

### 5. 多格式导出
支持BibTeX、Markdown、JSON等多种导出格式。

## 📊 示例输出

### 论文研读报告
```markdown
# 📄 论文研读报告

## 基本信息
- 标题：Attention Is All You Need
- 作者：Ashish Vaswani, et al.
- 发表：NeurIPS 2017
- 文献库ID：paper_abc123

## 📋 一句话总结
提出了Transformer架构，完全基于注意力机制。

## 💡 主要贡献
1. 提出了Transformer架构
2. 引入了自注意力机制
3. 实现了并行化训练

## 🔗 相关论文
1. BERT - 相关度：高
2. GPT-3 - 相关度：高
```

### 文献库报告
```markdown
# 📚 我的文献库

## 统计信息
- 📄 总论文数：25篇
- 📁 主题数：8个
- 👥 作者数：45位

## 最近添加
1. Attention Is All You Need - 2026-04-12
2. BERT - 2026-04-11
```

### 研究进展报告
```markdown
# 📊 研究进展报告

## 阅读统计（30天）
- 📄 阅读论文：18篇
- ⏱️ 投入时间：12小时
- 📈 日均阅读：0.6篇

## 关键发现
1. Transformer架构已成为主流
2. 预训练+微调范式效果显著
```

## 🔒 隐私与安全

- ✅ 所有数据存储在本地
- ✅ 不上传论文内容到云端
- ✅ 完全私密的研究助手
- ✅ 支持数据导出和备份

## 📝 更新日志

### v2.0.0 (2026-04-12)
- ✨ 新增文献库管理功能
- ✨ 新增上下文记忆机制
- ✨ 新增相关论文推荐
- ✨ 新增研究进展追踪
- ✨ 新增阅读统计报告
- ✨ 新增文献导出功能

### v1.0.0 (2026-04-10)
- 初始版本
- 支持 PDF、arXiv、DOI 格式
- 结构化研读报告生成

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 🔗 相关链接

- [SkillHub](https://skill.xfyun.cn)
- [OpenClaw](https://openclaw.ai)
- [文档](https://docs.openclaw.ai)

---

**让论文研读更高效，让研究更有条理！** 📚✨
