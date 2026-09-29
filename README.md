# ExplainAB：可解释实验分流平台（合成数据）

这个示例实现一个可解释、可复现的实验分流链路：

- **Angular 18**：配置互斥层、实验闭区间、变体权重、资格规则和白名单；展示完整判定路径。
- **Go + Gin**：执行身份检查、稳定哈希、互斥区间匹配、资格校验、白名单覆盖和分配/曝光写入。
- **PostgreSQL**：保存实验定义、桶区间约束、稳定分配以及曝光记录；不依赖外部分析平台。
- 所有用户均为明确命名的 **synthetic/ synthetic-boundary** 标识，不包含真实用户数据。

## 1. 分流模型

### 稳定哈希

层桶使用 64 位 FNV-1a：

```text
layerHashInput   = namespace + ":" + salt + ":" + stable_user_id
layerBucket      = FNV1a64(layerHashInput) mod 10000
```

示例：

```text
growth.checkout:2026-09-29:synthetic-000001 -> 1001
growth.checkout:2026-10-01:synthetic-000001 -> 5865
other.namespace:2026-09-29:synthetic-000001 -> 6633
growth.checkout:2026-09-29:synthetic-000002 -> 6368
```

这清楚体现了三部分作用：

- 用户标识变化：同一命名空间/盐值下重新分桶；
- 盐值变化：同一用户可随新 salt 重新分流；
- 命名空间变化：不同业务层互不影响。

代码只依赖确定字符串和标准哈希，没有内存随机状态，因此同一输入在进程重启后仍相同。

### 互斥层和闭区间

内置 `checkout` 层的两个活动实验为：

| 实验 | 桶区间（闭区间） | 含义 |
|---|---:|---|
| `paywall_v3` | `[0,4999]` | 50% 流量 |
| `checkout_banner` | `[5000,7499]` | 25% 流量 |
| 空档 | `[7500,9999]` | 不进入任何实验 |

实验归属只由同一个层桶决定，不取决于先调用哪个实验接口。PostgreSQL 使用 GiST 排他约束阻止活动实验区间重叠；Go 配置校验也会拒绝重叠。

边界样例：

| 用户 | 层桶 | 归属/状态 |
|---|---:|---|
| `boundary-4961` | 2499 | `paywall_v3`，包含边界 |
| `boundary-8281` | 2500 | `paywall_v3` |
| `boundary-4284` | 4999 | `paywall_v3`，包含边界 |
| `boundary-3792` | 5000 | `checkout_banner`，包含边界 |
| `boundary-8518` | 9999 | `NO_EXPERIMENT_RANGE`，空档 |

### 变体

进入实验后再计算独立的变体桶：

```text
variantHashInput = namespace + ":" + salt + ":" + experiment_key + ":variant:" + stable_user_id
variantBucket    = FNV1a64(variantHashInput) mod 10000
```

每个实验的变体权重合计必须为 10000：

- `control`: 5000，即 `[0,4999]`
- `red` / `badges`: 2500，即 `[5000,7499]`
- `blue` / `reviews`: 2500，即 `[7500,9999]`

### 身份、资格和曝光

判定顺序：

1. 身份是否已知。未知身份直接返回 `UNKNOWN_IDENTITY`，**不计算哈希、不产生分配、不写曝光**。
2. 查找白名单。白名单在同一层内受 `UNIQUE(layer_id,user_id)` 保护，不能同时覆盖两个互斥实验。
3. 普通用户计算层桶并匹配唯一实验区间。
4. 对命中的实验执行国家、注册状态、账龄等资格规则。
5. 资格失败：返回命中的实验和拒绝原因，但不产生 assignment/exposure。
6. 资格通过：计算变体桶，写 assignment；仅当调用方明确 `record_exposure=true` 时写 exposure。

白名单覆盖：

- `source='whitelist'`
- `reason` 保存运营原因，例如：`Customer success override for VIP design preview`
- 与 `source='bucket'` 的普通统计分开聚合。

## 2. PostgreSQL 表

- `layers`：互斥层 key、命名空间和盐值。
- `experiments`：实验、闭区间和资格 JSON；带 GiST 区间排他约束。
- `variants`：实验变体和权重。
- `whitelist`：白名单用户、指定变体、原因；同一层同一用户只能有一条覆盖。
- `assignments`：`UNIQUE(layer_id,user_id)` 保证刷新页面不会换组；存储桶、变体、来源和原因。
- `exposures`：真实勾选记录的曝光日志；普通桶分组与白名单覆盖可按 `source/reason` 分开统计。

## 3. 本地运行

### 启动 PostgreSQL

任选一种方式。

Docker Compose：

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://explorer:explorer@localhost:5432/explainab?sslmode=disable'
```

或使用已安装的 PostgreSQL 15+：

```bash
createdb explainab
export DATABASE_URL='postgres://$USER@localhost:5432/explainab?sslmode=disable'
```

### 启动 Go API

```bash
cd backend
go mod download
go test ./...
go run ./cmd/server
# 默认 HTTP_ADDR=:8080
```

服务启动时自动执行 schema migration 并在空库中写入示例层。

### 启动 Angular

```bash
cd frontend
npm install
npm start
# http://localhost:4200 ，/api 代理到 http://localhost:8080
```

## 4. API 示例

### 单用户解释

```bash
curl -X POST http://localhost:8080/api/layers/checkout/evaluate \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id":"boundary-3792",
    "known":true,
    "country":"US",
    "registered":true,
    "account_age_days":30,
    "persist":true,
    "record_exposure":true
  }'
```

响应中包含：

- `hash_input`：完整稳定哈希输入；
- `bucket`：层桶；
- `experiment_key`：互斥层区间选出的实验；
- `variant_key`：变体桶选出的组；
- `source`：`bucket` 或 `whitelist`；
- `trace[]`：身份、白名单、层桶、区间、资格、变体、曝光的每一步说明。

### 未知身份拒绝

```bash
curl -X POST http://localhost:8080/api/layers/checkout/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"unknown-person","known":false,"persist":true,"record_exposure":true}'
```

期望：`status=rejected`、`reason_code=UNKNOWN_IDENTITY`，并且没有 `hash_input`、`bucket`、`variant_key` 或曝光记录。

### 白名单覆盖

```bash
curl -X POST http://localhost:8080/api/layers/checkout/evaluate \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id":"vip-001",
    "known":true,
    "country":"FR",
    "registered":false,
    "account_age_days":0,
    "persist":true,
    "record_exposure":true
  }'
```

即使普通资格不通过，也会返回：

```text
status=assigned_whitelist
source=whitelist
variant_key=red
reason=Customer success override for VIP design preview
```

### 一批合成用户

```bash
curl -X POST http://localhost:8080/api/simulate \
  -H 'Content-Type: application/json' \
  -d '{"layer_key":"checkout","n":10000,"country":"US","seed":20260929}'
```

该接口生成确定且唯一的 `synthetic-seed20260929-000000 ...` 标识，不写库。一次 10,000 合成用户验证结果：

```text
assigned_bucket: 7589
out_of_traffic:  2411

paywall_v3:       5116（预期 50%）
checkout_banner:  2473（预期 25%）

paywall_v3:control 2494, red 1322, blue 1300
checkout_banner:control 1258, badges 594, reviews 621
```

实验命中比例接近配置的 50%/25%/25% 空档；实验内部变体接近 50%/25%/25%。

### 查看落库统计

```bash
curl http://localhost:8080/api/stats
```

统计行包含 `source` 和 `reason`，可以将白名单覆盖与普通桶分组分离。

## 5. 自动化验证

后端测试覆盖：

- 闭区间边界 0、2499、2500、4999、5000、9999；
- 两个互斥实验由桶而不是接口顺序决定；
- 未知身份在哈希前拒绝；
- 命中区间但资格失败不产生分组/曝光；
- 白名单覆盖保留原因并标记来源；
- 相同输入稳定，namespace/salt 参与哈希。

运行：

```bash
cd backend && go test ./... -v
```

前端生产构建：

```bash
cd frontend && npm run build
```
