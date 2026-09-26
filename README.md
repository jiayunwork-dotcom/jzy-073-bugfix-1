# ISA 标准大气服务 (isa-service)

基于 **Go 1.22 + Gin** 的国际标准大气（International Standard Atmosphere, ISA）核算常驻服务。
覆盖 **0 m – 20000 m**，喂一个高度直接返回温度、气压、密度、声速和密度高度；也支持整段区间出剖面。
无持久化、无图形界面，每次请求独立计算。

## 大气模型

| 层 | 高度区间 | 温度 | 气压 |
|---|---|---|---|
| 对流层 | 0 – 11 km | 线性下降，直减率 6.5 K/km：`T = T0 - L·h` | 对流层压高公式：`P = P0·(1 - L·h/T0)^(g/(R·L))` |
| 等温层（平流层下部） | 11 – 20 km | 钉在对流层顶值 `T1 = 216.65 K` | 等温指数衰减：`P = P1·exp(-g·(h-h1)/(R·T1))` |

钉死的基准/物理常数（调用方不可覆盖，集中在 `internal/isa/constants.go`，两套公式共用）：

- 海平面：`T0 = 288.15 K`、`P0 = 101325 Pa`、`ρ0 = 1.225 kg/m³`
- 直减率 `L = 0.0065 K/m`，重力加速度 `g = 9.80665 m/s²`，绝热指数 `γ = 1.4`
- 干空气比气体常数 `R = P0/(ρ0·T0)`（数值即标准表 287.05287 J/(kg·K)），由同一组基准导出，
  保证 `ρ(0)` 精确回到 1.225、且正算与反算互为严格逆运算

派生量：

- 密度（理想气体状态方程）：`ρ = P/(R·T)`
- 声速（绝热公式，只与温度有关）：`a = sqrt(γ·R·T)`
- 密度高度：在**标准大气**下反解 `ρ_std(h) = 当前密度` 得到的等效高度

### 温度偏差（实际大气）

`delta_t` 是一个均匀温度偏差 ΔT（K）。处理严格遵循“标准气压廓线”与“实际温度”分离：

- **气压始终用标准高度公式计算，偏差不改变气压**；
- 温度取 `T_std(h) + ΔT`，再用它重新算密度和声速；
- 不给偏差（默认 0）时，结果就是纯标准大气值。

因此暖空气（ΔT>0）密度偏小、密度高度**高于**真实几何高度；冷空气相反。

### 密度高度的逆运算性质

- 不给偏差时，对任意合法高度 `h`，`DensityAltitude(Model(h,0).Density) == h`，互为逆运算（测试逐点钉死，误差 < 1e-6 m）；
- 叠了偏差后密度偏离标准值，反解出的密度高度随之偏离几何高度——这正是密度高度的物理意义，二者不应混为一谈。

## HTTP 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/isa/point?altitude=<m>[&delta_t=<K>]` | 单点完整状态量 |
| GET | `/api/isa/profile?start=<m>&end=<m>&step=<m>[&delta_t=<K>]` | 区间逐点剖面（整数索引取点，不做浮点累加；端点不拉伸） |
| GET | `/api/isa/demo` | 示范算例：11 km 对流层顶巡航层标准值（T=216.65 K，P≈22632.06 Pa） |
| GET | `/healthz` | 健康检查 |

单点响应字段：`altitude_m`、`temperature_offset_k`、`standard_temperature_k`、`temperature_k`、
`pressure_pa`、`density_kg_m3`、`speed_of_sound_m_s`、`density_altitude_m`。

```bash
# 11 km 巡航层（示范算例，可随手核对）
curl 'http://localhost:8080/api/isa/point?altitude=11000'
# -> T=216.65 K, P≈22632.04 Pa, ρ≈0.3639 kg/m³, a≈295.07 m/s, 密度高度=11000 m

# 8 km，实际大气偏暖 15 K（气压仍是标准廓线，密度高度被抬高）
curl 'http://localhost:8080/api/isa/point?altitude=8000&delta_t=15'

# 0–10 km，每 2 km 一条完整剖面
curl 'http://localhost:8080/api/isa/profile?start=0&end=10000&step=2000'
```

非法输入返回 `400` 与结构化原因，例如：

```json
{"error":"invalid_request","reason":"altitude below sea level is not supported (h < 0 m)"}
```

## 边界与拒算规则

- `h < 0`（低于海平面）：非法；
- `h > 20000 m`（超过实现上限）：非法；
- 非有限高度/偏差、非正步长、`end < start`、点数超 100000：非法；
- 服务**绝不外推**到 20 km 以上的更高层大气给一个无物理意义的数字。

> 注意：密度高度反算结果本身可能落在 0 m 以下或 20 km 以上（极冷/极热空气的数学等效高度），
> 这是合理的反解结果，不属于“外推输入”。

## 目录结构

```
internal/isa/
  constants.go        # 全部钉死常数 + 状态方程/声速（唯一真源）
  troposphere.go      # 对流层公式
  isothermal.go       # 等温层公式
  density_altitude.go # 密度高度反解
  model.go            # 装配、校验、偏差处理、区间剖面
internal/httpapi/
  router.go           # 路由
  handlers.go         # 处理器
cmd/server/main.go    # 入口
```

## 运行 / 测试 / 构建

```bash
go test ./...                 # 自动化测试
go test ./... -race -cover    # 竞态 + 覆盖率
go run ./cmd/server           # 本地起服务（默认 :8080，PORT 可覆盖）

docker build -t isa-service . # 基础镜像 golang:1.22-alpine，多阶段构建静态二进制
docker run --rm -p 8080:8080 isa-service
```

## 自动化测试钉死的物理判据

- 高度为零：T/P/ρ 精确回到海平面基准值；
- 11 km 处对流层公式与等温层公式的气压（及温度、密度）连续，不跳；
- 对流层每升高 1 km 温度精确下降 6.5 K；
- 等温层继续升高温度恒定、气压与密度严格单调下降；
- 同温不同压声速完全相同（声速只认温度）；
- 11 km 对流层顶温度等于等温层恒定温度；
- 无偏差时密度高度正/反算严格互逆；加偏差后密度高度按预期偏离几何高度，气压廓线不变；
- 非法高度（负、超 20 km、非有限）一律返回结构化错误。
