#!/usr/bin/env python3
"""
上下文管理脚本
管理论文阅读历史、生成上下文总结、追踪研究进展
"""

import json
import os
import sys
from datetime import datetime, timedelta
from pathlib import Path
from collections import defaultdict

class ReadingContext:
    """阅读上下文管理"""
    
    def __init__(self, context_path=None):
        if context_path is None:
            context_path = os.path.expanduser("~/.openclaw/workspace/papers")
        self.context_path = Path(context_path)
        self.context_path.mkdir(parents=True, exist_ok=True)
        
        self.history_file = self.context_path / "reading_history.json"
        self.session_file = self.context_path / "current_session.json"
        self.insights_file = self.context_path / "research_insights.md"
        
        self.load_history()
        self.load_session()
    
    def load_history(self):
        """加载阅读历史"""
        if self.history_file.exists():
            with open(self.history_file, 'r', encoding='utf-8') as f:
                self.history = json.load(f)
        else:
            self.history = {
                "sessions": [],
                "papers_read": [],
                "total_time_spent": 0,
                "topics_explored": defaultdict(int),
                "created": datetime.now().isoformat()
            }
    
    def save_history(self):
        """保存阅读历史"""
        with open(self.history_file, 'w', encoding='utf-8') as f:
            json.dump(self.history, f, indent=2, ensure_ascii=False)
    
    def load_session(self):
        """加载当前会话"""
        if self.session_file.exists():
            with open(self.session_file, 'r', encoding='utf-8') as f:
                self.session = json.load(f)
        else:
            self.start_new_session()
    
    def save_session(self):
        """保存当前会话"""
        with open(self.session_file, 'w', encoding='utf-8') as f:
            json.dump(self.session, f, indent=2, ensure_ascii=False)
    
    def start_new_session(self):
        """开始新的阅读会话"""
        self.session = {
            "session_id": datetime.now().strftime("%Y%m%d_%H%M%S"),
            "started_at": datetime.now().isoformat(),
            "papers": [],
            "notes": [],
            "questions": [],
            "insights": [],
            "goals": []
        }
        self.save_session()
    
    def add_paper_to_session(self, paper_id, paper_title, summary=None):
        """添加论文到当前会话"""
        paper_entry = {
            "paper_id": paper_id,
            "title": paper_title,
            "summary": summary,
            "read_at": datetime.now().isoformat(),
            "time_spent_minutes": 0
        }
        
        self.session["papers"].append(paper_entry)
        self.save_session()
        
        # 更新历史
        if paper_id not in self.history["papers_read"]:
            self.history["papers_read"].append(paper_id)
        
        self.save_history()
    
    def add_note(self, note, paper_id=None):
        """添加阅读笔记"""
        note_entry = {
            "content": note,
            "paper_id": paper_id,
            "created_at": datetime.now().isoformat()
        }
        self.session["notes"].append(note_entry)
        self.save_session()
    
    def add_question(self, question, paper_id=None):
        """添加研究问题"""
        question_entry = {
            "content": question,
            "paper_id": paper_id,
            "status": "open",
            "created_at": datetime.now().isoformat()
        }
        self.session["questions"].append(question_entry)
        self.save_session()
    
    def add_insight(self, insight, related_papers=None):
        """添加研究洞察"""
        insight_entry = {
            "content": insight,
            "related_papers": related_papers or [],
            "created_at": datetime.now().isoformat()
        }
        self.session["insights"].append(insight_entry)
        self.save_session()
    
    def set_goal(self, goal):
        """设置阅读目标"""
        self.session["goals"].append({
            "content": goal,
            "status": "active",
            "created_at": datetime.now().isoformat()
        })
        self.save_session()
    
    def end_session(self):
        """结束当前会话"""
        self.session["ended_at"] = datetime.now().isoformat()
        
        # 计算会话时长
        start = datetime.fromisoformat(self.session["started_at"])
        end = datetime.fromisoformat(self.session["ended_at"])
        duration = (end - start).total_seconds() / 60
        self.session["duration_minutes"] = duration
        
        # 保存到历史
        self.history["sessions"].append(self.session)
        self.history["total_time_spent"] += duration
        self.save_history()
        
        # 生成会话总结
        summary = self.generate_session_summary()
        
        # 开始新会话
        self.start_new_session()
        
        return summary
    
    def generate_session_summary(self):
        """生成会话总结"""
        summary = {
            "session_id": self.session["session_id"],
            "duration_minutes": self.session.get("duration_minutes", 0),
            "papers_read": len(self.session["papers"]),
            "papers": [p["title"] for p in self.session["papers"]],
            "notes_count": len(self.session["notes"]),
            "questions_count": len(self.session["questions"]),
            "insights_count": len(self.session["insights"])
        }
        return summary
    
    def get_recent_papers(self, days=7):
        """获取最近阅读的论文"""
        cutoff = datetime.now() - timedelta(days=days)
        recent = []
        
        for session in self.history["sessions"]:
            session_time = datetime.fromisoformat(session["started_at"])
            if session_time > cutoff:
                recent.extend(session.get("papers", []))
        
        return recent
    
    def get_reading_stats(self, period_days=30):
        """获取阅读统计"""
        cutoff = datetime.now() - timedelta(days=period_days)
        
        stats = {
            "period_days": period_days,
            "total_papers": 0,
            "total_time_minutes": 0,
            "papers_by_topic": defaultdict(int),
            "papers_by_day": defaultdict(int),
            "avg_papers_per_day": 0
        }
        
        for session in self.history["sessions"]:
            session_time = datetime.fromisoformat(session["started_at"])
            if session_time > cutoff:
                stats["total_papers"] += len(session.get("papers", []))
                stats["total_time_minutes"] += session.get("duration_minutes", 0)
                
                day = session_time.strftime("%Y-%m-%d")
                stats["papers_by_day"][day] += len(session.get("papers", []))
        
        if period_days > 0:
            stats["avg_papers_per_day"] = stats["total_papers"] / period_days
        
        return stats
    
    def generate_context_summary(self):
        """生成上下文总结"""
        summary_parts = []
        
        # 最近阅读
        recent = self.get_recent_papers(days=7)
        if recent:
            summary_parts.append("## 📚 最近阅读的论文\n")
            for paper in recent[:5]:
                summary_parts.append(f"- {paper['title']}\n")
        
        # 当前会话
        if self.session["papers"]:
            summary_parts.append("\n## 📖 当前会话\n")
            summary_parts.append(f"已阅读 {len(self.session['papers'])} 篇论文\n")
            for paper in self.session["papers"]:
                summary_parts.append(f"- {paper['title']}\n")
        
        # 待解决问题
        open_questions = [q for q in self.session.get("questions", []) if q["status"] == "open"]
        if open_questions:
            summary_parts.append("\n## ❓ 待解决问题\n")
            for q in open_questions[:5]:
                summary_parts.append(f"- {q['content']}\n")
        
        # 研究洞察
        if self.session.get("insights"):
            summary_parts.append("\n## 💡 研究洞察\n")
            for insight in self.session["insights"][-3:]:
                summary_parts.append(f"- {insight['content']}\n")
        
        return "".join(summary_parts)
    
    def export_insights(self):
        """导出研究洞察"""
        insights_path = self.context_path / "research_insights.md"
        
        with open(insights_path, 'w', encoding='utf-8') as f:
            f.write("# 💡 研究洞察与思考\n\n")
            f.write(f"更新时间: {datetime.now().strftime('%Y-%m-%d %H:%M')}\n\n")
            
            # 按会话组织
            for session in reversed(self.history["sessions"][-10:]):
                if session.get("insights"):
                    session_time = datetime.fromisoformat(session["started_at"])
                    f.write(f"## {session_time.strftime('%Y-%m-%d')}\n\n")
                    
                    for insight in session["insights"]:
                        f.write(f"- {insight['content']}\n")
                        if insight.get("related_papers"):
                            f.write(f"  相关论文: {', '.join(insight['related_papers'])}\n")
                    f.write("\n")
        
        return str(insights_path)


def main():
    """命令行接口"""
    if len(sys.argv) < 2:
        print("Usage: context_manager.py <command> [args]")
        print("Commands: summary, stats, export, recent")
        sys.exit(1)
    
    command = sys.argv[1]
    ctx = ReadingContext()
    
    if command == "summary":
        summary = ctx.generate_context_summary()
        print(summary)
    
    elif command == "stats":
        days = int(sys.argv[2]) if len(sys.argv) > 2 else 30
        stats = ctx.get_reading_stats(days)
        print(json.dumps(stats, indent=2, ensure_ascii=False))
    
    elif command == "recent":
        days = int(sys.argv[2]) if len(sys.argv) > 2 else 7
        papers = ctx.get_recent_papers(days)
        print(json.dumps(papers, indent=2, ensure_ascii=False))
    
    elif command == "export":
        path = ctx.export_insights()
        print(f"Insights exported to: {path}")
    
    else:
        print(f"Unknown command: {command}")
        sys.exit(1)


if __name__ == '__main__':
    main()
