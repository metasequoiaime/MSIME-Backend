#!/usr/bin/env python3
"""使用合成凭据运行已构建镜像，检查 HTTP 接口和优雅关闭。"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--image', default='msime-server:verification')
args = parser.parse_args()
def docker(*args):
    return subprocess.check_output(['docker', *args], text=True).strip()
with tempfile.TemporaryDirectory(prefix='msime-container-smoke-') as directory:
    path = Path(directory)/'config.json'
    path.write_text(json.dumps({'listen':'0.0.0.0:8080', 'clients':[{'id':'smoke','token_env':'MSIME_SMOKE_TOKEN','requests_per_minute':10}]}))
    os.chmod(directory, 0o755)
    os.chmod(path, 0o644)
    container = docker('run','-d','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
        '-p','127.0.0.1::8080','-v',f'{path}:/config/config.json:ro',
        '-e','MSIME_SMOKE_TOKEN=synthetic-device-token-00000000000000000000',args.image)
    try:
        address = docker('port',container,'8080/tcp').splitlines()[0]
        base = 'http://'+address
        for attempt in range(100):
            try:
                with urllib.request.urlopen(base+'/healthz', timeout=1) as response:
                    assert json.load(response)=={'status':'ok'}
                break
            except (urllib.error.URLError, ConnectionError):
                time.sleep(0.05)
        else:
            raise RuntimeError('container did not become healthy')
        try:
            urllib.request.urlopen(base+'/v1/capabilities',timeout=1)
            raise AssertionError('anonymous API access accepted')
        except urllib.error.HTTPError as error:
            assert error.code==401
        req = urllib.request.Request(base+'/v1/capabilities',headers={'Authorization':'Bearer synthetic-device-token-00000000000000000000'})
        with urllib.request.urlopen(req,timeout=1) as response:
            assert json.load(response)=={'api_version':'1','cloud':False,'chat':False,'translation':False,'transcription':False,'streaming_transcription':False}
        docker('stop','--time','10',container)
        assert docker('inspect','--format','{{.State.ExitCode}}',container)=='0'
        print('容器健康检查、鉴权、能力响应和优雅关闭均通过')
    finally:
        subprocess.run(['docker','rm','-f',container],stdout=subprocess.DEVNULL,check=True)
