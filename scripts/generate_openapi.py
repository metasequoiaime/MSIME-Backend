#!/usr/bin/env python3
"""根据随仓 Engine 契约生成 Swagger OpenAPI 文档；--check 检查是否同步。"""
import json
from pathlib import Path
import sys
root = Path(__file__).resolve().parents[1]
spec = json.loads((root / 'contracts/protocol.json').read_text())
limits = spec['limits']
def obj(properties, required=None, strict=False):
    s = {'type':'object','properties':properties}
    if required: s['required']=required
    if strict: s['additionalProperties']=False
    return s
def string(**kw): return dict(type='string', **kw)
def infer(v):
    if isinstance(v, bool): return {'type':'boolean'}
    if isinstance(v, int): return {'type':'integer'}
    if isinstance(v, str): return string()
    if isinstance(v, list): return {'type':'array','items':infer(v[0]) if v else {}}
    return obj({k:infer(x) for k,x in v.items()}, list(v))
def text_limit(key): return string(minLength=1, description=f'非空文字，UTF-8 最多 {limits[key]} 字节。')
message=obj({'role':string(enum=['system','user','assistant']),'content':text_limit('chat_message_bytes')},['role','content'],True)
format_schema=obj({'type':string(enum=['json_object','text'])},['type'],True)
requests={
 'chat':obj({'model':string(description='兼容字段；实际模型由服务端固定。'),'messages':{'type':'array','minItems':1,'maxItems':limits['chat_messages'],'items':message},'stream':{'type':'boolean','enum':[False],'default':False},'max_tokens':{'type':'integer','minimum':0,'maximum':limits['chat_max_tokens'],'default':limits['chat_default_tokens'],'description':'省略或 0 使用服务端默认值。'},'temperature':{'type':'number','minimum':0,'maximum':2},'response_format':format_schema,'thinking':obj({'type':string(enum=['disabled'])},['type'],True),'enable_thinking':{'type':'boolean','enum':[False]}},['messages'],True),
 'translation':obj({'text':text_limit('translation_input_bytes'),'source_lang':string(pattern='^[A-Za-z-]{2,16}$',example='AUTO'),'target_lang':string(pattern='^[A-Za-z-]{2,16}$',example='EN')},['text','source_lang','target_lang'],True),
 'transcription':obj({'file':string(format='binary',description=f'PCM/IEEE-float RIFF/WAVE，最多 {limits["audio_file_bytes"]} 字节。'),'model':string(description='服务端覆盖此模型字段。'),'language':string(pattern='^[A-Za-z-]{2,16}$',example='zh'),'response_format':string(enum=['json'],default='json')},['file'],True)
}
summaries={'health':'健康检查','capabilities':'查询已启用能力','cloud':'云候选','chat':'AI 联想 / 语音润色','translation':'候选翻译','transcription':'WAV 批量语音转写'}
paths={}
for key,op in spec['operations'].items():
    responses={'200':{'description':'成功','content':{'application/json':{'schema':infer(op['response']),'example':op['response']}}}}
    if op['authenticated']:
        for code,description in spec['statuses'].items():
            responses[code]={'description':description,'content':{'application/json':{'schema':{'$ref':'#/components/schemas/Error'}}}}
        for code in ['429','503']:
            responses[code]['headers']={'Retry-After':{'description':'重试等待秒数（繁忙时提供）','schema':{'type':'integer'}}}
    operation={'operationId':key,'summary':summaries[key],'tags':['系统' if key in ['health','capabilities'] else '在线输入'],'responses':responses}
    if not op['authenticated']:operation['security']=[]
    if key in requests:
        media={'schema':requests[key]}
        if 'request' in op:media['example']=op['request']
        operation['requestBody']={'required':True,'content':{op['content_type']:media}}
    if key=='cloud':
        operation['description']=op['description']
        operation['parameters']=[{'name':name,'in':'query','required':name=='text','schema':schema} for name,schema in {'text':dict(text_limit('cloud_input_bytes'),example='haohaoxuexi',description='输入拼音，例如 haohaoxuexi 或 hao hao xue xi；不要输入已转换的中文。最多 256 UTF-8 字节。'),'scheme':string(enum=op['schemes'],default='pinyin'),'limit':{'type':'integer','minimum':1,'maximum':limits['cloud_candidates'],'default':5}}.items()]
    paths[op['path']]={op['method'].lower():operation}
for key,op in spec['websocket_operations'].items():
    paths[op['path']]={'get':{'operationId':key,'summary':'实时语音 WebSocket（仅文档）','tags':['实时语音'],'description':op['protocol']+'\n\nSwagger UI 不支持 WebSocket 二进制会话。使用 WSS 客户端携带设备 Bearer 令牌建立连接。'+op['close_policy']+f' 单消息最多 {limits["stream_message_bytes"]} 字节，每方向每会话最多 {limits["stream_session_bytes"]} 字节。','x-websocket':True,'responses':{'101':{'description':'WebSocket 升级成功；后续为豆包 ASR v1 二进制消息'},'400':{'description':'需要 WebSocket Upgrade'},'401':{'description':'设备令牌无效'},'503':{'description':'功能未启用或服务繁忙'}}}}
result={'openapi':'3.0.3','info':{'title':'水杉输入法后端 API','version':spec['version'],'description':'水杉输入法共通后端。点击 Authorize 填写设备令牌（不含 Bearer 前缀）。功能是否启用请查询 capabilities。JSON 请求最多 64 KiB，multipart 总体最多 16 MiB。服务不保存输入和音频。'},'servers':[{'url':'/'}],'security':[{'deviceToken':[]}],'paths':paths,'components':{'securitySchemes':{'deviceToken':{'type':'http','scheme':'bearer','description':'管理员发放的设备令牌；不是供应商密钥。'}},'schemas':{'Error':infer(spec['error_response'])}}}
# 用户体系独立于 Engine 输入协议，避免修改客户端共通契约。
user=obj({'id':string(),'display_name':string(),'created_at':string(format='date-time')})
tokens=obj({'access_token':string(),'refresh_token':string(),'token_type':string(enum=['Bearer']),'expires_in':{'type':'integer'},'user':user})
provider=string(enum=['apple','google','wechat','phone','email'])
auth_operations=[
 ('/v1/auth/providers','get','查询可用登录方式',None,obj({'providers':obj({p:{'type':'boolean'} for p in ['apple','google','wechat','phone','email']})}),False,200),
 ('/v1/auth/challenges','post','创建登录或绑定挑战',obj({'provider':provider,'target':string(description='邮箱地址或 E.164 手机号；第三方登录省略。'),'purpose':string(enum=['login','link'],default='login')},['provider'],True),obj({'challenge_id':string(),'expires_in':{'type':'integer'},'nonce':string(),'authorization_url':string()}),False,201),
 ('/v1/auth/login','post','验证凭据并登录或绑定',obj({'challenge_id':string(),'credential':string(description='六位验证码、Apple/Google ID Token 或微信授权 code。')},['challenge_id','credential'],True),tokens,False,200),
 ('/v1/auth/refresh','post','轮换用户会话令牌',obj({'refresh_token':string()},['refresh_token'],True),tokens,False,200),
 ('/v1/auth/logout','post','退出当前或全部会话',obj({'all':{'type':'boolean','default':False}},strict=True),None,True,204),
 ('/v1/users/me','get','查询当前用户和已绑定身份',None,obj({'user':user,'identities':{'type':'array','items':obj({'provider':provider,'subject':string()})}}),True,200),
 ('/v1/users/me','patch','修改当前用户昵称',obj({'display_name':string(maxLength=64)},['display_name'],True),None,True,204),
 ('/v1/users/me','delete','注销当前用户',None,None,True,204),
]
for path,method,title,body,response,protected,status in auth_operations:
    responses={str(status):{'description':'成功'}}
    if response: responses[str(status)]['content']={'application/json':{'schema':response}}
    for code in ['400','401','403','409','415','429','503']:
        responses[code]={'description':'请求无效、凭据失效、需要重新登录、身份冲突、格式错误、限流或功能不可用。','content':{'application/json':{'schema':{'$ref':'#/components/schemas/Error'}}}}
    op={'summary':title,'tags':['用户体系'],'security':[{'userSession':[]}] if protected else [],'responses':responses,'description':'JSON 请求最多 16 KiB。绑定身份需要在挑战创建和验证时携带同一用户的会话令牌；绑定和注销要求最近 10 分钟内登录。设备令牌不能用于用户管理。'}
    if body: op['requestBody']={'required':True,'content':{'application/json':{'schema':body}}}
    paths.setdefault(path,{})[method]=op
result['security']=[{'deviceToken':[]},{'userSession':[]}]
result['components']['securitySchemes']['userSession']={'type':'http','scheme':'bearer','description':'登录返回的 access_token，不是 refresh_token 或供应商密钥。'}
result['info']['description']+=' 用户接口详见用户体系标签；登录成功后也可使用用户 access_token 调用在线输入接口。'
output=root/'internal/server/swagger/openapi.json'
data=json.dumps(result,ensure_ascii=False,indent=2)+'\n'
if '--check' in sys.argv:
    if not output.exists() or output.read_text()!=data:sys.exit('OpenAPI 未同步：请运行 python3 scripts/generate_openapi.py')
else:output.write_text(data)
print('OpenAPI 与契约一致' if '--check' in sys.argv else '已生成 OpenAPI')
