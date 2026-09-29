# 可解释实验分流平台（合成数据演示）

这是一个不接真实分析平台、不使用真实用户数据的实验分流示例：

- **Angular**：运营配置层、实验、桶区间、资格条件，并输入合成用户查看完整解释。
- **Go + Gin**：执行稳定哈希、互斥层选实验、资格检查、变体选择、白名单覆盖和记录。
- **PostgreSQL**：保存实验定义、互斥区间约束、分配记录和曝光记录。

## 分流模型

桶区间采用半开区间 `[start, end)`，因此实验 `[0,5000)` 与 `[5000,10000)` 不重叠；边界 4999 属于前者，5000 属于后者。

### 1. 互斥层哈希

先在层上选择实验：

```text
namespace:layer_id:layer_salt:user_id
growth:checkout_layer:layer-salt-v1:syn-3340
```

实现为 FNV-1a 32-bit：

```text
bucket = fnv1a32(input) mod layer.bucket_size
```

用户标识变化会改变桶；命名空间和层 ID 防止跨产品/层复用同一抽签；层盐值允许在有审计记录时重新随机化整个层。

### 2. 实验内变体哈希

命中实验后再哈希变体：

```text
namespace:layer_id:layer_salt:experiment_id:experiment_salt:user_id
growth:checkout_layer:layer-salt-v1:one_click_promo:variant-salt-v1:syn-3340
```

第二次哈希与层桶独立，避免层位置决定变体位置。所有输入均来自稳定用户 ID 与已保存配置，不依赖请求先后、进程内存或随机数，所以重启后结果相同。

### 3. 资格与曝光

执行顺序：

1. 身份检查：未注册/不可识别的合成身份直接拒绝，不计算实验桶、不写曝光。
2. 层哈希：根据层桶落入唯一互斥实验。
3. 资格检查：失败时停在该实验并返回原因，不会因为另一个实验“刚好有空”而改投。
4. 变体哈希：仅资格通过的用户选择 control/treatment。
5. 白名单：已知身份可由运营强制覆盖；覆盖原因写入 `override_reason`，统计键加 `whitelist:` 前缀，与普通曝光分开。
6. 曝光：只在请求同时要求 `record=true` 和 `expose=true`，且最终状态为 `assigned` 时写入。拒绝用户只可能产生分配诊断记录，不会进入曝光统计。

## 预置验证样例

| 合成用户 | 输入 | 层桶 | 结果 |
|---|---|---:|---|
| `syn-3340` | `growth:checkout_layer:layer-salt-v1:syn-3340` | 4999 | `[0,5000)` → `one_click_promo`，变体桶 1748 → control |
| `syn-15461` | `growth:checkout_layer:layer-salt-v1:syn-15461` | 5000 | `[5000,10000)` → `rewards_panel`，变体桶 7319 → treatment |
| `unknown-user-x` | 未注册 | 不计算 | `UNKNOWN_IDENTITY`，无实验/变体、无曝光 |
| `vip-001` | CA/free，通常不满足 rewards 资格 | 6569 | 白名单强制到 `rewards_panel/rewards_treatment`，保留覆盖原因 |

## 本地运行

### Docker Compose + PostgreSQL

```bash
docker compose up --build postgres backend
# 另开终端启动 Angular
docker compose --profile frontend up frontend
```

访问：

- UI: http://localhost:4200
- API: http://localhost:8080/api/health

### 不用 Docker 时运行后端

仅 UI 开发时，未设置 `DATABASE_URL` 会使用内存种子；稳定性由纯函数保证，但记录不跨进程持久化。

```bash
cd backend
go test ./...
go run ./cmd/server
```

连接 PostgreSQL：

```bash
export DATABASE_URL='postgres://ab_platform:synthetic_only@localhost:5432/ab_platform?sslmode=disable'
go run ./cmd/server
```

启动时会按文件名执行 `backend/migrations/*.sql`。

### 启动 Angular

```bash
cd frontend
npm install
npm start
```

## API

### 计算并可选记录

```bash
curl -X POST http://localhost:8080/api/decide \
  -H 'Content-Type: application/json' \
  -d '{
    "subject": {
      "user_id": "syn-3340",
      "registered": true,
      "country": "US",
      "plan": "free",
      "source": "synthetic"
    },
    "record": true,
    "expose": true
  }'
```

只读解释时将 `record` 和 `expose` 都设为 `false`。

### 批量合成用户比例

```bash
curl -X POST http://localhost:8080/api/simulate \
  -H 'Content-Type: application/json' \
  -d '{"count":1000}'
```

批量接口不保存任何记录，用于观察稳定哈希下各实验/变体比例与资格拒绝原因。

### 统计已保存的合成记录

```bash
curl 'http://localhost:8080/api/stats?source=synthetic'
```

## 数据库要点

- `experiments` 使用 PostgreSQL `EXCLUDE` + `int4range(..., '[)')` 防止同一层活跃实验桶区间重叠。
- `variants` 同样防止同一实验内活跃变体区间重叠。
- `assignments` 对 `(user_id, layer_id, source)` 建唯一键；重复请求更新同一条诊断分配，而不是产生新随机组。
- `exposures` 与 `assignments` 分离；曝光只属于已分配用户。
- 白名单的 `reason` 同时保存在白名单、分配和曝光表中，便于审计和单独统计。

## 已验证

```bash
cd backend && CGO_ENABLED=0 go test ./...
cd frontend && npm run build
```

测试覆盖重启等价、互斥区间 4999/5000 边界、未知身份拒绝、资格失败不落到另一实验、白名单覆盖及原因保留。
