# 2026-10-29 补录董庆庆离职日期（消除虚假应发）

## 问题

董庆庆（员工 id=11）在工资条上出现 **「2026-09 应发 2,000 / 未发」** 的虚假欠款，
10 月还挂着一条 upcoming 的 2,000。

但他实际只干了几天：

| 事实 | 数据 |
|---|---|
| 入职 | 2026-08-12（`entry_date`） |
| 打卡记录 | 2026-08-14 ~ 2026-08-20，共 7 天 |
| 发薪 | 2026-08-21 领 550（流水 `PAY-2026-08-11`） |
| 账号删除 | 2026-10-03 13:10（`deleted_at`） |
| 业绩 | 0 单 |

**根因**：`deleted_at`（10-03，账号清理时间）被 `EmploymentEndDate()` 当作在职终点，
而 `last_working_date` 一直是空。于是 `employeeWorkedDuringMonth()` 判定他 9 月、
10 月仍在职，薪资报表照常给他生成「底薪 2,000 / 无提成 / 未发」的条目。

## 处理

```sql
UPDATE employees
SET last_working_date = '2026-08-20',   -- 最后打卡日（8-21 是发薪日，无打卡）
    employment_status  = 'departed'
WHERE name = '董庆庆' AND entry_date = '2026-08-12' AND last_working_date IS NULL;
```

- 本地 + 生产执行同一份 SQL（业务键守卫，不用裸 id；本地/生产该员工 id 均为 11）
- `operation_histories` 留快照（生产 #104），记录改动前后值

## 效果

| 月份 | 改前 | 改后 |
|---|---|---|
| 2026-08 | 应发 516 / 实付 550 / 差 -34 | **不变**（见下） |
| 2026-09 | 应发 2,000 / 实付 0 / **差 2,000** | **整月消失** |
| 2026-10 | 应发 2,000 / upcoming | **整月消失** |

**8 月为什么不变**：8 月存在人工月度考勤（`monthly_attendances` leave_days=27），
`monthlyEmployeePayrollReport` 里人工假天数优先级高于 `countAbsentDays()` 的自动推算，
所以改离职日期不会改变 8 月应发。这也是为什么不选「不动数据、靠考勤修正」的原因——
改考勤反而会影响 8 月已定的 516。

## 验证

- 工资条接口 `/api/employees/11/payroll-history?year=2026`：只剩 2026-08 一条 ✓
- 薪资报表 `/api/employees/commission-report?month=2026-09`：名单为李梅梅/郑玉清/任志鹏，无董庆庆 ✓
- 其他员工工资条未受影响（李梅梅 11 月项、任志鹏 11 月项、郑玉清 4 月项，数值不变）✓
- 账户余额与流水净额零偏差（`accounts_ok = 0`）✓

## 备份

- 本地：`output/backup-local-employees-pre-offboard-20261029.sql`（仅 employees 表，5.8K）
- 生产：`/mnt/opt/docker/install/db-backups/erp_db-employees-backup-pre-offboard-20261029.sql`（5.5K）

## 同类现象之二：刘玉艳（2026-10-29 用户确认「9 月初就走了」，已处理）

刘玉艳（id=2）工资条同样显示「2026-09 应发 2,000 / 未发 / 差 2,000」，10 月 upcoming 2,000。

| 月份 | 打卡天数 | 区间 |
|---|---|---|
| 2026-08 | 18 天 | 8-01 ~ 8-31（全月在职） |
| 2026-09 | **2 天** | 9-03 ~ 9-04（9-05 起全公司仅李梅梅/郑玉清打卡） |
| 2026-10 | 0 天 | — |

用户确认她 **9 月初就走了**（9-04 为最后工作日，10-03 只是系统删号日），9 月 2,000 属虚假应发。

### 处理（`fix_liuyuyan_offboard_local_20261029.sql` / `sync_liuyuyan_offboard_prod_20261029.sql`）

```sql
-- 1) 补离职日期
UPDATE employees SET last_working_date='2026-09-04', employment_status='departed'
WHERE name='刘玉艳' AND entry_date='2026-05-01' AND last_working_date IS NULL;

-- 2) 9 月离职月底薪按在职日历日折算（人工月度考勤优先于自动推算）
INSERT INTO monthly_attendances (employee_id, month, leave_days, actual_days, late_count, ...)
SELECT id, '2026-09', 30, 0, 0, ... FROM employees WHERE name='刘玉艳' ...
```

折算口径（与董庆庆 8 月同族算法）：

| 项 | 值 |
|---|---|
| 底薪 | 2,000 |
| 日薪 | 2,000 / 30 = 66.6667 |
| 免扣天数 | 4（每月政策） |
| 扣款天数 | 30 − 4 = 26 |
| 扣款 | 26 × 66.6667 = 1,733.33 |
| **应发** | 2,000 − 1,733.33 = 266.67 → 抹零 **266** |
| 等效在岗 | 30 − 26 = **4 天**（9-01 ~ 9-04，与「9 月初离职」一致） |

### 效果

| 月份 | 改前 | 改后 |
|---|---|---|
| 2026-09 | 应发 2,000 / 实付 0 / **差 2,000** | 应发 **266** / 实付 0 / **差 266**（真实欠款） |
| 2026-10 | 应发 2,000 / upcoming | **整月消失** |
| 2026-08 | 应发 1,827（本地）/ 1,949（生产）/ 实付 2,059 | **不变** |

- 生产 9 月发薪名单（`commission-report?month=2026-09`）：李梅梅 2,211 / 郑玉清 2,093 / 任志鹏 2,027，共 3 人，**已无刘玉艳、董庆庆**
- 生产 `operation_histories` #105 留快照
- 生产备份：`/mnt/opt/docker/install/db-backups/erp_db-backup-pre-liuyuyan-20261029.sql.gz`
- 本地备份：`output/backup-local-employees-attendance-pre-liuyuyan-20261029.sql`

### 遗留

266 元为 9 月**真实欠款**，因离职员工不进「薪资结算」在职名单，需在工资条历史月视图查看；
如需补发，手工记一笔 `PAY-2026-09-2`（或走离职结算流程）。
