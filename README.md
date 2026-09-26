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
- 叠了偏差后密度偏离标准值，反解出的密度高度随之偏离几何高度——这正是密度高度的物理意义，二者不应混为一谈；
- 但反解出的等效高度**必须落在模型实际覆盖的 `[0 m, 20000 m]` 之内**。若密度比海平面标准空气还稠密
  （等效高度 < 0 m，极冷空气），或比 20 km 处标准空气还稀薄（等效高度 > 20 km，极热空气），
  服务判定为超出模型能力范围，返回结构化错误 `out_of_model_range`，**绝不把反解野值当正常结果返回**。
  注意“密度高度离几何高度很远”与“越界”是两回事：只要等效高度仍在 `[0, 20000]` 内，差多远都照常计算。

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

密度高度反解落到模型分层范围之外时，单点接口同样返回 `400`，但错误码为 `out_of_model_range`，
并带 `direction`（`below_sea_level` / `above_ceiling`）说明越界方向：

```json
{"error":"out_of_model_range","direction":"below_sea_level","reason":"density altitude inversion fell below the 0 m model floor (equivalent altitude -4669.8 m): ..."}
```

批量接口对越界点做**逐点隔离**：整批仍返回 `200`，每个采样位置都保留在 `points` 数组中（槽位与
索引不丢）；越界点是一个结构化错误对象（`status`/`error` 为 `out_of_model_range`，带 `reason`、
`direction` 和仅用于诊断的 `equivalent_density_altitude_m`），合法点保持原有完整结果形状不变；
顶层新增 `failed_count`。一个点越界既不会把野值混进结果，也不会连坐其它合法点。
只有请求本身的结构性错误（区间越界、步长非法等）才整批 `400`。

```json
{
  "count": 3, "failed_count": 1,
  "points": [
    {"index": 0, "altitude_m": 0, "status": "out_of_model_range", "error": "out_of_model_range", "direction": "below_sea_level", "reason": "...", "equivalent_density_altitude_m": -4669.83},
    {"altitude_m": 10000, "temperature_offset_k": -100, "...": "正常点，形状不变"},
    {"altitude_m": 20000, "...": "正常点"}
  ]
}
```

## 边界与拒算规则

**输入几何高度**：

- `h < 0`（低于海平面）：非法；
- `h > 20000 m`（超过实现上限）：非法；
- 非有限高度/偏差、非正步长、`end < start`、点数超 100000：非法；
- 服务**绝不外推**到 20 km 以上的更高层大气给一个无物理意义的数字。

**密度高度反解输出**：

- 反解等效高度 `< 0 m`（密度比海平面标准空气稠密，极冷场景）：`out_of_model_range` 结构化错误；
- 反解等效高度 `> 20000 m`（密度比顶层标准空气稀薄，极热场景）：`out_of_model_range` 结构化错误；
- 恰好落在 0 m / 20000 m 边界上：合法，边界本身属于模型覆盖范围；
- 单点接口对该错误返回 `400`；批量接口只把对应采样点标记为失败（`failed_count` 计数），其它点照常。

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

docker build -t isa-service . # 基础镜像 golang:1.22-alpine；构建过程在容器内跑完全部 go test，测试不过则镜像构建失败
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
- 非法高度（负、超 20 km、非有限）一律返回结构化错误；
- 偏冷使反解等效高度落到 0 m 以下、偏热使之落到 20 km 以上：单点与批量接口都返回结构化
  `out_of_model_range` 说明；批量中仅越界采样点失败，合法点结果形状与数值不变；
- 等效高度在界内（即使与几何高度相差很远）的请求一律正常计算，边界检查不得误伤。
