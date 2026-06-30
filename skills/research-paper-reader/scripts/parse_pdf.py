#!/usr/bin/env python3
"""
PDF 解析脚本
提取 PDF 文件中的文本内容和结构
"""

import sys
import json
import os
import re

def parse_pdf_simple(pdf_path):
    """
    简单的 PDF 解析（使用 pdftotext）
    如果没有安装 pdftotext，返回 None
    """
    try:
        import subprocess
        
        # 检查 pdftotext 是否可用
        result = subprocess.run(['which', 'pdftotext'], capture_output=True)
        if result.returncode != 0:
            return None
        
        # 提取文本
        result = subprocess.run(
            ['pdftotext', '-layout', pdf_path, '-'],
            capture_output=True,
            text=True,
            timeout=60
        )
        
        if result.returncode != 0:
            return None
        
        text = result.stdout
        
        # 提取元数据
        meta_result = subprocess.run(
            ['pdfinfo', pdf_path],
            capture_output=True,
            text=True
        )
        
        metadata = {}
        if meta_result.returncode == 0:
            for line in meta_result.stdout.split('\n'):
                if ':' in line:
                    key, value = line.split(':', 1)
                    metadata[key.strip().lower().replace(' ', '_')] = value.strip()
        
        return {
            'text': text,
            'metadata': metadata,
            'method': 'pdftotext'
        }
        
    except Exception as e:
        print(f"Error with pdftotext: {e}", file=sys.stderr)
        return None

def parse_pdf_pypdf2(pdf_path):
    """
    使用 PyPDF2 解析 PDF
    需要安装：pip install PyPDF2
    """
    try:
        import PyPDF2
        
        with open(pdf_path, 'rb') as f:
            reader = PyPDF2.PdfReader(f)
            
            # 提取文本
            text_parts = []
            for page in reader.pages:
                text_parts.append(page.extract_text())
            
            text = '\n\n'.join(text_parts)
            
            # 提取元数据
            metadata = {}
            if reader.metadata:
                if reader.metadata.title:
                    metadata['title'] = reader.metadata.title
                if reader.metadata.author:
                    metadata['author'] = reader.metadata.author
                if reader.metadata.subject:
                    metadata['subject'] = reader.metadata.subject
            
            return {
                'text': text,
                'metadata': metadata,
                'num_pages': len(reader.pages),
                'method': 'PyPDF2'
            }
            
    except ImportError:
        return None
    except Exception as e:
        print(f"Error with PyPDF2: {e}", file=sys.stderr)
        return None

def parse_pdf_pdfplumber(pdf_path):
    """
    使用 pdfplumber 解析 PDF（更准确）
    需要安装：pip install pdfplumber
    """
    try:
        import pdfplumber
        
        text_parts = []
        metadata = {}
        
        with pdfplumber.open(pdf_path) as pdf:
            metadata['num_pages'] = len(pdf.pages)
            
            if pdf.metadata:
                metadata.update(pdf.metadata)
            
            for page in pdf.pages:
                text = page.extract_text()
                if text:
                    text_parts.append(text)
        
        return {
            'text': '\n\n'.join(text_parts),
            'metadata': metadata,
            'method': 'pdfplumber'
        }
        
    except ImportError:
        return None
    except Exception as e:
        print(f"Error with pdfplumber: {e}", file=sys.stderr)
        return None

def extract_structure(text):
    """提取论文结构"""
    structure = {
        'abstract': None,
        'introduction': None,
        'methods': None,
        'results': None,
        'discussion': None,
        'conclusion': None,
        'references': None
    }
    
    # 常见章节标题模式
    section_patterns = {
        'abstract': [
            r'abstract\s*\n',
            r'摘要\s*\n'
        ],
        'introduction': [
            r'1\.?\s+introduction\s*\n',
            r'1\.?\s+引言\s*\n',
            r'introduction\s*\n'
        ],
        'methods': [
            r'\d+\.?\s+method(s)?\s*\n',
            r'\d+\.?\s+methodology\s*\n',
            r'\d+\.?\s+方法\s*\n'
        ],
        'results': [
            r'\d+\.?\s+results?\s*\n',
            r'\d+\.?\s+experiments?\s*\n',
            r'\d+\.?\s+结果\s*\n'
        ],
        'discussion': [
            r'\d+\.?\s+discussion\s*\n',
            r'\d+\.?\s+讨论\s*\n'
        ],
        'conclusion': [
            r'\d+\.?\s+conclusions?\s*\n',
            r'\d+\.?\s+结论\s*\n'
        ],
        'references': [
            r'references\s*\n',
            r'bibliography\s*\n',
            r'参考文献\s*\n'
        ]
    }
    
    text_lower = text.lower()
    
    for section, patterns in section_patterns.items():
        for pattern in patterns:
            match = re.search(pattern, text_lower)
            if match:
                structure[section] = match.start()
                break
    
    return structure

def main():
    if len(sys.argv) < 2:
        print("Usage: parse_pdf.py <pdf_file>", file=sys.stderr)
        sys.exit(1)
    
    pdf_path = sys.argv[1]
    
    if not os.path.exists(pdf_path):
        print(f"Error: File not found: {pdf_path}", file=sys.stderr)
        sys.exit(1)
    
    # 尝试不同的解析方法
    result = None
    
    # 优先级：pdfplumber > PyPDF2 > pdftotext
    for parser in [parse_pdf_pdfplumber, parse_pdf_pypdf2, parse_pdf_simple]:
        result = parser(pdf_path)
        if result and result.get('text'):
            break
    
    if not result or not result.get('text'):
        print("Error: Could not extract text from PDF", file=sys.stderr)
        sys.exit(1)
    
    # 提取结构
    result['structure'] = extract_structure(result['text'])
    
    # 统计信息
    result['stats'] = {
        'char_count': len(result['text']),
        'word_count': len(result['text'].split())
    }
    
    print(json.dumps(result, indent=2, ensure_ascii=False))

if __name__ == '__main__':
    main()
