// audit-2026-08-02-balance-replay.go
// 用途：按 financial_logs.id 顺序重放每个账户的余额，与 accounts.balance 比对
// 运行：go run scripts/audit-2026-08-02-balance-replay.go -db erp_db
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	_ "github.com/go-sql-driver/mysql"
)

type entry struct {
	ID         int64
	FlowType   int
	Amount     float64
	BalanceAft float64
}

func main() {
	dbName := flag.String("db", "erp_audit_test", "数据库名")
	flag.Parse()

	dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") +
		"@tcp(127.0.0.1:3306)/" + *dbName + "?parseTime=true&loc=Local"
	if os.Getenv("DB_USER") == "" {
		dsn = "root@tcp(127.0.0.1:3306)/" + *dbName + "?parseTime=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, name, balance FROM accounts ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	type acc struct {
		ID      int64
		Name    string
		Current float64
	}
	var accounts []acc
	for rows.Next() {
		var a acc
		if err := rows.Scan(&a.ID, &a.Name, &a.Current); err != nil {
			log.Fatal(err)
		}
		accounts = append(accounts, a)
	}
	rows.Close()

	fmt.Printf("%-5s %-15s %12s %12s %12s %12s\n", "ID", "Name", "Current", "Replayed", "Diff", "Logs")
	for _, a := range accounts {
		rs, err := db.Query("SELECT id, flow_type, amount, balance_after FROM financial_logs WHERE account_id = ? ORDER BY id ASC", a.ID)
		if err != nil {
			log.Fatal(err)
		}
		var logs []entry
		for rs.Next() {
			var e entry
			if err := rs.Scan(&e.ID, &e.FlowType, &e.Amount, &e.BalanceAft); err != nil {
				log.Fatal(err)
			}
			logs = append(logs, e)
		}
		rs.Close()
		sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })

		replayed := 0.0
		for _, l := range logs {
			switch l.FlowType {
			case 1, 3: // 收入 / 内部调拨（划入）
				replayed += l.Amount
			case 2: // 支出
				replayed -= l.Amount
			case 4: // 校准：差额方向已经在 amount 的符号里
				// 按当前实现：diff>0 → flow_type=1 / amount=diff；
				//             diff<0 → flow_type=2 / amount=-diff
				// 上面已覆盖
			}
		}
		diff := a.Current - replayed
		fmt.Printf("%-5d %-15s %12.2f %12.2f %12.2f %12d\n",
			a.ID, a.Name, a.Current, replayed, diff, len(logs))
	}
}
