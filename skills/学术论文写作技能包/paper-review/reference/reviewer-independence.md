# 审计独立性协议参考 (Reviewer Independence Protocol)

## 核心原则

所有审计（数字核对、引用审核、证明验证、对抗审查、综合审稿）必须遵循**严格独立性协议**，确保审稿人不被原始研究意图污染。

## 规则

### Rule 1: 零上下文审计
- 审稿人**仅接收**论文 .tex 文件和原始数据文件
- **不接收**论文摘要、作者意图说明、实验设计动机等任何形式的上下文解释
- 审稿人应像会议审稿人那样，仅从文本判断论文质量

### Rule 2: 全新线程
- 每项审计使用**独立的内置审稿上下文会话/线程**（本地处理，不外传）
- 不允许在同一会话中执行多个审计
- 不允许审稿人之间有任何信息交叉

### Rule 3: 无改进提示
- 审稿提示中**不得包含**:
  - "自上一轮以来..."
  - "已修复了以下问题..."
  - "请特别关注..."
  - 任何暗示论文已被修改过的语言

### Rule 4: 当前版本唯一证据
- 审稿人唯一可接受的改进证据是当前的 .tex 源文件和编译后的 PDF
- 不提供 diff、changelog、revision notes 等

### Rule 5: 代码计算裁决
- 审计的最终 PASS/FAIL 裁决由**技能代码逻辑计算**
- 不由审稿人 LLM 自评（避免 self-judgment bias）
- 审稿人 LLM 仅提供事实发现，裁决由代码汇总生成

## 实施示例

```python
# ✅ 正确: 零上下文审稿提示
review_prompt = f"""
You are a conference reviewer. Review the following paper:

{paper_tex_content}

Raw data files are attached for verification.
Identify any discrepancies between claimed numbers and raw data.
"""

# ❌ 错误: 污染了上下文
review_prompt = f"""
我们开发了一个新方法，在X任务上表现很好。
论文说达到了85.3%。请帮我们确认数字是否正确。
{paper_tex_content}
"""
```
