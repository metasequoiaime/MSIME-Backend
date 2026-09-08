# API 逐项验证记录

2026-09-08，验证本机 `http://127.0.0.1:8080` 运行实例与受控 HTTPS/WSS 上游。实例只启用云候选；未配置的 AI、翻译和语音服务不算真实供应商验证通过。

| API | 当前实例结果 | 受控上游 / 自动化结果 |
|---|---|---|
| GET /healthz | 200，status=ok | HTTP 契约通过 |
| GET /v1/capabilities | 200，cloud=true，其他在线能力=false | 能力结构、鉴权通过 |
| GET /v1/cloud/candidates | 拼音 haohaoxuexi → 好好学习；日语 nihon → 日本、にほん、二本 | 编码、去重、非法候选过滤、部分匹配过滤通过 |
| POST /v1/chat/completions | 503 feature_disabled | 联想/润色响应、模型覆盖、兼容参数、错误输入和异常上游通过 |
| POST /v1/translate | 503 feature_disabled | DeepLX 与腾讯 TMT 转换、TC3 签名、语言和空输入校验通过 |
| POST /v1/audio/transcriptions | 503 feature_disabled | WAV multipart、模型覆盖、无效 WAV/空转写拒绝通过 |
| GET /v1/audio/stream | 503 feature_disabled | WSS 双向二进制、鉴权、大小限制、超时和关闭通过 |

受保护的六个入口无令牌均返回 401。当前实例 18 项检查全部符合预期。Go race 测试共有 55 项测试/子测试通过，3 个可选原生测试在默认运行时跳过，随后由 `native_e2e.py` 和 `apple_e2e.py` 单独执行并通过。`go vet` 通过。原始本地结果位于 `bin/live-api-report.json`、`bin/api-tests.json`、`bin/native-api-tests.log` 和 `bin/apple-api-tests.log`；不含设备令牌。

## 本次发现与修复

1. pinyin 接口接受汉字导致原文/短词被当作拼音候选返回。现在含汉字的 pinyin 请求返回 400 `pinyin_spelling_required`，Swagger 给出拼音示例。
2. Google 拼音候选包含只覆盖输入前缀的词，原适配器忽略了 `matched_length`。现在有该元数据时，仅返回覆盖整段拼音的候选；`limit` 是最大数量，过滤后可少于该值。
3. 日语 `matched_length` 按转换后的假名计数，不能与罗马字长度直接比较，已增加独立兼容测试，防止误过滤日语候选。

修复后已重启服务并复测：`haohaoxuexi` 返回 `{"candidates":["好好学习"]}`；`好好学习` 配合 `scheme=pinyin` 返回 400；日语 `nihon` 正常返回候选。

这证明协议、参数处理与响应转换，不证明未配置模型的实际生成质量、真实腾讯账户权限或真实语音供应商识别效果。完整业务验收仍需为相应能力配置上游。

## EveryAPI 实际上游补充验证

后续已将本地实例聊天上游配置为 EveryAPI `/v1/chat/completions`，模型 `gpt-5.6-luna`，`chat` 能力已启用。通过本地服务实测三项均为 200：连接探针返回 `EveryAPI connected.`；原生 AI 候选请求返回 `{"candidates":[{"text":"好好学习"}]}`；语音文本润色返回“今天我们测试输入法，明天下午三点开会。”。分别耗时约 1.79、27.71、1.89 秒；此前也观察到候选请求超过 30 秒超时，不能将其视为稳定的实时候选延迟。原始脱敏结果在 `bin/everyapi-verification.json`。

发现 EveryAPI JSON 模式检查用户输入中的 json 要求，而既有客户端仅在 system 中声明 JSON 格式会失败；已添加格式兼容指令并完成回归测试。Go race/vet 通过。上表中 chat 未启用是配置前的结果；其他未启用能力未改变。

## EveryAPI 翻译与录音转写启用验证

2026-09-08 后续本地配置已启用翻译和批量录音转写。通过本地入口实测 `/v1/translate`（`gpt-5.6-luna`）200，约 3.35 秒；`/v1/audio/transcriptions`（`volc.seedasr.sauc.duration`）200，约 1.53 秒。使用合成中文句子和录音，译文与转写符合该样例预期。能力查询为 chat/cloud/translation/transcription=true，streaming_transcription=false。以上单次耗时不代表延迟保证。

Go 全量测试、竞态检查和 vet 通过。真实 EveryAPI 语音与翻译结果保存在未提交的本地产物中；生产密钥、本机配置与音频不进入仓库。前文“未配置”表格描述的是首次检查时的历史状态。
