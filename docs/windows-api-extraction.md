# Windows Server 公共 API 抽取清单

目标：把 Windows Server 中不依赖本机输入上下文的服务能力全部提供为公共后端 API。公共表示跨平台共用，不表示匿名开放用户数据。生产 `docs_enabled` 继续为 false。

核对来源：Windows `server/src/settings/settings_app.cpp`、`dictionary_manager.cpp`、`clipboard/clipboard_history.*`、`skin/candidate_skin_catalog.*`、`english/english_ime.cpp`、`emoji/emoji_ime.cpp`、`kaomoji/kaomoji_ime.cpp`、`emoji-panel/EmojiPanel.cpp`、`conversion/chinese_converter.cpp`、`ipc/event_listener.cpp` 和 Engine contracts。Windows 工作树有用户未提交修改，只读核对，不覆盖。

| 能力 | 公共接口覆盖要求 | 当前状态 |
| --- | --- | --- |
| 云候选、AI 候选与语音润色、翻译、批量与实时转写 | 现有 `/v1/cloud/candidates`、`/v1/chat/completions`、`/v1/translate`、`/v1/audio/*`，保留现有契约 | 已有；需最终回归 |
| 配置读取、更新 | 按用户保存可同步设置，版本冲突检测；禁止上传或下发供应商密钥、本机路径及运行权限 | 已实现 GET/PUT preferences、GET schema；真实 PostgreSQL 并发 CAS、隔离及注销清理测试通过；待部署 |
| 拼音、五笔、英文、快捷短语词库 | 查询、增删改、分页、带权重文本导入导出、纯汉字注音导入；隔离基础词库与用户覆盖 | 待实现 |
| 用户候选调频、置顶、固定位置、删除与升级回放 | 保存用户覆盖、固定位置和删除记录；支持跨端导出与恢复，不修改全局基础词库 | 待实现 |
| 剪贴板历史 | 显式开启、查询/搜索、添加/去重、删除、清空；每人最多 50 条，每条最多 4000 个 UTF-16 单元 | 已实现 5 个操作；真实 PostgreSQL 显式同意、并发上限、去重、隔离及清理测试通过；待部署 |
| 皮肤目录及元数据 | 内置/自定义皮肤目录、兼容布局与主题筛选、资源安全读取 | 待实现 |
| 英文补全与中英释义 | 复用 Engine 词库查询，提供前缀补全及双向释义 | 已接入 HTTP；发布词库英文补全及英译中实测通过，反向释义待补充断言 |
| 拼音、双拼、五笔、简拼和辅助码查询 | 复用 Engine 的无状态查询/切分/辅助码接口，不在 Go 重写输入算法 | 已接入 HTTP；全拼、小鹤双拼、五笔、简拼有发布词库断言，辅助码原生实测通过；待完整边界测试及部署 |
| Emoji、颜文字、符号 | 查询候选，目录/分类与关键词搜索，固定数据版本及来源 | 已接入 HTTP，候选及三类目录分页实测通过；native/resources.lock.json 固定来源和摘要；待搜索隔离断言及部署 |
| Unicode、日期时间候选 | 复用 Engine local_modes，日期时间明确时区和参考时间 | 已接入 HTTP；真实 Engine 码点和跨日时区测试通过，待部署 |
| 简繁转换 | 复用 OpenCC s2t，固定词典和配置，不用模型伪装确定性转换 | 待实现 |

以下留在客户端，不创建远程操作入口：TSF/COM/Named Pipe 协商，按键吃键与焦点、composition 代次、候选上屏和窗口控制，DPI、UIAccess、WebView2、托盘、打开本机目录/外部程序、麦克风采集/播放/静音、系统剪贴板监听、Windows 手写 COM 识别、本机文件切换与安装升级。这些操作依赖本机权限或活动输入会话；其中可复用的数据和纯查询仍包含在上表，不以“本机功能”为由省略。

验收：逐项落实接口、严格参数限制、租户隔离及并发/失败测试、OpenAPI（仅开发显式开启）、生产迁移及真实数据验证。Windows 客户端自动上传私人输入或剪贴板不属于接口新增，不会静默开启；客户端接入需显式用户操作。
