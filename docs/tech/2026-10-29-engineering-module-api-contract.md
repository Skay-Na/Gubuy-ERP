# 工程模块 接口契约

| 项 | 内容 |
| --- | --- |
| 文档编号 | API-ENG-001 |
| 版本 | v1.1（**冻结基线**，前后端并行开发的唯一依据） |
| 日期 | 2026-10-29 |
| 上游 | TECH-ENG-001 v1.1（技术方案）、SPEC-ENG-001 v1.1（产品设计） |
| 基础路径 | `/api/admin/engineering` |
| 认证 | 全部接口要求 `Authorization: Bearer <erp_token>` 且为管理员；员工 token 一律 403 |

**变更记录（v1.0 → v1.1，纯追加，无破坏性变更）**

| 变更 | 内容 |
| --- | --- |
| 新增接口 | 4 个费用接口（台账查询 / 新增 / 编辑 / 删除），接口总数 12 → **16** |
| 新增枚举 | `expense_category`、`pay_status`、`pay_method` |
| 字段追加 | `/summary` 追加 `year_expense`、`year_expense_count`、`avg_net_margin_rate`、`total_net_profit`；`/projects` 与 `/projects/:id` 追加 `expense_total`、`net_profit`、`net_margin_rate` |
| 详情聚合 | `/projects/:id` 响应追加 `expenses[]` |

> 契约变更规则：字段只能**追加**，不得改名或改类型；确需破坏性变更，版本号 +0.1 并在本文档登记。

---

## 1. 通用约定

### 1.1 响应包络（遵循 `docs/api-conventions.md`）

```json
{ "code": 200, "msg": "success", "data": { } }
```

- 列表接口在**顶层**附加 `total`（不是 data 里）；
- 成功默认 `msg: "success"`；带业务提示时用 `OKWithMsg`；
- 失败时 HTTP 状态码与 `code` 同值。

### 1.2 错误码

| HTTP | code | 场景 |
| --- | --- | --- |
| 200 | 200 | 成功 |
| 400 | 400 | 参数缺失/格式错、状态不允许、库存不足、出库超订购量、作废原因过短、删除条件不满足、回款金额超过节点剩余 |
| 403 | 403 | 非管理员 |
| 404 | 404 | 项目 / 节点 / 清单行 / 回款记录不存在 |
| 409 | 409 | 合同总额与付款节点合计不平、并发导致的超额回款、重复提交（幂等命中） |
| 500 | 500 | 数据库或内部错误 |

### 1.3 字段格式

| 类型 | 格式 | 示例 |
| --- | --- | --- |
| 金额 | JSON number，2 位小数 | `860000.00` |
| 日期（业务日期） | 字符串 `YYYY-MM-DD` | `"2026-10-29"` |
| 时间戳 | RFC3339 字符串 | `"2026-10-29T20:04:48+08:00"` |
| 比例/百分比 | number，最多 2 位小数 | `30` / `19.84` |
| 空值 | `null`（不用空字符串占位金额/日期） | `null` |

> 日期统一由后端格式化输出（DTO 层），前端**不需要**做 `slice(0,10)`。

### 1.4 枚举字典

| 字典 | 值 |
| --- | --- |
| `status` | 1 跟进中 / 2 已签约 / 3 供货中 / 4 已验收 / 5 已结清 / 9 已作废 |
| `project_type` | 1 住宅精装 / 2 商业综合体 / 3 酒店公寓 / 4 政企学校 / 5 其他 |
| `method`（收款方式） | 1 银行转账 / 2 承兑汇票 / 3 现金 / 4 支票 / 5 其他 |
| `warehouse_type` | 2 门店 / 3 云仓 |
| `action`（状态流转） | `sign` / `accept` / `revert` / `void` |
| **`expense_category`（费用类别）** | **1 安装人工 / 2 运输物流 / 3 辅材耗材 / 4 现场施工 / 5 差旅招待 / 6 外包服务 / 7 其他** |
| **`pay_status`（付款状态）** | **1 已付 / 0 未付（挂账）** |
| **`pay_method`（付款方式，仅备查）** | **0 未指定 / 1 微信 / 2 支付宝 / 3 公户 / 4 现金 / 5 其他** |

所有含枚举的响应对象同时返回 `xxx_label` 字段（如 `status_label`、`project_type_label`），前端直接展示，不在前端维护字典副本（避免两处不一致）。

### 1.5 分页

请求：`page`（默认 1）、`page_size`（默认 20，最大 100）。
响应：`data.list` + 顶层 `total`。

---

## 2. 接口清单

| # | 方法 | 路径 | 用途 | 幂等键 |
| --- | --- | --- | --- | --- |
| 1 | GET | `/summary` | 汇总卡 | — |
| 2 | GET | `/projects` | 列表 | — |
| 3 | POST | `/projects` | 创建 | ✅ |
| 4 | GET | `/projects/:id` | 详情（聚合） | — |
| 5 | PUT | `/projects/:id` | 更新主档 | — |
| 6 | DELETE | `/projects/:id` | 删除 | — |
| 7 | PUT | `/projects/:id/status` | 状态流转 | — |
| 8 | PUT | `/projects/:id/items` | 整表保存清单 | — |
| 9 | PUT | `/projects/:id/payment-terms` | 整表保存节点 | — |
| 10 | POST | `/projects/:id/receipts` | 登记回款 | ✅ |
| 11 | POST | `/projects/:id/receipts/:rid/reverse` | 冲销回款 | ✅ |
| 12 | POST | `/projects/:id/shipments` | 出库 | — |
| **13** | **GET** | **`/expenses`** | **工程费用台账（跨项目 + 公共费用）** | — |
| **14** | **POST** | **`/expenses`** | **新增费用（`project_id` 可空 = 公共费用）** | ✅ |
| **15** | **PUT** | **`/expenses/:eid`** | **编辑费用** | — |
| **16** | **DELETE** | **`/expenses/:eid`** | **删除费用（软删除）** | — |

> 费用接口路径**不带** `/projects/:id` 前缀：公共费用没有项目，两条路径会让前端同一个弹窗要写两套提交逻辑。统一在 `/expenses`，用 `project_id` 区分。

---

## 3. 接口定义

### 3.1 GET `/summary` — 汇总卡

**Query**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `include_void` | bool | 否 | 默认 false（汇总恒不含作废，此参数仅保留给未来用，一期忽略） |

**响应**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `active_count` | int | 状态 1/2/3 的项目数 |
| `active_amount` | number | 上述项目合同额合计（状态 1 无合同时用 `estimated_amount`） |
| `pending_amount` | number | Σ(合同额 − 已实收)，状态 1~4 |
| `pending_project_count` | int | 待回款 > 0 的项目数 |
| `overdue_amount` | number | 逾期节点未收金额合计 |
| `overdue_term_count` | int | 逾期节点数 |
| `year_received` | number | 本自然年回款合计（`payment_date` 在今年，含冲销负数） |
| `year_receipt_count` | int | 本年回款笔数 |
| `avg_margin_rate` | number | Σ设备毛利 / Σ合同额 × 100（仅状态 ≥2），保留 2 位。**v1.1 保留但前端不再展示**（改用 `avg_net_margin_rate`） |
| `gross_profit` | number | 上述项目的**设备毛利**合计（v1.1 保留字段，语义不变） |
| **`year_expense`** | number | **本自然年工程费用合计（项目费用 + 公共费用，已付未付都算），`expense_date` 在今年的** |
| **`year_expense_count`** | number | **本年费用笔数** |
| **`avg_net_margin_rate`** | number | **Σ项目净利 / Σ合同额 × 100（仅状态 ≥2，不含公共费用），保留 2 位** |
| **`total_net_profit`** | number | **上述项目的项目净利合计 = Σ(合同额 − 设备成本 − 本项目费用)** |

```json
{
  "code": 200, "msg": "success",
  "data": {
    "active_count": 7, "active_amount": 3942400.00,
    "pending_amount": 1124000.00, "pending_project_count": 5,
    "overdue_amount": 246000.00, "overdue_term_count": 3,
    "year_received": 2818400.00, "year_receipt_count": 14,
    "avg_margin_rate": 17.40, "gross_profit": 686000.00,
    "year_expense": 128600.00, "year_expense_count": 37,
    "avg_net_margin_rate": 12.18, "total_net_profit": 480200.00
  }
}
```

> 口径提醒（前端 tooltip 直接用）：`total_net_profit` **不含公共费用**，而 `year_expense` **含**公共费用。两个数放在同一排卡片上，口径差异必须在副文案里写明，否则老板必然发现"减法对不上"。

---

### 3.2 GET `/projects` — 列表

**Query**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `keyword` | string | 否 | 模糊匹配 project_no / name / client_name / client_contact / client_phone，上限 50 字 |
| `status` | string | 否 | 逗号分隔多值，如 `1,2,3`；不传=全部（不含作废） |
| `project_type` | int | 否 | 单值 |
| `owner_id` | int | 否 | 负责人员工 ID |
| `sign_start` / `sign_end` | string | 否 | `YYYY-MM-DD` |
| `only_overdue` | bool | 否 | true 时仅返回存在逾期节点的项目 |
| `include_void` | bool | 否 | true 时包含状态 9 |
| `page` / `page_size` | int | 否 | 见 1.5 |
| `sort_by` | string | 否 | `updated_at`(默认) / `contract_total` / `pending_amount` / `next_due_date` / **`net_margin_rate`** |
| `order` | string | 否 | `desc`(默认) / `asc` |

**响应 `data.list[]`**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | int | |
| `project_no` | string | |
| `name` | string | |
| `project_type` / `project_type_label` | int / string | |
| `client_name` / `client_contact` / `client_phone` | string | |
| `owner_employee_id` / `owner_name` | int / string | owner_name 为空串表示未设置 |
| `contract_total` | number | 状态 1 且无清单时为 0，前端展示 `estimated_amount` |
| **`cost_total`** | number | **设备成本合计（快照）** |
| **`expense_total`** | number | **本项目工程费用合计（不含公共费用）** |
| `estimated_amount` | number | 跟进阶段预估额 |
| `received_amount` | number | 已实收 |
| `pending_amount` | number | `contract_total - received_amount`，下限 0 |
| `received_ratio` | number | 0~100 整数 |
| `status` / `status_label` | int / string | |
| `ordered_qty` / `shipped_qty` | int | 台数 |
| `overdue_amount` | number | 该项目的逾期未收合计 |
| `overdue_days` | int | 最大逾期天数，无逾期为 0 |
| `next_due_date` | string\|null | 下一个未收节点的计划日期 |
| `sign_date` | string\|null | |
| **`net_profit`** | number | **`contract_total - cost_total - expense_total`** |
| **`net_margin_rate`** | number | **`net_profit / contract_total × 100`，合同额为 0 时返回 0；保留 2 位** |
| `updated_at` | string | RFC3339 |

```json
{
  "code": 200, "msg": "success",
  "total": 37,
  "data": {
    "list": [{
      "id": 1, "project_no": "GC-20260920-001",
      "name": "绿城桂语 3 期精装房空调工程",
      "project_type": 1, "project_type_label": "住宅精装",
      "client_name": "绿城置业有限公司", "client_contact": "张经理", "client_phone": "13800008000",
      "owner_employee_id": 3, "owner_name": "王冲",
      "contract_total": 860000.00, "estimated_amount": 0,
      "cost_total": 690000.00, "expense_total": 46800.00,
      "net_profit": 123200.00, "net_margin_rate": 14.33,
      "received_amount": 516000.00, "pending_amount": 344000.00, "received_ratio": 60,
      "status": 3, "status_label": "供货中",
      "ordered_qty": 86, "shipped_qty": 52,
      "overdue_amount": 0, "overdue_days": 0,
      "next_due_date": "2026-11-15", "sign_date": "2026-09-20",
      "updated_at": "2026-10-28T15:22:10+08:00"
    }]
  }
}
```

---

### 3.3 POST `/projects` — 创建

**Body**

| 字段 | 类型 | 必填 | 校验 |
| --- | --- | --- | --- |
| `project_no` | string | 否 | 空则后端生成 `GC-YYYYMMDD-NNN`；非空则 ≤50 字、唯一 |
| `name` | string | ✅ | 1~100 字 |
| `project_type` | int | ✅ | 1~5 |
| `client_name` | string | ✅ | 1~150 字 |
| `client_contact` | string | ✅ | 1~50 字 |
| `client_phone` | string | ✅ | 手机 `^1[3-9]\d{9}$` 或座机 `^0\d{2,3}-?\d{7,8}$` |
| `address` | string | 否 | ≤255 字 |
| `owner_employee_id` | int | ✅ | 必须存在且未删除 |
| `contract_no` | string | 否 | ≤50 字 |
| `estimated_amount` | number | 否 | ≥0，2 位小数 |
| `sign_date` | string | 否 | `YYYY-MM-DD` |
| `remark` | string | 否 | ≤500 字 |
| `client_request_id` | string | 否 | ≤64 字，唯一；重复则 409 |

**响应 data**：`{ "id": 12, "project_no": "GC-20261029-001" }`

**错误**：400 校验失败（`msg` 指明字段，如 `联系电话格式不正确`）；409 编号占用 / 幂等命中（`data` 带已存在项目的 `id` 与 `project_no`）。

---

### 3.4 GET `/projects/:id` — 详情（聚合）

**响应 data**

```json
{
  "id": 1, "project_no": "GC-20260920-001", "name": "…",
  "project_type": 1, "project_type_label": "住宅精装",
  "client_name": "…", "client_contact": "…", "client_phone": "…", "address": "…",
  "owner_employee_id": 3, "owner_name": "王冲",
  "contract_no": "HT-2026-0912", "sign_date": "2026-09-20",
  "accept_date": null, "settled_at": null,
  "estimated_amount": 0, "items_total": 820000.00,
  "install_fee": 30000.00, "freight_fee": 15000.00, "other_fee": 0, "discount_amount": 5000.00,
  "contract_total": 860000.00, "cost_total": 690000.00, "expense_total": 46800.00,
  "gross_profit": 170000.00, "margin_rate": 19.77,
  "net_profit": 123200.00, "net_margin_rate": 14.33,
  "received_amount": 516000.00, "pending_amount": 344000.00, "received_ratio": 60,
  "ordered_qty": 86, "shipped_qty": 52, "shipped_ratio": 60,
  "status": 3, "status_label": "供货中",
  "overdue_amount": 86000.00, "overdue_term_count": 1,
  "void_reason": "", "voided_at": null, "remark": "",
  "created_at": "2026-09-20T10:12:00+08:00", "updated_at": "2026-10-28T15:22:10+08:00",

  "items": [{
    "id": 11, "product_id": 42, "product_name": "美的 KFR-35GW 变频挂机",
    "product_sku": "KFR-35GW/N8XHC1",
    "quantity": 10, "unit_price": 2600.00, "unit_cost": 2180.00,
    "amount": 26000.00, "cost_amount": 21800.00, "gross_profit": 4200.00, "margin_rate": 16.15,
    "shipped_qty": 6, "remark": "", "sort_order": 0,
    "cost_missing": false
  }],

  "terms": [{
    "id": 5, "name": "预付款", "ratio": 30.00,
    "planned_amount": 258000.00, "planned_date": "2026-09-25",
    "received_amount": 258000.00, "pending_amount": 0,
    "status": "paid", "status_label": "已收",
    "overdue_days": 0
  }],

  "receipts": [{
    "id": 88, "term_id": 5, "term_name": "预付款",
    "amount": 258000.00, "payment_date": "2026-09-26", "method": 1, "method_label": "银行转账",
    "bill_no": "", "remark": "", "is_reversal": false,
    "reversed_receipt_id": null, "operator_name": "王冲",
    "created_at": "2026-09-26T14:30:00+08:00"
  }],

  "shipments": [{
    "id": 31, "item_id": 11, "product_name": "美的 KFR-35GW 变频挂机",
    "quantity": 6, "warehouse_type": 2, "warehouse_label": "门店",
    "ship_date": "2026-10-10", "tracking_no": "", "remark": "",
    "allow_negative": false, "operator_name": "王冲",
    "created_at": "2026-10-10T09:20:00+08:00"
  }],

  "expenses": [{
    "id": 201, "project_id": 1, "project_name": "绿城桂语 3 期精装房空调工程",
    "category": 1, "category_label": "安装人工",
    "amount": 12000.00, "expense_date": "2026-10-25",
    "pay_status": 1, "pay_status_label": "已付",
    "pay_method": 1, "pay_method_label": "微信",
    "paid_date": "2026-10-25", "payee": "李师傅",
    "bill_no": "", "remark": "3 人 5 天",
    "operator_name": "王冲", "created_at": "2026-10-25T18:00:00+08:00"
  }],

  "expense_summary": {
    "total": 46800.00, "paid": 40800.00, "unpaid": 6000.00, "count": 9,
    "by_category": [
      { "category": 1, "category_label": "安装人工", "amount": 12000.00 },
      { "category": 2, "category_label": "运输物流", "amount": 18000.00 },
      { "category": 3, "category_label": "辅材耗材", "amount": 8800.00 },
      { "category": 4, "category_label": "现场施工", "amount": 8000.00 },
      { "category": 5, "category_label": "差旅招待", "amount": 0 },
      { "category": 6, "category_label": "外包服务", "amount": 0 },
      { "category": 7, "category_label": "其他", "amount": 0 }
    ]
  }
}
```

> `expenses` 只返回 `project_id = 本项目的` 费用，**公共费用不出现在项目详情里**。`by_category` 恒返回 7 项（无金额也返回 0），前端直接渲染 chip，不必自己补零。

**派生字段说明**

| 字段 | 算法 |
| --- | --- |
| `gross_profit`（项目） | `contract_total - cost_total`（**设备毛利**） |
| `margin_rate` | `gross_profit / contract_total × 100`，合同额为 0 时返回 0 |
| **`net_profit`** | **`contract_total - cost_total - expense_total`（项目净利）** |
| **`net_margin_rate`** | **`net_profit / contract_total × 100`，合同额为 0 时返回 0** |
| `pending_amount` | `max(contract_total - received_amount, 0)` |
| `terms[].status` | `paid` 已收 / `partial` 部分收 / `unpaid` 未收 / `overdue` 逾期（`pending>0 且 planned_date<今天`） |
| `items[].cost_missing` | `unit_cost <= 0` |
| `receipts[].is_reversal` | `amount < 0` |

**错误**：404。

---

### 3.5 PUT `/projects/:id` — 更新主档

**Body**：同 3.3 的全部可编辑字段（除 `project_no`、`client_request_id`），全部字段按"传入即覆盖"处理，未传字段保持不变（使用指针接收）。

额外规则：

| 规则 | 违反响应 |
| --- | --- |
| 状态 5（已结清）只允许改 `client_contact`、`client_phone`、`remark` | 400 `项目已结清，金额字段不可修改` |
| 状态 9（已作废）不可编辑 | 400 `项目已作废，不可编辑` |
| 修改 `install_fee` / `freight_fee` / `other_fee` / `discount_amount` 导致合同额变化，且状态 ≥2 且新合同额 ≠ 节点合计 | 409 `合同总额与付款节点合计相差 ¥10,000.00，请同时调整付款节点` |

**响应 data**：`{ "id": 1, "contract_total": 860000.00, "pending_amount": 344000.00 }`

---

### 3.6 DELETE `/projects/:id` — 删除

**规则**：仅 `status=1` 且无回款记录、无出库记录；软删除。

**响应 data**：`{}`，`msg: "项目已删除"`

**错误**：400 `仅跟进中且无回款、无出库的项目可删除`；404。

---

### 3.7 PUT `/projects/:id/status` — 状态流转

**Body**

| `action` | 附加字段 | 说明 |
| --- | --- | --- |
| `sign` | `sign_date`（必填，`YYYY-MM-DD`）、`contract_no`（可选） | 1 → 2 |
| `accept` | `accept_date`（必填）、`remark`（可选） | 3 → 4 |
| `revert` | `reason`（必填 ≥5 字） | 2 → 1 或 3 → 2 |
| `void` | `reason`（必填 ≥5 字） | 任意 → 9（状态 5 除外） |

```json
{ "action": "sign", "sign_date": "2026-09-20", "contract_no": "HT-2026-0912" }
```

**响应 data**

```json
{ "id": 1, "status": 2, "status_label": "已签约", "sign_date": "2026-09-20" }
```

**错误**

| 场景 | 响应 |
| --- | --- |
| 清单为空 | 400 `设备清单为空，无法签约` |
| 合同额 ≤ 0 | 400 `合同总额必须大于 0` |
| 无付款节点 | 400 `请先配置付款节点` |
| 节点合计 ≠ 合同额 | 409 `付款节点合计与合同总额相差 ¥10,000.00` |
| 未全部发货即验收 | 400 `还有 34 台未发货，无法验收` |
| 有出库记录时 3→2 | 400 `项目已有出库记录，不可回退到已签约` |
| 非法流转 | 400 `当前状态不允许该操作` |
| 已结清项目作废 | 400 `已结清项目不可作废` |

---

### 3.8 PUT `/projects/:id/items` — 整表保存清单

**Body**

```json
{
  "items": [
    { "id": 0, "product_id": 42, "quantity": 10, "unit_price": 2600.00, "unit_cost": 2180.00, "remark": "" },
    { "id": 11, "product_id": 43, "quantity": 6, "unit_price": 3200.00, "unit_cost": 2700.00, "remark": "含 3 米铜管" }
  ]
}
```

- `items` **全量替换**：`id=0` 为新增行，`id>0` 为保留行（未出现在列表中的行被删除）；
- 每条校验：`product_id` 存在；`quantity ≥ 1` 且 `≥ 该行已出库数量`；`unit_price > 0`；`unit_cost ≥ 0`；
- 已出库行（`shipped_qty > 0`）不允许更换 `product_id`；
- 清单为空数组时允许（用于清空，但状态 ≥2 时禁止清空）。

**响应 data**

```json
{
  "items_total": 820000.00, "cost_total": 690000.00,
  "contract_total": 860000.00, "ordered_qty": 86,
  "gross_profit": 170000.00, "margin_rate": 19.77,
  "terms_total": 860000.00, "balanced": true
}
```

**错误**

| 场景 | 响应 |
| --- | --- |
| 数量小于已出库数 | 400 `第 2 行数量不能小于已出库数量 6` |
| 已出库行换商品 | 400 `已出库的商品不可替换` |
| 状态 ≥2 时清空清单 | 400 `已签约项目不可清空设备清单` |
| 状态 ≥2 且新合同额 ≠ 节点合计 | 409 `合同总额调整为 ¥870,000.00，与付款节点合计相差 ¥10,000.00，请同时调整付款节点` |

---

### 3.9 PUT `/projects/:id/payment-terms` — 整表保存节点

**Body**

```json
{
  "terms": [
    { "id": 0, "name": "预付款", "ratio": 30, "planned_amount": 258000.00, "planned_date": "2026-09-25" },
    { "id": 5, "name": "验收款", "ratio": 70, "planned_amount": 602000.00, "planned_date": "2026-12-15" }
  ]
}
```

- 全量替换语义同 3.8；
- `id>0` 的节点若已有回款记录（`received_amount > 0`），**不可删除、不可把 `planned_amount` 改到小于已收金额**；
- `name` 1~50 字；`planned_amount > 0`；`ratio` 可空，0~100；`planned_date` 可空。

**响应 data**

```json
{
  "terms_total": 860000.00, "contract_total": 860000.00,
  "balanced": true, "diff": 0,
  "terms": [{ "id": 5, "name": "预付款", "planned_amount": 258000.00, "received_amount": 258000.00, "status": "paid", "status_label": "已收", "overdue_days": 0 }]
}
```

**错误**

| 场景 | 响应 |
| --- | --- |
| 删除有回款的节点 | 400 `节点「预付款」已有回款记录，不可删除` |
| 计划金额小于已收 | 400 `节点「预付款」计划金额不能小于已收金额 ¥258,000.00` |
| 状态 ≥2 且合计 ≠ 合同额 | 409 `付款节点合计 ¥850,000.00 与合同总额 ¥860,000.00 相差 ¥10,000.00` |

---

### 3.10 POST `/projects/:id/receipts` — 登记回款

**Body**

| 字段 | 类型 | 必填 | 校验 |
| --- | --- | --- | --- |
| `term_id` | int | ✅ | 必须属于该项目 |
| `amount` | number | ✅ | > 0，≤ 节点剩余应收 |
| `payment_date` | string | ✅ | `YYYY-MM-DD`，不得晚于今天 |
| `method` | int | ✅ | 1~5 |
| `bill_no` | string | 否 | ≤50 字 |
| `remark` | string | 否 | ≤200 字 |
| `client_request_id` | string | 否 | ≤64 字，唯一 |

**响应 data**

```json
{
  "receipt_id": 88,
  "project_received": 774000.00,
  "project_pending": 86000.00,
  "term_received": 258000.00,
  "term_pending": 0,
  "project_status": 3,
  "project_status_label": "供货中",
  "settled": false
}
```

**错误**

| 场景 | 响应 |
| --- | --- |
| 金额 > 节点剩余（预检） | 400 `不能超过该节点剩余应收 ¥86,000.00` |
| 并发导致条件更新失败 | 409 `该节点应收已变化，请刷新后重试` |
| 到账日期晚于今天 | 400 `到账日期不能晚于今天` |
| 幂等命中 | 409 `该笔回款已登记` |
| 项目已作废 | 400 `项目已作废，不可登记回款` |

---

### 3.11 POST `/projects/:id/receipts/:rid/reverse` — 冲销回款

**Body**

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `reason` | string | ✅ | 5~200 字，写入冲销记录的 `remark`（前缀 `冲销：`） |
| `payment_date` | string | 否 | 默认今天 |
| `client_request_id` | string | 否 | 幂等键 |

**规则**

- 只能冲销正数记录（`amount > 0`），且未被冲销过（`reversed_receipt_id` 无指向它的记录）；
- 冲销金额 = 原记录金额的相反数（不支持部分冲销，需要部分冲销时先调整节点计划再操作）；
- 冲销后若项目已结清（status=5）且已收 < 合同额，状态回退到 4（已验收）；若项目未验收则回退到 3 或 2（按发货情况）；
- 写新记录，`reversed_receipt_id` 指向原记录。

**响应 data**

```json
{
  "receipt_id": 89, "amount": -258000.00,
  "project_received": 258000.00, "project_pending": 602000.00,
  "project_status": 3, "project_status_label": "供货中"
}
```

**错误**：404 记录不存在；400 `该记录已冲销` / `负数记录不可冲销`；400 项目已作废。

---

### 3.12 POST `/projects/:id/shipments` — 出库

**Body**

| 字段 | 类型 | 必填 | 校验 |
| --- | --- | --- | --- |
| `warehouse_type` | int | ✅ | 2 门店 / 3 云仓 |
| `ship_date` | string | ✅ | `YYYY-MM-DD`，不得晚于今天 |
| `tracking_no` | string | 否 | ≤100 字 |
| `remark` | string | 否 | ≤200 字 |
| `allow_negative` | bool | 否 | 默认 false |
| `lines` | array | ✅ | 至少 1 行 |
| `lines[].item_id` | int | ✅ | 属于该项目 |
| `lines[].quantity` | int | ✅ | ≥1 且 ≤ (订购 − 已出库) |

```json
{
  "warehouse_type": 2, "ship_date": "2026-10-30", "tracking_no": "SF1234567890",
  "remark": "", "allow_negative": false,
  "lines": [{ "item_id": 11, "quantity": 6 }, { "item_id": 12, "quantity": 4 }]
}
```

**响应 data**

```json
{
  "shipment_count": 2, "total_qty": 10,
  "project_status": 3, "project_status_label": "供货中",
  "shipped_qty": 62, "ordered_qty": 86, "shipped_ratio": 72
}
```

**错误**

| 场景 | 响应 |
| --- | --- |
| 数量超待发 | 400 `本次出库不能超过待发数量 34 台` |
| 库存不足且未确认 | 400 `商品「美的 KFR-35GW 变频挂机」门店库存不足（当前 3 台），如需继续请确认负库存出库` |
| 项目已作废 | 400 `项目已作废，不可出库` |
| 项目状态为 1（未签约） | 400 `项目尚未签约，不可出库` |
| 出库日期晚于今天 | 400 `出库日期不能晚于今天` |

---

### 3.13 GET `/expenses` — 工程费用台账

**Query**

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `project_id` | string | 否 | 项目 ID；传 `none` 表示只看公共费用；不传 = 全部（含公共费用） |
| `category` | string | 否 | 逗号分隔，如 `1,2`；不传 = 全部七类 |
| `date_start` / `date_end` | string | 否 | `YYYY-MM-DD`，按 `expense_date` 过滤 |
| `pay_status` | int | 否 | 1 已付 / 0 未付；不传 = 全部 |
| `keyword` | string | 否 | 模糊匹配：项目名 / `payee` / `remark`，上限 50 字 |
| `page` / `page_size` | int | 否 | 见 1.5 |
| `sort_by` | string | 否 | `expense_date`(默认) / `amount` / `created_at` |
| `order` | string | 否 | `desc`(默认) / `asc` |

**响应 `data.list[]`**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | int | 费用 ID |
| `project_id` | int\|null | **null = 公共费用** |
| `project_no` / `project_name` | string | 公共费用时为空串 `""` |
| `is_public` | bool | `project_id == null` 时为 true，前端用灰色 chip |
| `category` / `category_label` | int / string | 七类 |
| `amount` | number | 恒为正 |
| `expense_date` | string | `YYYY-MM-DD` |
| `pay_status` / `pay_status_label` | int / string | `1` → `已付`，`0` → `未付` |
| `pay_method` / `pay_method_label` | int / string | 0 时 label 为 `—` |
| `paid_date` | string\|null | |
| `payee` / `bill_no` / `remark` | string | |
| `operator_name` | string | 快照 |
| `created_at` | string | RFC3339 |

**响应同时返回 `data.summary` 与 `data.by_category`（顶层放 `total`）**

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `summary.total` | number | 当前筛选范围内合计 |
| `summary.paid` / `summary.unpaid` | number | 已付 / 未付合计 |
| `summary.count` | int | 笔数 |
| `summary.year_total` | number | **本自然年合计，不受筛选影响（固定口径）** |
| `by_category[]` | array | 恒 7 项，`{category, category_label, amount}`，无金额返回 0 |

```json
{
  "code": 200, "msg": "success", "total": 37,
  "data": {
    "list": [
      {
        "id": 201, "project_id": 1, "project_no": "GC-20260920-001",
        "project_name": "绿城桂语 3 期精装房空调工程", "is_public": false,
        "category": 1, "category_label": "安装人工",
        "amount": 12000.00, "expense_date": "2026-10-25",
        "pay_status": 1, "pay_status_label": "已付",
        "pay_method": 1, "pay_method_label": "微信",
        "paid_date": "2026-10-25", "payee": "李师傅",
        "bill_no": "", "remark": "3 人 5 天",
        "operator_name": "王冲", "created_at": "2026-10-25T18:00:00+08:00"
      },
      {
        "id": 205, "project_id": null, "project_no": "", "project_name": "",
        "is_public": true,
        "category": 2, "category_label": "运输物流",
        "amount": 2000.00, "expense_date": "2026-10-26",
        "pay_status": 1, "pay_status_label": "已付",
        "pay_method": 4, "pay_method_label": "现金",
        "paid_date": "2026-10-26", "payee": "货车王师傅",
        "bill_no": "", "remark": "一批工程共用一趟车",
        "operator_name": "王冲", "created_at": "2026-10-26T19:10:00+08:00"
      }
    ],
    "summary": { "total": 128600.00, "paid": 112600.00, "unpaid": 16000.00, "count": 37, "year_total": 96400.00 },
    "by_category": [
      { "category": 1, "category_label": "安装人工", "amount": 42000.00 },
      { "category": 2, "category_label": "运输物流", "amount": 38000.00 },
      { "category": 3, "category_label": "辅材耗材", "amount": 22000.00 },
      { "category": 4, "category_label": "现场施工", "amount": 14000.00 },
      { "category": 5, "category_label": "差旅招待", "amount": 8600.00 },
      { "category": 6, "category_label": "外包服务", "amount": 2000.00 },
      { "category": 7, "category_label": "其他", "amount": 2000.00 }
    ]
  }
}
```

> `project_id=none` 时 `summary` / `by_category` 只统计公共费用。**按具体项目筛选时，公共费用不出现在结果里**——这是"公共费用不摊入项目"在接口层的体现。

---

### 3.14 POST `/expenses` — 新增费用

**Body**

| 字段 | 类型 | 必填 | 校验 |
| --- | --- | --- | --- |
| `project_id` | int\|null | 否 | null/不传 = 公共费用；传值时项目必须存在且 `status != 9` |
| `category` | int | ✅ | 1~7 |
| `amount` | number | ✅ | `> 0`，`≤ 9,999,999.99`，2 位小数（后端 `roundMoney` 归一） |
| `expense_date` | string | ✅ | `YYYY-MM-DD`，不得晚于今天 |
| `pay_status` | int | ✅ | 1 已付 / 0 未付 |
| `pay_method` | int | `pay_status=1` 时必填 | 0~5 |
| `paid_date` | string | 否 | `pay_status=1` 时默认 = `expense_date` |
| `payee` | string | 否 | ≤100 字 |
| `bill_no` | string | 否 | ≤50 字 |
| `remark` | string | `category=7` 时必填 | ≤200 字 |
| `client_request_id` | string | 否 | ≤64 字，唯一 |

**响应 data**

```json
{
  "expense_id": 201,
  "project_expense_total": 46800.00,
  "net_profit": 123200.00,
  "net_margin_rate": 14.33
}
```

> 公共费用时 `project_expense_total` / `net_profit` / `net_margin_rate` 均返回 `null`（没有项目可重算），前端据此跳过净利刷新。

**错误**

| 场景 | 响应 |
| --- | --- |
| `amount <= 0` 或超上限 | 400 `费用金额必须大于 0` |
| `expense_date` 晚于今天 | 400 `发生日期不能晚于今天` |
| `category` 不在 1~7 | 400 `费用类别不正确` |
| `category=7` 且 `remark` 空 | 400 `「其他」类别请填写备注，说明这笔钱花在哪` |
| `pay_status=1` 且 `pay_method` 缺失 | 400 `请选择付款方式` |
| 项目不存在 | 404 `项目不存在` |
| 项目已作废 | 400 `项目已作废，不可新增费用` |
| 幂等命中 | 409 `该笔费用已记录` |

---

### 3.15 PUT `/expenses/:eid` — 编辑费用

**Body**：同 3.14 全部字段（`client_request_id` 除外），**传入即覆盖**。

**额外规则**

| 规则 | 违反响应 |
| --- | --- |
| 费用不存在或已删除 | 404 |
| 原费用所属项目已作废（`status=9`） | 400 `项目已作废，不可修改费用` |
| 修改 `project_id` 为目标项目且目标已作废 | 400 `项目已作废，不可挂载费用` |

**响应 data**：同 3.14（`project_expense_total` 为**变更后**所属项目的合计，公共费用的净利字段为 `null`）

> 编辑时若 `project_id` 从 A 改为 B，A 与 B 的 `expense_total` **都要**重算，后端按 ID 升序锁两行，避免死锁。

---

### 3.16 DELETE `/expenses/:eid` — 删除费用

**规则**

- 软删除（写 `deleted_at`），`operation_histories` 留 before 快照；
- 所属项目 `expense_total` 同事务重算；
- 公共费用删除不涉及任何项目。

**响应 data**

```json
{ "expense_id": 201, "project_expense_total": 34800.00, "net_profit": 135200.00 }
```

`msg`：`费用已删除`

**错误**：404 `费用不存在`；400 `项目已作废，不可删除费用`。

---

## 4. 前端调用映射（`src/api/engineering.js`）

```js
import request from '../utils/request'

export const fetchEngineeringSummary = params => request.get('/api/admin/engineering/summary', { params })
export const fetchEngineeringProjects = params => request.get('/api/admin/engineering/projects', { params })
export const createEngineeringProject = data => request.post('/api/admin/engineering/projects', data)
export const getEngineeringProject = id => request.get(`/api/admin/engineering/projects/${id}`)
export const updateEngineeringProject = (id, data) => request.put(`/api/admin/engineering/projects/${id}`, data)
export const deleteEngineeringProject = id => request.delete(`/api/admin/engineering/projects/${id}`)
export const updateEngineeringStatus = (id, data) => request.put(`/api/admin/engineering/projects/${id}/status`, data)
export const saveEngineeringItems = (id, data) => request.put(`/api/admin/engineering/projects/${id}/items`, data)
export const saveEngineeringTerms = (id, data) => request.put(`/api/admin/engineering/projects/${id}/payment-terms`, data)
export const createEngineeringReceipt = (id, data) => request.post(`/api/admin/engineering/projects/${id}/receipts`, data)
export const reverseEngineeringReceipt = (id, rid, data) => request.post(`/api/admin/engineering/projects/${id}/receipts/${rid}/reverse`, data)
export const createEngineeringShipment = (id, data) => request.post(`/api/admin/engineering/projects/${id}/shipments`, data)

// 工程费用（v1.1）
export const fetchEngineeringExpenses = params => request.get('/api/admin/engineering/expenses', { params })
export const createEngineeringExpense = data => request.post('/api/admin/engineering/expenses', data)
export const updateEngineeringExpense = (expenseId, data) => request.put(`/api/admin/engineering/expenses/${expenseId}`, data)
export const deleteEngineeringExpense = expenseId => request.delete(`/api/admin/engineering/expenses/${expenseId}`)
```

**前端约定**

- 幂等键：进入建档抽屉 / 打开回款弹窗 / **打开费用弹窗**时生成 `crypto.randomUUID()`，提交成功后重置；
- 所有金额展示走统一格式化函数（`¥` + 千分位 + 2 位）；
- 接口返回 `code === 200` 才视为成功（`request.js` 拦截器已统一处理 401，其余业务错误由调用方 `ElMessage.error(msg)`）；
- **费用保存/删除成功后，从响应里取 `project_expense_total` / `net_profit` 直接更新详情页与列表，不要重新拉整个项目**（省一次请求，也避免刷新时列表闪动）；公共费用响应里这三个字段为 `null`，此时跳过。

---

## 5. 契约自检清单（开发完成后逐条核对）

- [ ] **16 个**接口全部实现，路径、方法、参数名与本文档逐字一致
- [ ] 所有接口均返回 `{code,msg,data}` 包络，列表接口顶层有 `total`
- [ ] 所有枚举字段均附带 `xxx_label`（含 `category_label` / `pay_status_label` / `pay_method_label`）
- [ ] 所有日期字段为 `YYYY-MM-DD` 字符串，时间戳为 RFC3339
- [ ] 所有金额为 2 位小数 number
- [ ] 错误码与第 1.2 节一致，`msg` 为可直接展示的中文整句
- [ ] 员工 token 访问任一接口返回 403
- [ ] 幂等键重复提交返回 409 且数据只写一次
- [ ] **`GET /expenses` 的 `by_category` 恒返回 7 项（含 0 金额项）**
- [ ] **`project_id=none` 只返回公共费用；按具体项目筛选时不含公共费用**
- [ ] **费用写操作响应返回 `project_expense_total` / `net_profit`，公共费用时三者均为 `null`**
- [ ] **费用接口全程不产生 `financial_logs` 记录、不改 `accounts` 余额**

---

**本契约一经确认即为冻结基线**，第四阶段的任务卡将按本文档逐接口派发，开发不得自行改字段。
