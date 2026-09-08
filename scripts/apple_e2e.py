#!/usr/bin/env python3
"""编译实际 Apple NSURLSession 客户端，以进程内测试信任验证 Go TLS 接口。"""
import os
from pathlib import Path
import platform
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
if platform.system() != 'Darwin':
    raise SystemExit('Apple 网络集成测试需要 macOS 和 Xcode 命令行工具')
bridge = ROOT.parent / 'MSIME-Apple/shared/apple-bridge'
with tempfile.TemporaryDirectory(prefix='msime-apple-network-') as directory:
    binary = Path(directory) / 'apple-client'
    subprocess.run(['xcrun', 'clang', '-fobjc-arc', '-Wall', '-Wextra', '-Werror',
                    '-framework', 'Foundation', '-framework', 'Security', '-I', str(bridge),
                    str(ROOT / 'tests/native_apple_client.m'), str(bridge / 'MSIMEBackendClient.m'),
                    '-o', str(binary)], check=True)
    subprocess.run(['go', 'test', './internal/server', '-run', '^TestNativeAppleNetwork$', '-v', '-count=1'],
                   cwd=ROOT, env=dict(os.environ, MSIME_NATIVE_APPLE=str(binary)), check=True)
