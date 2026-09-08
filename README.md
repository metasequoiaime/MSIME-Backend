# 水杉输入法共通后端（MSIME-Backend）

水杉输入法共通网络后端，使用 Go 实现；HTTP 服务基于标准库，WebSocket 使用固定版本的 [coder/websocket](https://github.com/coder/websocket)。集中保存服务商凭据，向 Windows、macOS、iOS 和 Linux 提供 HTTP API 和实时语音 WebSocket。平台继续负责本地输入、焦点、候选合并、麦克风权限及上屏。

当前服务与各平台源码接入已实现；已通过真实客户端到合成上游的网络测试，原生宿主验收仍在进行。完整需求与未完成项见 [需求核对](docs/requirements.md)。

## 启动

需要 Go 1.25 或更高版本。

```sh
cp config.example.json config.json
export MSIME_CLIENT_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/msime-server -config config.json
```

客户端用 `Authorization: Bearer <MSIME_CLIENT_TOKEN>` 访问。每台设备分配独立随机令牌和客户端 ID；添加到 `clients` 数组，通过不同环境变量注入。不要把服务商密钥或公共共享令牌编入客户端。更换配置和令牌后重启服务。

```sh
curl -H "Authorization: Bearer $MSIME_CLIENT_TOKEN" \
  http://127.0.0.1:8080/v1/capabilities
curl -G -H "Authorization: Bearer $MSIME_CLIENT_TOKEN" \
  --data-urlencode 'text=nihao' http://127.0.0.1:8080/v1/cloud/candidates
```

`config.example.json` 仅启用云候选。配置其他功能时填入管理员控制的完整 HTTPS 接口地址、服务端模型及 `token_env` 指向的环境变量；空 URL 表示关闭功能，返回 503。对外部署需由 TLS 反向代理暴露 HTTPS，限制公网端口只到反向代理。容器内把 `listen` 改为 `0.0.0.0:8080`，只读挂载配置到 `/config/config.json` 并注入令牌环境变量。Dockerfile 已通过构建；`python3 scripts/smoke_container.py --image msime-server:verification` 可验证只读容器的启动、鉴权、功能查询和优雅关闭。

## 接口

业务接口除 `GET /healthz` 外均需 Bearer 鉴权；Swagger 和 OpenAPI 文档可匿名访问。响应禁用缓存，输入正文、音频和凭据不写日志或磁盘。

| 方法与路径 | 请求 | 响应 |
|---|---|---|
| GET `/healthz` | 无 | `{"status":"ok"}`，进程存活检查 |
| GET `/v1/capabilities` | 无 | API 版本、已配置功能开关 |
| GET `/v1/cloud/candidates` | `text`，`scheme=pinyin\|japanese`，`limit=1..10` | `{"candidates":["你好"]}` |
| POST `/v1/chat/completions` | `messages`、可选 `model`、`max_tokens`、`temperature` | Chat Completions JSON；支持联想和语音润色 |
| POST `/v1/translate` | `text`、`source_lang`、`target_lang` | DeepLX 兼容 `{"code":200,"data":"..."}` |
| POST `/v1/audio/transcriptions` | multipart `file`（WAV）、可选 `model`、`language`、`response_format=json` | `{"text":"..."}` |

聊天及语音模型由服务端固定，忽略客户端模型选择；聊天未指定输出长度时最多生成 2048 token，避免长语音润色被候选场景的小预算截断；现有 AI 候选客户端明确请求 512 token，请求上限为 2048。聊天仅接受非流式文本消息（system/user/assistant），不支持工具调用。支持现有 Linux/Windows 请求中的 `response_format.type=json_object` 或 `text`；客户端的 `thinking.type=disabled` 和 `enable_thinking=false` 只作兼容接收，不透传服务商专有字段。JSON 请求上限 64 KiB；语音文件上限 15 MiB，multipart 总体上限 16 MiB；上游响应上限 1 MiB。客户端必须保留取消和输入代次校验，失败时继续本地输入。

翻译上游支持 OpenAI 兼容聊天模型、DeepLX 与腾讯 TMT `TextTranslateBatch`；客户端统一使用 DeepLX 格式，不持有腾讯密钥。语音上游使用 multipart 转写协议，可解析 `text`、`transcription` 和 `result.text`。云候选上游使用 Google Input Tools 的响应格式，输出过滤控制字符、超长候选和重复项。

错误采用 `{"error":{"code":"...","message":"..."}}`：400 参数不合法、401 未认证、403 来源不允许、413 上传过大、415 格式不支持、429 限流、502 上游失败、503 功能关闭或并发已满、504 超时。错误不透传服务商正文。429/并发已满提供 Retry-After。额度按客户端的 token bucket 控制，重启会重置；当前限流适用于单进程部署，多副本需共享配额实现后再启用。

浏览器访问须明确配置 HTTPS `allowed_origins`，不使用通配来源。原生客户端不需要 Origin。官网目前负责展示与下载，没有在线输入功能，不应添加不需要的云调用。

## 验证

```sh
go test -race ./...
go vet ./...
go build ./cmd/msime-server
```

测试使用本地 TLS 模拟服务，不需要真实服务商密钥、不消耗线上配额。测试覆盖认证、额度隔离与补充、并发上限、超时、凭据替换、模型限制、候选编码与过滤、翻译、语音 multipart、重定向与超大上游响应。

共享接口权威在 Engine `contracts/backend/protocol.json`，本仓 `contracts/protocol.json` 为精确副本。`python3 scripts/sync_contract.py --check` 检查生成常量；跨仓核对传入 `--source ../MSIME-Engine/contracts/backend/protocol.json`。契约测试通过 HTTP 和 TLS 上游执行清单中每个操作的请求/响应示例。

WAV 上传现在校验 RIFF 文件长度、分块边界、fmt/data 必需块和采样帧一致性，允许 PCM 8/16/24/32 位与 IEEE 浮点 32/64 位（1–8 声道、8–192 kHz）。分块规则参考 [Microsoft RIFF 文档](https://learn.microsoft.com/en-us/windows/win32/xaudio2/resource-interchange-file-format--riff-)。缺失或空白转写文本返回 502，不再伪装成功。

本地同时检出 MSIME-Linux、MSIME-Windows（含各自 Engine 依赖）并启动 Docker 后，可运行 `python3 scripts/native_e2e.py`。脚本自动构建专用测试镜像，编译实际客户端源码，在一次性容器中访问 Go HTTPS 服务（测试 CA 只写入容器）。覆盖两端云候选、Linux AI/翻译/语音/润色、Windows 翻译与批量语音协议，以及 Apple 使用的 Engine 共享转写/润色类。此结果不代替 Windows TSF 或 Apple 签名设备验收。

### 腾讯翻译上游

将配置的 `translation` 替换为：

```json
{
  "provider": "tencent",
  "secret_id_env": "MSIME_TENCENT_SECRET_ID",
  "token_env": "MSIME_TENCENT_SECRET_KEY",
  "region": "ap-guangzhou"
}
```

密钥通过服务进程环境注入。省略 URL 时使用 `https://tmt.tencentcloudapi.com/`；如指定 URL，必须为 HTTPS 根路径且无查询参数。服务使用 TC3 签名调用现有 Windows 适配器对应的 `TextTranslateBatch`，输出仍为 `{ "code": 200, "data": "译文" }`。腾讯账户须支持该接口；当前验证使用合成上游，未调用真实账户。选择 DeepLX 时使用 `provider: "deeplx"`（或省略 provider）并填写 URL；DeepLX 的空 URL 表示禁用翻译。

### 实时语音

`GET /v1/audio/stream` 升级为 WebSocket，使用相同的 `Authorization: Bearer <device-token>` 鉴权。输入输出均为现有豆包 ASR v1 二进制消息，保留消息边界与实时结果。该入口与 multipart 批量转写分开。

管理员配置 `streaming.url` 为豆包 WSS 接口地址，例如 `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async`，`token_env` 指向供应商 API Key 环境变量，`resource_id` 填写账户对应资源 ID。旧 App Key/Access Key 模式还须设置 `app_key_env`；设备请求无法覆盖这些上游头。空 URL 禁用功能，能力查询返回 `streaming_transcription: false`。

单条消息最大 1 MiB，每个方向每次会话累计最大 32 MiB。`max_seconds` 默认 120，可设为 1–600；会话占用一个全局并发槽。任一端断开、时限到达或服务关闭时，两个连接均释放。反向代理须允许 WebSocket Upgrade，并设置至少与会话时长一致的空闲超时。不会将供应商 HTTP 错误正文或 WebSocket 关闭原因返回设备。

Windows 设置中选择“MSIME 共通后端（实时语音）”，地址填写 `wss://你的服务/v1/audio/stream`，令牌填写设备令牌；无需填写豆包 App Key。保留原有录音、实时预编辑与会话代次校验。服务端 WSS 合成上游测试和 Windows 目标语法检查已通过，Windows 原生录音宿主与真实供应商尚未验收。

在 macOS 同时检出 MSIME-Apple 后，运行 `python3 scripts/apple_e2e.py` 可编译实际 Foundation 客户端并连接 Go TLS 服务。测试证书只作为测试进程的信任锚，不修改系统信任或 Keychain；验证候选、日语、错误令牌、未受信任证书、在途取消与主线程回传。WebSocket 依赖的许可见 `THIRD_PARTY_NOTICES.txt`，容器内放在 `/licenses/`。

## Swagger / OpenAPI

启动后访问 `/swagger/`（`/swagger` 自动跳转），规范文件为 `/openapi.json`。文档无需登录；在线 API 仍需设备令牌。点击 **Authorize**，只填写令牌本身，再使用 **Try it out → Execute**。令牌不持久化到浏览器存储。WAV 接口提供文件上传；实时语音仅展示 WebSocket 协议，不提供 HTTP 调试按钮。页面与 Swagger UI 5.32.15 的 JS/CSS 均内嵌到二进制，无需外部 CDN，禁用外部 validator。

规范由 `scripts/generate_openapi.py` 从 Engine 契约副本和接口 schema 生成；接口更新后运行该脚本，CI 使用 `--check` 检查是否同步。Swagger UI 配置参考 https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/ 。第三方资源版本及完整性值在 `internal/server/swagger/version.json`，许可和 NOTICE 随资源嵌入。

云候选的 `text` 是待转换的拼写。`scheme=pinyin` 时传拼音（如 `haohaoxuexi`），包含汉字返回 400 `pinyin_spelling_required`。当上游提供匹配长度时，拼音候选只保留覆盖整个输入的结果；`limit` 为最大数量，不保证凑满。逐项测试结果见 [API 验证记录](docs/api-verification.md)。

### EveryAPI 聊天上游

`chat.url` 使用 `https://api.everyapi.ai/v1/chat/completions`，`chat.model` 可配置为 `gpt-5.6-luna`，`chat.token_env` 指向保存供应商密钥的环境变量。客户端继续使用 MSIME 设备令牌。JSON object 模式会在用户输入没有明确 json 要求时追加一条格式指令，以兼容仅检查用户消息的 Chat-to-Responses 网关；原始消息内容保持不变。此接入覆盖 AI 联想和语音文本润色，不自动启用录音转写、翻译或实时语音。

### EveryAPI 翻译与录音转写

在服务环境中设置 `MSIME_CHAT_TOKEN`，将配置的两个字段替换为：

```json
{
  "translation": {
    "provider": "openai",
    "url": "https://api.everyapi.ai/v1/chat/completions",
    "token_env": "MSIME_CHAT_TOKEN",
    "model": "gpt-5.6-luna"
  },
  "transcription": {
    "url": "https://api.everyapi.ai/v1/audio/transcriptions",
    "token_env": "MSIME_CHAT_TOKEN",
    "model": "volc.seedasr.sauc.duration"
  }
}
```

翻译由服务添加默认提示词，支持 `source_lang: "auto"`，返回格式仍为 `{"code":200,"data":"译文"}`。上游输出被截断、拒绝或为空时返回 502，不返回不完整译文。录音上传仍使用 WAV multipart；可将转写模型改为 `openai/whisper-large-v3-turbo`。这些配置不启用实时 WebSocket；已检查 EveryAPI 源码：公开 `/v1/realtime` 使用 OpenAI Realtime 协议，豆包实时支持已提交至 [EveryAPI PR #2331](https://github.com/everyapi-ai/everyapi/pull/2331)，尚未合并部署。本服务仍需增加 EveryAPI Bearer/模型配置适配后才能切换实时上游。豆包 WAV 输入须为 16kHz、16-bit PCM、单声道或双声道。
