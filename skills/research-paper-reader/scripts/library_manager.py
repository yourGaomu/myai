#!/usr/bin/env python3
"""
文献库管理脚本
管理已读论文的索引、检索和关联分析
"""

import json
import os
import sys
from datetime import datetime
from pathlib import Path
import hashlib

class PaperLibrary:
    """论文文献库管理"""
    
    def __init__(self, library_path=None):
        if library_path is None:
            library_path = os.path.expanduser("~/.openclaw/workspace/papers/library")
        self.library_path = Path(library_path)
        self.library_path.mkdir(parents=True, exist_ok=True)
        self.index_file = self.library_path / "index.json"
        self.load_index()
    
    def load_index(self):
        """加载文献索引"""
        if self.index_file.exists():
            with open(self.index_file, 'r', encoding='utf-8') as f:
                self.index = json.load(f)
        else:
            self.index = {
                "papers": {},
                "topics": {},
                "authors": {},
                "keywords": {},
                "created": datetime.now().isoformat(),
                "updated": datetime.now().isoformat()
            }
            self.save_index()
    
    def save_index(self):
        """保存文献索引"""
        self.index["updated"] = datetime.now().isoformat()
        with open(self.index_file, 'w', encoding='utf-8') as f:
            json.dump(self.index, f, indent=2, ensure_ascii=False)
    
    def generate_paper_id(self, paper_data):
        """生成论文唯一ID"""
        # 使用标题+年份生成ID
        title = paper_data.get('title', '')
        year = paper_data.get('year', '')
        authors = paper_data.get('authors', [])
        
        if title:
            base = f"{title}_{year}"
        else:
            base = f"paper_{datetime.now().timestamp()}"
        
        # 生成短hash
        hash_obj = hashlib.md5(base.encode())
        return f"paper_{hash_obj.hexdigest()[:8]}"
    
    def add_paper(self, paper_data, notes_path=None):
        """添加论文到文献库"""
        paper_id = self.generate_paper_id(paper_data)
        
        # 构建论文记录
        paper_record = {
            "id": paper_id,
            "title": paper_data.get('title', ''),
            "authors": paper_data.get('authors', []),
            "year": paper_data.get('year', ''),
            "venue": paper_data.get('venue', ''),
            "keywords": paper_data.get('keywords', []),
            "arxiv_id": paper_data.get('arxiv_id'),
            "doi": paper_data.get('doi'),
            "url": paper_data.get('url'),
            "summary": paper_data.get('summary', ''),
            "contributions": paper_data.get('contributions', []),
            "methods": paper_data.get('methods', ''),
            "results": paper_data.get('results', ''),
            "topics": paper_data.get('topics', []),
            "notes_path": str(notes_path) if notes_path else None,
            "added_date": datetime.now().isoformat(),
            "read_count": 0,
            "tags": paper_data.get('tags', []),
            "related_papers": []
        }
        
        # 添加到索引
        self.index["papers"][paper_id] = paper_record
        
        # 更新主题索引
        for topic in paper_data.get('topics', []):
            if topic not in self.index["topics"]:
                self.index["topics"][topic] = []
            self.index["topics"][topic].append(paper_id)
        
        # 更新作者索引
        for author in paper_data.get('authors', []):
            if author not in self.index["authors"]:
                self.index["authors"][author] = []
            self.index["authors"][author].append(paper_id)
        
        # 更新关键词索引
        for keyword in paper_data.get('keywords', []):
            keyword_lower = keyword.lower()
            if keyword_lower not in self.index["keywords"]:
                self.index["keywords"][keyword_lower] = []
            self.index["keywords"][keyword_lower].append(paper_id)
        
        self.save_index()
        return paper_id
    
    def get_paper(self, paper_id):
        """获取论文信息"""
        return self.index["papers"].get(paper_id)
    
    def find_by_title(self, title):
        """按标题搜索论文"""
        results = []
        title_lower = title.lower()
        for paper_id, paper in self.index["papers"].items():
            if title_lower in paper.get('title', '').lower():
                results.append(paper)
        return results
    
    def find_by_topic(self, topic):
        """按主题搜索论文"""
        paper_ids = self.index["topics"].get(topic, [])
        return [self.index["papers"][pid] for pid in paper_ids if pid in self.index["papers"]]
    
    def find_by_author(self, author):
        """按作者搜索论文"""
        paper_ids = self.index["authors"].get(author, [])
        return [self.index["papers"][pid] for pid in paper_ids if pid in self.index["papers"]]
    
    def find_by_keyword(self, keyword):
        """按关键词搜索论文"""
        keyword_lower = keyword.lower()
        paper_ids = self.index["keywords"].get(keyword_lower, [])
        return [self.index["papers"][pid] for pid in paper_ids if pid in self.index["papers"]]
    
    def find_related_papers(self, paper_id, max_results=10):
        """查找相关论文"""
        paper = self.get_paper(paper_id)
        if not paper:
            return []
        
        # 基于关键词、主题、作者计算相关性
        scores = {}
        
        # 关键词匹配
        for keyword in paper.get('keywords', []):
            for related_id in self.index["keywords"].get(keyword.lower(), []):
                if related_id != paper_id:
                    scores[related_id] = scores.get(related_id, 0) + 2
        
        # 主题匹配
        for topic in paper.get('topics', []):
            for related_id in self.index["topics"].get(topic, []):
                if related_id != paper_id:
                    scores[related_id] = scores.get(related_id, 0) + 3
        
        # 作者匹配
        for author in paper.get('authors', []):
            for related_id in self.index["authors"].get(author, []):
                if related_id != paper_id:
                    scores[related_id] = scores.get(related_id, 0) + 1
        
        # 排序并返回
        sorted_papers = sorted(scores.items(), key=lambda x: x[1], reverse=True)
        return [
            self.index["papers"][pid] 
            for pid, score in sorted_papers[:max_results] 
            if pid in self.index["papers"]
        ]
    
    def get_statistics(self):
        """获取文献库统计信息"""
        return {
            "total_papers": len(self.index["papers"]),
            "total_topics": len(self.index["topics"]),
            "total_authors": len(self.index["authors"]),
            "total_keywords": len(self.index["keywords"]),
            "created": self.index["created"],
            "updated": self.index["updated"]
        }
    
    def list_papers(self, sort_by="added_date", limit=None):
        """列出所有论文"""
        papers = list(self.index["papers"].values())
        
        # 排序
        if sort_by == "added_date":
            papers.sort(key=lambda x: x.get('added_date', ''), reverse=True)
        elif sort_by == "title":
            papers.sort(key=lambda x: x.get('title', '').lower())
        elif sort_by == "year":
            papers.sort(key=lambda x: x.get('year', ''), reverse=True)
        
        if limit:
            papers = papers[:limit]
        
        return papers
    
    def update_paper(self, paper_id, updates):
        """更新论文信息"""
        if paper_id not in self.index["papers"]:
            return False
        
        paper = self.index["papers"][paper_id]
        paper.update(updates)
        paper["updated_date"] = datetime.now().isoformat()
        
        self.save_index()
        return True
    
    def add_tag(self, paper_id, tag):
        """添加标签"""
        if paper_id not in self.index["papers"]:
            return False
        
        paper = self.index["papers"][paper_id]
        if tag not in paper.get('tags', []):
            paper['tags'].append(tag)
            self.save_index()
        
        return True
    
    def add_relation(self, paper_id1, paper_id2, relation_type="related"):
        """添加论文关系"""
        if paper_id1 not in self.index["papers"] or paper_id2 not in self.index["papers"]:
            return False
        
        paper1 = self.index["papers"][paper_id1]
        paper2 = self.index["papers"][paper_id2]
        
        # 双向关联
        if paper_id2 not in [r.get('id') for r in paper1.get('related_papers', [])]:
            paper1.setdefault('related_papers', []).append({
                "id": paper_id2,
                "type": relation_type
            })
        
        if paper_id1 not in [r.get('id') for r in paper2.get('related_papers', [])]:
            paper2.setdefault('related_papers', []).append({
                "id": paper_id1,
                "type": relation_type
            })
        
        self.save_index()
        return True
    
    def export_bibliography(self, format="bibtex"):
        """导出参考文献列表"""
        papers = list(self.index["papers"].values())
        
        if format == "bibtex":
            return self._export_bibtex(papers)
        elif format == "markdown":
            return self._export_markdown(papers)
        else:
            return None
    
    def _export_bibtex(self, papers):
        """导出BibTeX格式"""
        entries = []
        for paper in papers:
            authors = " and ".join(paper.get('authors', []))
            title = paper.get('title', '')
            year = paper.get('year', '')
            venue = paper.get('venue', '')
            
            # 生成citation key
            first_author = paper.get('authors', ['unknown'])[0].split()[-1].lower() if paper.get('authors') else 'unknown'
            cite_key = f"{first_author}{year}"
            
            entry = f"@article{{{cite_key},\n"
            entry += f"  title={{{title}}},\n"
            entry += f"  author={{{authors}}},\n"
            if year:
                entry += f"  year={{{year}}},\n"
            if venue:
                entry += f"  journal={{{venue}}},\n"
            entry += "}\n"
            
            entries.append(entry)
        
        return "\n".join(entries)
    
    def _export_markdown(self, papers):
        """导出Markdown格式"""
        lines = ["# 📚 我的文献库\n"]
        
        for paper in papers:
            lines.append(f"## {paper.get('title', 'Untitled')}\n")
            lines.append(f"- **作者**: {', '.join(paper.get('authors', []))}\n")
            lines.append(f"- **年份**: {paper.get('year', 'N/A')}\n")
            if paper.get('venue'):
                lines.append(f"- **发表**: {paper.get('venue')}\n")
            if paper.get('summary'):
                lines.append(f"- **摘要**: {paper.get('summary')}\n")
            lines.append("\n---\n")
        
        return "".join(lines)


def main():
    """命令行接口"""
    if len(sys.argv) < 2:
        print("Usage: library_manager.py <command> [args]")
        print("Commands: add, find, list, stats, export")
        sys.exit(1)
    
    command = sys.argv[1]
    library = PaperLibrary()
    
    if command == "stats":
        stats = library.get_statistics()
        print(json.dumps(stats, indent=2, ensure_ascii=False))
    
    elif command == "list":
        papers = library.list_papers()
        for paper in papers:
            print(f"- [{paper['id']}] {paper['title']} ({paper.get('year', 'N/A')})")
    
    elif command == "find":
        if len(sys.argv) < 4:
            print("Usage: library_manager.py find <field> <query>")
            sys.exit(1)
        
        field = sys.argv[2]
        query = sys.argv[3]
        
        if field == "title":
            results = library.find_by_title(query)
        elif field == "topic":
            results = library.find_by_topic(query)
        elif field == "author":
            results = library.find_by_author(query)
        elif field == "keyword":
            results = library.find_by_keyword(query)
        else:
            print(f"Unknown field: {field}")
            sys.exit(1)
        
        print(json.dumps(results, indent=2, ensure_ascii=False))
    
    elif command == "export":
        format = sys.argv[2] if len(sys.argv) > 2 else "bibtex"
        output = library.export_bibliography(format)
        print(output)
    
    else:
        print(f"Unknown command: {command}")
        sys.exit(1)


if __name__ == '__main__':
    main()
