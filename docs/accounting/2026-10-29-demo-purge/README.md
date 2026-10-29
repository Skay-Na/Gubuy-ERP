# 2026-10-29 演示数据清理（本地）

## 背景

2026-10-29 21:00–22:30，本地环境为验证工程模块上线造了一批演示/走查数据，特征为
`client_request_id` 以 `seed-` 或 `walkthrough-` 开头。生产环境从未造过演示数据
（engineering 6 表始终 0 行），因此本次**仅清理本地**。

## 清理范围与影响

| 对象 | 清理前 | 清理后 | 说明 |
|---|---|---|---|
| engineering_projects | 10 | 0 | 全部 seed-/walkthrough- 项目 |
| engineering_project_items | 10 | 0 | |
| engineering_receipts | 6 | 0 | |
| engineering_expenses | 18（含 3 条已软删） | 0 | 含 6 条无 project_id 的公共费用 |
| engineering_shipments | 3 | 0 | |
| engineering_payment_terms | 15 | 0 | |
| inbound_records 106/107 | 2 | 0 | 15 号 +4@6800、18 号 +4@7600 演示入库 |
| inventory_logs ≥546 | 13 | 0 | 演示入库 + 工程出库（含门店仓扣减） |
| operation_histories（21:00 后） | 431 | 0 | 演示走查操作留痕 |
| products 524 TEST-CAT-VERIFY-ITEM | 1 | 0 | 10-28 分类验证测试商品，零引用 |

**未受影响**：资金账户流水（演示期间 21:00 后无任何新增 financial_logs，工程收付款独立核算不进资金账户）；
订单与逐单成本（清理后本地 285 单与生产共有订单成本零差异）。

## 6 个受影响品种按生产真实值恢复

演示操作动了门店仓（warehouse_type=2），products 的库存/成本被带偏，统一按生产真实值回填：

| 品种 | avg_cost | latest_cost | main_stock | store_stock |
|---|---|---|---|---|
| 4 KFR-72W2W/BP2DN1-TR | 0.00 | 0.00 | 0 | 0 |
| 15 KFR-72T2W/B3-LX（乐享四代） | 5000.00 | 5000.00 | 3 | 1 |
| 18 DNL-V180/NSN8-615(雪焰) | 15800.00 | 15800.00 | 4 | 0 |
| 45 35C1-1S | 2949.00 | 2949.00 | 45 | 3 |
| 78 FSA25UB | 99.00 | 99.00 | 0 | 7 |
| 332 35GW/XA2 | 1850.00 | 1850.00 | 24 | 3 |

清理后商品全表与生产比对：除生产新增的 524/525（真实商品，本地无）外完全一致。

## 执行与校验

- 备份：`output/backup-local-pre-demo-clean-20261029.sql.gz`（58MB）
- 单事务执行 `purge_engineering_demo_20261029.sql`，末尾写 operation_histories 快照
- 校验：工程 6 表 0 行、GC- 库存引用 0、演示入库 0、测试商品 0；
  账户余额净额=余额全部成立（1:16333 / 2:22487 / 3:51481.30 / 4:35366 / 6:-306467.20 / 7:-222998.06）

## 遗留（历史问题，非本次引入）

- 78 号 FSA25UB 的 main_stock=0 而库存日志最后一条 main 仓 after_qty=10 —— 生产同样如此，两边一致，属历史不同步，未处理
