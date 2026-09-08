#!/usr/bin/env python3
"""编译原生云候选、AI、翻译和批量语音客户端，通过 TLS 验证 Go 后端。"""
import os
from pathlib import Path
import subprocess
import tempfile
ROOT = Path(__file__).resolve().parents[1]
image = 'msime-backend-native-build:verification'
build = subprocess.run(['docker','build','--platform','linux/amd64','-t',image,str(ROOT / 'tests/native')],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
if build.returncode:
    raise SystemExit(build.stdout)
print('原生集成测试镜像已就绪', flush=True)
with tempfile.TemporaryDirectory(prefix='msime-native-e2e-') as directory:
    env = dict(os.environ, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    subprocess.run(['go','test','-c','-o',str(Path(directory)/'server.test'),'./internal/server'],cwd=ROOT,env=env,check=True)
    command = r'''
set -eu
g++ -std=c++17 -Wall -Wextra -Werror -DMSIME_WINDOWS_CLIENT -I/windows/server/src /server/tests/native_cloud_client.cpp /windows/server/src/cloud/cloud_request.cpp -lcurl -o /tmp/windows-cloud
engine=/linux/vendor/MetasequoiaImeEngine
g++ -std=c++17 -Wall -Wextra -Werror -I/linux/src -I"$engine" -I"$engine/include" /server/tests/native_cloud_client.cpp /linux/src/online/GoogleCloudProvider.cpp /linux/src/online/CurlHttpTransport.cpp /linux/src/online/EndpointPolicy.cpp -lcurl -lboost_json -o /tmp/linux-cloud
cmake -S /server/tests/native -B /tmp/native-build -DCMAKE_BUILD_TYPE=Release > /tmp/native-configure.log 2>&1 || { cat /tmp/native-configure.log; exit 1; }
cmake --build /tmp/native-build --target native-services --parallel 6 > /tmp/native-build.log 2>&1 || { tail -60 /tmp/native-build.log; exit 1; }
MSIME_NATIVE_CLIENTS=/tmp/windows-cloud:/tmp/linux-cloud MSIME_NATIVE_SERVICES=/tmp/native-build/native-services /test/server.test -test.run '^TestNative(CloudClients|Services)$' -test.v
'''
    subprocess.run(['docker','run','--rm','--platform','linux/amd64',
        '-v',f'{ROOT}:/server:ro','-v',f'{ROOT.parent / "MSIME-Windows"}:/windows:ro',
        '-v',f'{ROOT.parent / "MSIME-Linux"}:/linux:ro','-v',f'{directory}:/test:ro',
        image,'sh','-c',command],check=True)
