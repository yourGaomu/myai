#!/usr/bin/env python3
"""
arXiv 论文获取脚本
通过 arXiv API 获取论文元数据和 PDF
"""

import sys
import json
import re
import urllib.request
import urllib.parse
from xml.etree import ElementTree

def extract_arxiv_id(text):
    """从文本中提取 arXiv ID"""
    # 支持多种格式：
    # arXiv:2301.07001
    # https://arxiv.org/abs/2301.07001
    # https://arxiv.org/pdf/2301.07001
    # 2301.07001
    
    patterns = [
        r'arXiv[:\s]+(\d{4}\.\d{4,5})',
        r'arxiv\.org/(?:abs|pdf)/(\d{4}\.\d{4,5})',
        r'^(\d{4}\.\d{4,5})$'
    ]
    
    for pattern in patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            return match.group(1)
    
    return None

def fetch_arxiv_metadata(arxiv_id):
    """通过 arXiv API 获取论文元数据"""
    base_url = "http://export.arxiv.org/api/query?"
    query = f"id_list={arxiv_id}"
    
    try:
        with urllib.request.urlopen(base_url + query, timeout=30) as response:
            xml_data = response.read().decode('utf-8')
        
        # 解析 XML
        root = ElementTree.fromstring(xml_data)
        
        # 定义命名空间
        ns = {
            'atom': 'http://www.w3.org/2005/Atom',
            'arxiv': 'http://arxiv.org/schemas/atom'
        }
        
        entry = root.find('atom:entry', ns)
        if entry is None:
            return None
        
        # 提取元数据
        title = entry.find('atom:title', ns).text.strip()
        
        authors = []
        for author in entry.findall('atom:author', ns):
            name = author.find('atom:name', ns).text
            authors.append(name)
        
        summary = entry.find('atom:summary', ns).text.strip()
        
        published = entry.find('atom:published', ns).text[:10]  # YYYY-MM-DD
        
        # 提取分类（领域）
        categories = []
        for cat in entry.findall('atom:category', ns):
            term = cat.get('term')
            if term:
                categories.append(term)
        
        # PDF 链接
        pdf_url = None
        for link in entry.findall('atom:link', ns):
            if link.get('title') == 'pdf':
                pdf_url = link.get('href')
                break
        
        # 如果没有 PDF 链接，构造一个
        if not pdf_url:
            pdf_url = f"https://arxiv.org/pdf/{arxiv_id}.pdf"
        
        return {
            'arxiv_id': arxiv_id,
            'title': title,
            'authors': authors,
            'abstract': summary,
            'published': published,
            'categories': categories,
            'pdf_url': pdf_url,
            'abs_url': f"https://arxiv.org/abs/{arxiv_id}"
        }
        
    except Exception as e:
        print(f"Error fetching arXiv metadata: {e}", file=sys.stderr)
        return None

def main():
    if len(sys.argv) < 2:
        print("Usage: fetch_arxiv.py <arxiv_id_or_url>", file=sys.stderr)
        sys.exit(1)
    
    input_text = sys.argv[1]
    arxiv_id = extract_arxiv_id(input_text)
    
    if not arxiv_id:
        print(f"Error: Could not extract arXiv ID from '{input_text}'", file=sys.stderr)
        sys.exit(1)
    
    metadata = fetch_arxiv_metadata(arxiv_id)
    
    if metadata:
        print(json.dumps(metadata, indent=2, ensure_ascii=False))
    else:
        print(f"Error: Could not fetch metadata for arXiv:{arxiv_id}", file=sys.stderr)
        sys.exit(1)

if __name__ == '__main__':
    main()
