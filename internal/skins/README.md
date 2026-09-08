# 皮肤目录 API

接口需要设备或用户 access token，生产 Swagger 仍默认关闭。

- `GET /v1/skins?layout=horizontal&theme=dark`：查询兼容皮肤；两个筛选条件均可省略。
- `GET /v1/skins/{id}`：读取 `skin.toml` 对应元数据、资源路径、大小和 SHA-256。
- `GET /v1/skins/{id}/resources/{resource}`：下载资源，`resource` 可包含皮肤内子目录。
- `GET /v1/skins/source`：内置皮肤来源提交及文件摘要。
- `GET /v1/skins/license`：内置皮肤完整许可证。

四个内置 ID 是 `fluent`、`wechat`、`graphite`、`willow_green`。CSS 按 `source.json` 固定到 Windows 已合并提交，保持原始字节。自定义包不能覆盖这些 ID。

管理员可在后端配置设置绝对路径 `skins_root`，例如 `/data/skins`，并只读挂载此目录。每个子目录是独立皮肤包，目录名必须等于 `skin.toml` 的 `id`，遵循 Windows `schema_version = 1` 格式。

```toml
schema_version = 1
id = "example-skin"
name = "示例皮肤"
version = "1.0.0"
base = "fluent"
preview = "images/preview.png"
toolbar_stylesheet = "toolbar.css"

[supports]
layouts = ["horizontal", "vertical"]
themes = ["dark", "light"]

[candidate_window]
min_width_dip = 240

[candidate_window.decoration]
top_inset_dip = 0
width_dip = 0
```

声明的预览与工具栏文件必须存在。每个资源最多 4 MiB，包资源合计最多 16 MiB，扫描最多 512 个目录条目。只提供皮肤清单中的 CSS、图片、字体和 `skin.toml`，不提供任意文件下载；资源路径不能越出该包。无效包不进入列表，`invalid_packages` 给出数量，响应不包含服务器本机路径。

下载接口提供原始资源，不在服务端渲染页面。客户端保留 Windows 已有的 CSS URL 隔离和资源嵌入规则，并可按清单摘要验证下载内容。发布自定义包时宜采用完整目录切换，避免客户端下载期间文件发生变化。
