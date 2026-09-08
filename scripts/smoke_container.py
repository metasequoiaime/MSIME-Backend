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
parser.add_argument('--native', action='store_true', help='验证镜像内置原生资源与并发日语查询')
args = parser.parse_args()
def docker(*args):
    return subprocess.check_output(['docker', *args], text=True).strip()
with tempfile.TemporaryDirectory(prefix='msime-container-smoke-') as directory:
    path = Path(directory)/'config.json'
    config={'listen':'0.0.0.0:8080', 'clients':[{'id':'smoke','token_env':'MSIME_SMOKE_TOKEN','requests_per_minute':120}]}
    if args.native:
        config['engine']={'binary':'/usr/local/bin/msime-engine','resources':'/usr/share/msime'}
    path.write_text(json.dumps(config))
    os.chmod(directory, 0o755)
    os.chmod(path, 0o644)
    container = docker('run','-d','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
        *(['--memory','2g','--cpus','2','--tmpfs','/tmp:rw,noexec,nosuid,size=2147483648,mode=1777'] if args.native else []),
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
        for path in ['/swagger/','/openapi.json']:
            try:
                urllib.request.urlopen(base+path,timeout=2)
                raise AssertionError('production docs exposed')
            except urllib.error.HTTPError as error:
                assert error.code==404
        if args.native:
            def native_query(operation,body,expected):
                request=urllib.request.Request(base+'/v1/input/'+operation,data=json.dumps(body).encode(),headers={'Authorization':'Bearer synthetic-device-token-00000000000000000000','Content-Type':'application/json'})
                with urllib.request.urlopen(request,timeout=15) as response:
                    payload=json.load(response)
                    assert expected in json.dumps(payload,ensure_ascii=False), payload
                return payload
            native_query('candidates',{'text':'nihao','limit':5},'你好')
            native_query('convert',{'text':'头发发展'},'頭髮發展')
            native_query('annotate',{'text':'重庆银行'},"chong'qing'yin'hang")
            native_query('romaji',{'text':'konnichiha'},'こんにちは')
            from concurrent.futures import ThreadPoolExecutor
            with ThreadPoolExecutor(max_workers=4) as executor:
                futures=[executor.submit(native_query,'japanese',{'text':'kanji','limit':5},'感じ') for _ in range(4)]
                for future in futures:
                    future.result()
            assert docker('inspect','--format','{{.State.OOMKilled}}',container)=='false'
            print('镜像内置词库、简繁转换、注音、罗马字及四路日语查询通过（2 CPU / 2 GiB）')
        docker('stop','--time','10',container)
        assert docker('inspect','--format','{{.State.ExitCode}}',container)=='0'
        print('容器健康检查、鉴权、能力响应和优雅关闭均通过')
    finally:
        subprocess.run(['docker','rm','-f',container],stdout=subprocess.DEVNULL,check=True)
