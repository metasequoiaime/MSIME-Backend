#!/usr/bin/env python3
"""下载并校验固定的公共词库；不覆盖已有文件或读取客户端用户词库。"""
import argparse
import hashlib
import json
from pathlib import Path
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]

def download(destination):
    lock = json.loads((ROOT/'native/resources.lock.json').read_text())
    destination.mkdir(parents=True, exist_ok=True)
    for name, digest in lock['assets'].items():
        target = destination/name
        if target.exists():
            if hashlib.sha256(target.read_bytes()).hexdigest() != digest:
                raise SystemExit(f'已有资源摘要不匹配：{name}；请选择新的资源目录')
            continue
        url = f'https://github.com/{lock["repository"]}/releases/download/{lock["tag"]}/{name}'
        request = urllib.request.Request(url, headers={'User-Agent': 'MSIME-Backend-resource-fetch'})
        with tempfile.NamedTemporaryFile(dir=destination, prefix='.download-', delete=False) as output:
            temporary = Path(output.name)
            try:
                with urllib.request.urlopen(request, timeout=60) as response:
                    digest_state = hashlib.sha256()
                    size = 0
                    while chunk := response.read(1024*1024):
                        size += len(chunk)
                        if size > 256*1024*1024:
                            raise ValueError('resource too large')
                        digest_state.update(chunk)
                        output.write(chunk)
                output.close()
                if digest_state.hexdigest() != digest:
                    raise ValueError(f'资源摘要不匹配：{name}')
                temporary.replace(target)
            finally:
                temporary.unlink(missing_ok=True)
    source_root = ROOT/'third_party/MSIME-Engine'
    for name, entry in lock.get('source_files', {}).items():
        source = source_root/entry['path']
        raw = source.read_bytes()
        if hashlib.sha256(raw).hexdigest() != entry['sha256']:
            raise SystemExit(f'固定源码资源摘要不匹配：{name}')
        target = destination/name
        target.parent.mkdir(parents=True, exist_ok=True)
        if target.exists():
            if target.read_bytes() != raw:
                raise SystemExit(f'已有源码资源不匹配：{name}；请选择新的资源目录')
        else:
            target.write_bytes(raw)
    manifest = json.loads((destination/'dictionary-manifest.json').read_text())
    if manifest['source']['commit'] != lock['source_commit'] or manifest['format_version'] != 1:
        raise SystemExit('词库来源或格式不匹配')
    print('固定词库来源与 SHA-256 校验通过')

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('destination', type=Path)
    download(parser.parse_args().destination)
