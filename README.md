<h1 align="center">Claude2API</h1>

<p align="center">Claude2API 是一个基于 Go 开发的 Claude.ai API 兼容网关、账号池与网页镜像服务。项目通过统一管理多个 Claude.ai 账号并自动轮询可用账号，将 Claude 模型能力以 OpenAI API 和 Anthropic API 兼容接口的形式提供给第三方客户端、自动化脚本与开发工具。Claude2API 支持 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 接口，兼容 Claude Code、Codex CLI 以及支持自定义 Base URL 的应用；同时提供流式输出、多轮对话、System Prompt、多模态图片输入、扩展思考（Thinking）、Function Calling / Tool Use、长上下文处理、会话清理、API Key 鉴权、调用日志和可视化后台管理。项目支持 Docker Compose 一键部署，适合个人学习、接口适配、客户端联调和 Claude API 集成测试。</p>

> [!WARNING]
> 免责声明：
>
> 本项目涉及对 Claude.ai 官网相关能力的逆向研究，仅供个人学习、技术研究与非商业性技术交流使用。
>
> - 严禁将本项目用于任何商业用途、盈利性使用、批量操作、自动化滥用或规模化调用。
> - 严禁将本项目用于破坏市场秩序、恶意竞争、套利倒卖、二次售卖相关服务，以及任何违反 Anthropic 服务条款或当地法律法规的行为。
> - 严禁将本项目用于生成、传播或协助生成违法、暴力、色情、未成年人相关内容，或用于诈骗、欺诈、骚扰等非法或不当用途。
> - 使用者应自行承担全部风险，包括但不限于账号受限、临时封禁、永久封禁以及因违规使用导致的法律责任。
> - 本项目依赖 Claude.ai 上游接口，上游接口、风控策略及页面结构的变化都可能导致部分功能失效。
> - 使用本项目即视为你已充分理解并同意本免责声明；请勿使用重要账号、常用账号或高价值账号进行测试。

## 快速开始

Docker Compose 一键部署

```bash
git clone https://github.com/crossfff/claude2api.git
cd claude2api
cp config.example.yaml config.yaml
# 编辑 config.yaml：至少填写 proxy（访问 claude.ai 的出口代理）与 web_uc_host（沙箱域名）
docker compose up -d --build
```

> `docker compose` 默认使用仓库内的 `Dockerfile` 本地构建，确保运行的是本仓库代码。
> 首次构建需要下载 Go 依赖，耗时较长；之后更新只需 `git pull && docker compose up -d --build`。
>
> 也可以使用预构建镜像：给仓库打 `v*.*.*` 标签会触发 GitHub Actions 构建并推送
> `ghcr.io/crossfff/claude2api:latest`（需先在 GitHub Packages 页面把镜像设为 public）。

## 核心功能

- **官网镜像**：提供 Claude.ai 官网镜像和账号池选择页面，可随机或指定可用账号进入镜像站，并自动维护访问所需的 Cookie、浏览器指纹与会话信息。
- **号池管理**：支持批量导入 Claude.ai 账号，自动获取邮箱和组织信息，并提供账号状态查看、手动刷新、失效清理与定期巡检。
- **账号轮询**：API 请求自动轮询可用账号，请求失败时可按配置换号重试，避免单个账号异常影响服务。
- **OpenAI 兼容**：支持 `POST /v1/chat/completions` 和 `POST /v1/responses` 接口，可接入 OpenAI SDK 及支持自定义 Base URL 的客户端。
- **Anthropic 兼容**：支持 `POST /v1/messages` 接口，可接入 Anthropic SDK、Claude Code 等兼容客户端。
- **开发工具接入**：支持配置 Codex CLI、Claude Code 使用本服务的兼容 API。
- **模型列表**：提供 `GET /v1/models` 接口，统一返回当前支持的基础模型及 Thinking 模型。
- **流式响应**：OpenAI 与 Anthropic 接口均支持流式和非流式输出。
- **多轮对话**：支持 System Prompt、user / assistant 历史消息以及多轮上下文拼接。
- **多模态输入**：支持 OpenAI 与 Anthropic 格式的 Base64 图片输入，并自动上传至 Claude.ai。
- **扩展思考**：模型名添加 `-thinking` 后缀即可启用扩展思考模式。
- **工具调用**：支持 OpenAI `tools / tool_calls`、Responses `function_call` 和 Anthropic `tools / tool_use`，兼容流式、非流式及工具结果回传。
- **长上下文处理**：提示词超过配置阈值时自动转换为文本附件，减少超长上下文直接提交造成的问题。
- **会话清理**：支持请求完成后自动删除 Claude.ai 上游会话。
- **密钥管理**：可在后台创建、查看和删除多个 API 密钥，用于号池、镜像和 API 接口鉴权。
- **在线测试**：后台内置多轮对话测试页面，支持选择模型、流式输出和图片上传。
- **调用日志**：记录调用时间、接口、模型、使用账号、响应状态、输入输出 Token、总耗时、首字延迟和 TPS。
- **日志详情**：可选保存完整请求与响应内容，支持查看详情、批量删除及仅保留最近指定数量的日志。
- **运行时配置**：可在后台调整出口代理、重试次数、长上下文阈值、会话清理、账号巡检和详细日志等设置。

## 效果展示

<table width="100%">
  <tr>
    <td width="50%"><img src="https://i.ibb.co/3yMvBm0R/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/zhpt12gG/image.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/XZ4G85yp/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/LhxVLkYN/image.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/99v19T70/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/ZpYzPkb9/image.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/1Y0C0Nyq/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/rK773GJR/image.png" alt="image" border="0"></td>
  </tr>
</table>

## 账号导入

进入管理后台的「账号管理」，点击「批量导入」，每行填写一个 `sessionKey`：

```text
sk-ant-sid01-xxxxxxxx
sk-ant-sid01-yyyyyyyy
```

导入任务会在后台执行，并自动查询账号信息。`sessionKey` 等同于账号登录凭据，请妥善保管。

### 支持的模型

模型清单集中定义在 `internal/models/models.go`，并支持从 Claude.ai 网页端自动拉取。
内置清单对应 Anthropic 2026-10 的官方模型：

| 对外模型名 | 官方名称 | 最低订阅等级 | Thinking |
| --- | --- | --- | --- |
| `claude-fable-5-1` | Claude Fable 5.1 | Max | 支持 |
| `claude-opus-5-5` | Claude Opus 5.5 | Pro | 支持 |
| `claude-sonnet-5-5` | Claude Sonnet 5.5 | Free | 支持 |
| `claude-haiku-5-5` | Claude Haiku 5.5 | Free | 支持 |

每个模型都额外提供 `-thinking` 版本（例如 `claude-opus-5-5-thinking`），
用于开启 Claude.ai 的扩展思考模式。

### 模型清单自动同步

服务启动后会按 `model_sync_seconds`（默认 3600 秒）定时从 Claude.ai 网页端
拉取当前账号可见的模型清单，并持久化到 `data/web_models.json`：

- 拉取成功后 `GET /v1/models` 立即反映最新模型，重启后自动恢复缓存；
- 拉取失败时保留上一次成功的缓存，失败原因记录在 `GET /api/models` 的 `dynamic.last_error`；
- 上游无法提供等级信息时按模型系列推断（fable/opus → pro，sonnet/haiku → free）。

清单合并优先级（后者覆盖前者）：**内置清单 → web 端拉取 → `config.yaml` 的 `model_overrides`**。
也就是说自动同步会跟随上游变化，而人工配置始终拥有最终决定权。

| 接口 | 说明 |
| --- | --- |
| `POST /api/models/sync` | 立即同步；body 可选 `{"email":"指定账号"}`，不传则自动挑等级最高的账号 |
| `POST /api/models/clear` | 清空 web 缓存，回退到内置清单 |
| `GET /api/models` | 查看当前清单、来源、更新时间与失败原因 |
| `POST /api/models/probe` | 探测上游原始响应，用于排查字段变化 |

关闭自动同步：把 `model_sync_seconds` 设为负数。后台「账号管理」页的
「从 web 同步」按钮可随时手动触发一次。

`最低订阅等级` 是调用该模型时对账号的要求：调度器只会把请求分配给等级满足要求的
账号，等级未知的账号不做限制。账号等级在导入或刷新账号时，从 Claude.ai
`/api/account` 返回的 `rate_limit_tier` / `organization_type` 字段自动识别。

实际可用性取决于账号权限和 Claude.ai 上游状态，请以 `GET /v1/models` 的返回结果为准。

<details>
<summary><code>GET /v1/models</code></summary>
<br>

获取当前服务暴露的模型列表，可附加 `?tier=pro` 只看某个等级可用的模型：

```bash
curl "http://localhost:8787/v1/models?tier=pro" \
  -H "Authorization: Bearer <api-key>"
```

```json
{
  "object": "list",
  "data": [
    {
      "id": "claude-opus-5-5",
      "object": "model",
      "created": 1750000000,
      "owned_by": "anthropic",
      "name": "Claude Opus 5.5",
      "tier": "pro",
      "upstream_id": "claude-opus-5-5",
      "thinking": false
    }
  ]
}
```

</details>

<details>
<summary><code>POST /v1/chat/completions</code></summary>
<br>

OpenAI Chat Completions 兼容接口，支持多轮对话、图片输入以及流式输出。图片使用 Data URI：

```bash
curl http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5-5",
    "messages": [
      {
        "role": "system",
        "content": "回答尽量简短"
      },
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "描述这张图片"},
          {
            "type": "image_url",
            "image_url": {"url": "data:image/png;base64,iVBORw0KGgo..."}
          }
        ]
      }
    ],
    "stream": true
  }'
```

</details>

<details>
<summary><code>POST /v1/responses</code></summary>
<br>

OpenAI Responses API 兼容接口，`input` 支持字符串或消息数组：

```bash
curl http://localhost:8787/v1/responses \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5-5",
    "instructions": "回答尽量简短",
    "input": [
      {
        "role": "user",
        "content": [
          {"type": "input_text", "text": "描述这张图片"},
          {
            "type": "input_image",
            "image_url": "data:image/png;base64,iVBORw0KGgo..."
          }
        ]
      }
    ],
    "stream": false
  }'
```

</details>

<details>
<summary><code>POST /v1/messages</code></summary>
<br>

Anthropic Messages 兼容接口，可用于接入支持自定义 Anthropic Base URL 的客户端：

```bash
curl http://localhost:8787/v1/messages \
  -H "x-api-key: <api-key>" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5-5",
    "max_tokens": 1024,
    "system": "回答尽量简短",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "描述这张图片"},
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/png",
              "data": "iVBORw0KGgo..."
            }
          }
        ]
      }
    ],
    "stream": true
  }'
```

</details>

### 客户端接入

Codex CLI 可通过自定义 OpenAI Base URL 使用 `/v1/responses`，Claude Code 可通过自定义 Anthropic Base URL 使用 `/v1/messages`；两者均填写本服务地址和后台创建的 API Key 即可。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=crossfff/claude2api&type=Date)](https://www.star-history.com/#crossfff/claude2api&Date)

## 社区支持

学 AI，上 L 站：[Linux.do](https://linux.do)

交流群：点击链接加入群聊【开源无限画布(2群)】：https://qm.qq.com/q/Shgvco7XEu
