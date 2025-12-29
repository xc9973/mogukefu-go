# Go Telegram FAQ Bot

使用 Go 语言实现的 Telegram 群聊 FAQ 机器人，采用向量相似度搜索匹配用户问题。

## 功能特性

- 🔍 **向量相似度搜索**: 使用 Embedding API 将问题转换为向量，通过余弦相似度匹配 FAQ
- 🔑 **关键词快速匹配**: 支持精确关键词匹配，跳过 API 调用直接回复
- 📝 **预设话术回复**: 严格使用预设话术，避免 AI 自由发挥产生幻觉
- 💾 **SQLite 存储**: 使用 SQLite 数据库存储知识库数据
- 🛠 **离线向量化工具**: 支持本地预生成 FAQ 向量
- 💬 **讨论组支持**: 支持 Telegram Forum/Topic 模式
- ⚡ **高性能**: Go 语言实现，资源占用低

## 核心原则

**只需要 Embedding 模型，不需要 LLM 模型**

机器人通过向量相似度匹配用户问题，使用预设模板回复，避免 AI 幻觉。

## 快速开始

### 前置要求

- Go 1.21+
- SQLite3
- Telegram Bot Token（从 [@BotFather](https://t.me/BotFather) 获取）
- OpenAI 格式 Embedding API

### 编译

```bash
cd go-bot

# 编译 Bot
go build -o bot ./cmd/bot

# 编译向量化工具
go build -o vectorize ./cmd/vectorize
```

### 配置

1. 复制示例配置文件：

```bash
cp config.example.yaml config.yaml
```

2. 编辑 `config.yaml`，填入实际配置：

```yaml
bot:
  token: "YOUR_BOT_TOKEN"
  admin_ids:
    - 123456789

embedding:
  base_url: "https://api.openai.com/v1"
  api_key: "YOUR_API_KEY"
  model: "text-embedding-3-small"

vector:
  similarity_threshold: 0.75
  short_message_threshold: 10
```

### 初始化知识库

1. 创建 FAQ 数据文件 `faq.yaml`：

```yaml
keywords:
  - keyword: "教程"
    reply: "📖 新手指南：请查看置顶消息"

faq:
  - faq_id: "register"
    question: "如何注册账号、注册流程、怎么注册"
    answer: |
      📝 注册步骤：
      1. 访问官网点击注册
      2. 填写手机号获取验证码
      3. 设置密码完成注册
```

2. 运行向量化工具：

```bash
./vectorize --input faq.yaml --output knowledge.db --config config.yaml
```

### 运行 Bot

```bash
./bot --config config.yaml --db knowledge.db
```

## 命令行参数

### Bot

| 参数 | 说明 | 默认值 |
|------|------|--------|
| --config | 配置文件路径 | config.yaml |
| --db | SQLite 数据库路径 | knowledge.db |
| --log-level | 日志级别 (debug/info/warn/error) | info |
| --log-format | 日志格式 (text/json) | text |

### Vectorize CLI

| 参数 | 说明 | 默认值 |
|------|------|--------|
| --input | 输入 YAML 文件路径 | (必填) |
| --output | 输出数据库路径 | knowledge.db |
| --config | 配置文件路径 | config.yaml |
| --verify | 验证数据库完整性 | false |

## 管理命令

Bot 支持以下管理命令（仅管理员可用）：

### 关键词管理

| 命令 | 说明 |
|------|------|
| /addkw <关键词> <回复> | 添加关键词 |
| /delkw <关键词> | 删除关键词 |
| /listkw | 列出所有关键词 |

### FAQ 管理

| 命令 | 说明 |
|------|------|
| /addfaq <ID> <问题> \| <答案> | 添加 FAQ（自动生成向量） |
| /delfaq <ID> | 删除 FAQ |
| /listfaq | 列出所有 FAQ |
| /showfaq <ID> | 查看 FAQ 详情 |

### 知识库导入导出

| 命令 | 说明 |
|------|------|
| /exportkb | 导出知识库为 YAML |
| /importkb | 导入知识库（附带 YAML 文件） |

## 消息处理流程

```
收到消息 → 长度检查 → 命令过滤 → 关键词匹配 → 向量搜索 → 预设回复
                ↓           ↓           ↓           ↓
              忽略        忽略      直接回复    根据相似度回复
```

1. **消息过滤**: 忽略长度 < 2 的消息和命令消息（以 `/` 开头）
2. **关键词匹配**: 先进行精确关键词匹配
3. **短消息过滤**: 关键词未匹配且长度 <= 阈值的消息被忽略
4. **向量搜索**: 调用 Embedding API 生成向量，搜索最相似的 FAQ
5. **回复发送**: 相似度超过阈值则发送预设回复，否则静默

## 项目结构

```
go-bot/
├── cmd/
│   ├── bot/
│   │   └── main.go          # Bot 入口
│   └── vectorize/
│       └── main.go          # 向量化工具入口
├── internal/
│   ├── bot/                 # Telegram Bot 实现
│   ├── config/              # 配置管理
│   ├── embedding/           # Embedding 客户端
│   ├── handler/             # 消息处理器
│   ├── kbstore/             # 知识库存储
│   ├── keyword/             # 关键词匹配器
│   ├── logger/              # 日志模块
│   └── vector/              # 向量存储
├── go.mod
├── go.sum
├── config.example.yaml      # 配置示例
├── faq.example.yaml         # FAQ 数据示例
└── README.md
```

## Docker 部署

### 使用 Docker

```bash
# 构建镜像
docker build -t telegram-faq-bot .

# 运行容器
docker run -d \
  --name telegram-faq-bot \
  --restart unless-stopped \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/knowledge.db:/app/knowledge.db \
  telegram-faq-bot
```

### 使用 Docker Compose

```bash
# 启动 Bot
docker compose up -d bot

# 查看日志
docker compose logs -f bot

# 运行向量化工具（需要先将 faq.yaml 放入 data/ 目录）
mkdir -p data
cp faq.example.yaml data/faq.yaml
docker compose run --rm vectorize --input /data/faq.yaml --output /app/knowledge.db --config /app/config.yaml

# 停止服务
docker compose down
```

### 在 Docker 中使用向量化工具

```bash
# 方式一：使用 docker run
docker run --rm \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/knowledge.db:/app/knowledge.db \
  -v $(pwd)/faq.yaml:/data/faq.yaml:ro \
  --entrypoint /app/vectorize \
  telegram-faq-bot \
  --input /data/faq.yaml --output /app/knowledge.db --config /app/config.yaml

# 方式二：使用 docker compose
docker compose run --rm vectorize --input /data/faq.yaml --output /app/knowledge.db
```

## 开发

### 运行测试

```bash
go test ./...
```

### 运行属性测试

```bash
go test -v ./... -run Property
```

## 配置说明

### bot 配置

| 字段 | 说明 | 必填 |
|------|------|------|
| token | Telegram Bot Token | ✅ |
| admin_ids | 管理员用户 ID 列表 | ✅ |

### embedding 配置

| 字段 | 说明 | 必填 |
|------|------|------|
| base_url | Embedding API 地址 | ✅ |
| api_key | API 密钥 | ✅ |
| model | 模型名称 | ✅ |

### vector 配置

| 字段 | 说明 | 默认值 |
|------|------|--------|
| similarity_threshold | 相似度阈值 | 0.75 |
| short_message_threshold | 短消息阈值（字符数） | 10 |

## 常见问题

### 如何获取 Telegram Bot Token？

1. 在 Telegram 中搜索 [@BotFather](https://t.me/BotFather)
2. 发送 `/newbot` 命令
3. 按提示设置 Bot 名称和用户名
4. 获取 Token

### 如何获取用户 ID？

1. 在 Telegram 中搜索 [@userinfobot](https://t.me/userinfobot)
2. 发送任意消息
3. Bot 会返回你的用户 ID

### 如何让 Bot 在群组中工作？

1. 将 Bot 添加到群组
2. 如果是超级群组，需要将 Bot 设为管理员或关闭群组的隐私模式
3. 在 BotFather 中使用 `/setprivacy` 命令设置为 `Disable`

### 向量搜索不准确怎么办？

1. 调整 `similarity_threshold` 参数（降低阈值会匹配更多，但可能不够精确）
2. 优化 FAQ 的 question 字段，添加更多同义词
3. 使用更好的 Embedding 模型

### 如何使用其他 Embedding API？

只要 API 兼容 OpenAI 格式即可，修改 `embedding.base_url` 指向你的 API 地址。

支持的 API 包括：
- OpenAI API
- Azure OpenAI
- 本地部署的兼容 API（如 LocalAI、Ollama）

## 环境变量

Bot 也支持通过环境变量覆盖配置：

| 环境变量 | 说明 |
|----------|------|
| BOT_TOKEN | Telegram Bot Token |
| EMBEDDING_API_KEY | Embedding API 密钥 |

## License

MIT License
