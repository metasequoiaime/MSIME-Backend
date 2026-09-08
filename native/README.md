# 公共 Engine 桥接

后端以子进程调用固定版本 MSIME-Engine，单次 JSON 请求最长 64 KiB、响应最长 1 MiB、执行最长 10 秒。每次查询使用独立临时目录，完成或失败后由 Go 清理，不保存学习记录。请求不能指定程序、资源路径或执行参数。

## 构建与验证

依赖：CMake 3.25+、C++17、Boost、fmt、spdlog、SQLite3、nlohmann-json 3.11+。

```sh
git submodule update --init --recursive
cmake -S native -B bin/native -DCMAKE_BUILD_TYPE=Release
cmake --build bin/native --parallel 4
python3 scripts/fetch_engine_resources.py bin/resources --native-build bin/native
MSIME_ENGINE_TEST_BINARY="$PWD/bin/native/msime-engine" python3 scripts/test_native.py
MSIME_ENGINE_TEST_BINARY="$PWD/bin/native/msime-engine" MSIME_ENGINE_TEST_RESOURCES="$PWD/bin/resources" go test -race ./internal/server
python3 scripts/fetch_engine_resources.py bin/resources --native-build bin/native
```

下载脚本根据 `resources.lock.json` 验证发布文件的 SHA-256、数据来源和格式；辅助码、拼音模型与许可文件来自固定子模块的已校验文件。查询后的再次检查用于确认基础资源未被修改。发现不匹配时脚本拒绝覆盖，更新资源应在新目录校验后切换配置。

服务配置示例（路径需绝对路径）：

```json
{
  "engine": {
    "binary": "/usr/local/bin/msime-engine",
    "resources": "/data/resources"
  }
}
```

这是完整配置中的 `engine` 字段，鉴权和其他配置仍按后端配置提供。容器包含原生程序，资源目录需要挂载并校验；生产 `docs_enabled` 保持 `false`。服务账号仅需基础资源的读取权限和临时目录写入权限。

HTTP 查询与返回结构由 `scripts/generate_openapi.py` 生成，开发环境显式启用文档后可查看。现有 Engine 在线输入协议保持不变，新能力清单由 `GET /v1/input/capabilities` 提供。

当前桥接覆盖无状态查询、词条校验、OpenCC s2t 转换及 cpp-pinyin 词组注音。四类用户词库 CRUD、事务导入导出、纯汉字导入与增量变更记录已有真实 PostgreSQL + Engine 测试；用户覆盖合并查询、调频和恢复仍在开发。
