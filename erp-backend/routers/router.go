package routers

import (
	"erp-backend/controllers"
	"erp-backend/middlewares"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// corsAllowOrigins 读取 CORS 白名单（逗号分隔）。
// 默认包含本地开发前端与生产地址，未配置时保持可用的最小集。
func corsAllowOrigins() map[string]struct{} {
	raw := os.Getenv("CORS_ALLOW_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		raw = "http://localhost:5173,http://192.168.5.22:8080"
	}
	allowed := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return allowed
}

// SetupRouter 初始化路由配置
func SetupRouter() *gin.Engine {
	r := gin.Default()

	// 全局跨域中间件
	r.Use(Cors())

	// 静态文件服务 (Vue/Vite 编译后的 dist 目录)
	r.Static("/assets", "./dist/assets")
	r.StaticFile("/favicon.ico", "./dist/favicon.ico")

	// Swagger UI 路由（生产文档不裸奔：挂管理员鉴权）
	r.GET("/swagger/*any", middlewares.AdminAuthRequired(), ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api")
	{
		// 无需 JWT 认证的接口：管理员验证、系统状态检查、系统初始化
		api.POST("/admin/verify", controllers.VerifyAdminPassword)
		api.GET("/admin/status", controllers.CheckAdminStatus)
		api.POST("/admin/init", controllers.InitAdmin)
		api.GET("/employees/simple", controllers.GetEmployeesSimple)
		api.POST("/employees/verify-pin", controllers.VerifyPin)

		// 需要 JWT 认证的接口
		auth := api.Group("")
		auth.Use(middlewares.AuthRequired())
		auth.Use(middlewares.OperationHistoryMiddleware())
		{
			auth.GET("/products", controllers.GetProducts)
			auth.POST("/products", controllers.CreateProduct)
			auth.GET("/categories", controllers.GetCategories)
			auth.POST("/categories", controllers.CreateCategory)
			auth.PUT("/categories/:id", controllers.UpdateCategory)
			auth.DELETE("/categories/:id", controllers.DeleteCategory)
			auth.POST("/products/:id/stocktake", controllers.ProductStocktake)
			auth.POST("/stocktake-batches", controllers.CreateStocktakeBatch)
			auth.GET("/stocktake-batches", controllers.ListMyStocktakeBatches)
			auth.GET("/stocktake-batches/:id", controllers.GetStocktakeBatch)
			auth.GET("/admin/stocktake-batches", middlewares.AdminAuthRequired(), controllers.ListAdminStocktakeBatches)
			auth.POST("/admin/stocktake-batches/:id/terminate", middlewares.AdminAuthRequired(), controllers.TerminateStocktakeBatch)
			auth.DELETE("/admin/stocktake-batches/:id", middlewares.AdminAuthRequired(), controllers.DeleteStocktakeBatch)
			auth.POST("/stocktake-batches/:id/print", controllers.MarkStocktakeBatchPrinted)
			auth.PUT("/stocktake-batches/:id/items", controllers.UpdateStocktakeBatchItems)
			auth.POST("/stocktake-batches/:id/submit", controllers.SubmitStocktakeBatch)
			auth.POST("/stocktake-batches/:id/items/:item_id/refresh", controllers.RefreshStocktakeBatchItem)
			auth.GET("/products/:id/logs", controllers.GetInventoryLogs)
			auth.POST("/orders", controllers.CreateOrder)
			auth.POST("/ai/orders", controllers.CreateAIOrder)
			auth.POST("/ai/imports", controllers.AnalyzeAIImport)
			auth.POST("/ai/chat", middlewares.AdminAuthRequired(), controllers.ChatAI)
			// 新版 Agent（pi 核心 agent-service）的数据工具接口
			auth.GET("/ai/agent-tools/product-inventory", middlewares.AdminAuthRequired(), controllers.AgentToolProductInventory)
			auth.GET("/ai/agent-tools/sales-summary", middlewares.AdminAuthRequired(), controllers.AgentToolSalesSummary)
			auth.GET("/ai/agent-tools/sales-trend", middlewares.AdminAuthRequired(), controllers.AgentToolSalesTrend)
			auth.GET("/ai/agent-tools/inventory-sales", middlewares.AdminAuthRequired(), controllers.AgentToolInventorySales)
			auth.GET("/ai/agent-tools/unpaid-orders", middlewares.AdminAuthRequired(), controllers.AgentToolUnpaidOrders)
			auth.GET("/system/ai/providers", middlewares.AdminAuthRequired(), controllers.GetAIProviderSettings)
			auth.PUT("/system/ai/providers/:provider", middlewares.AdminAuthRequired(), controllers.SaveAIProviderSettings)
			auth.DELETE("/system/ai/providers/:provider", middlewares.AdminAuthRequired(), controllers.DeleteAIProviderKey)
			auth.PUT("/system/ai/active", middlewares.AdminAuthRequired(), controllers.ActivateAIProvider)
			auth.GET("/ai/imports", controllers.ListAIImports)
			auth.GET("/ai/imports/:id", controllers.GetAIImport)
			auth.GET("/ai/imports/:id/image", controllers.GetAIImportImage)
			auth.DELETE("/ai/imports/:id", controllers.DeleteAIImport)
			auth.GET("/orders", controllers.GetOrders)
			auth.PUT("/orders/:id/subsidy", controllers.UpdateSubsidyStatus)
			auth.PUT("/orders/:id/balance", controllers.PayOrderBalance)
			auth.PUT("/orders/:id/cancel", controllers.CancelOrder)
			auth.PUT("/orders/:id/reversal-cancel", controllers.ReversalCancelOrder)
			auth.PUT("/orders/:id/install", controllers.UpdateInstallStatus)
			auth.PUT("/orders/:id/deliver", controllers.UpdateDeliveryStatus)
			auth.PUT("/order-items/:item_id/deliver", controllers.UpdateItemDeliveryStatus)
			auth.PUT("/order-items/:item_id/install", controllers.UpdateItemInstallStatus)
			auth.PUT("/orders/:id/commission", controllers.UpdateOrderCommission)
			auth.GET("/pending-inbounds/mine", controllers.ListMyPendingInbounds)
			auth.GET("/pending-stocktakes/mine", controllers.ListMyPendingStocktakes)
			auth.GET("/inventory/logs", controllers.GetGlobalInventoryLogs)
			auth.GET("/expenses", controllers.GetExpenses)
			auth.POST("/expenses", middlewares.AdminAuthRequired(), controllers.CreateExpense)
			auth.PUT("/expenses/:id", middlewares.AdminAuthRequired(), controllers.UpdateExpense)
			auth.GET("/search/related", controllers.SearchRelated)
			auth.GET("/finance/logs", controllers.GetFinancialLogs)
			auth.GET("/finance/summary", controllers.GetFinanceSummary)
			// 看板利润计算过程透明化：公式汇总与逐项明细钻取（均为只读查询）
			auth.GET("/finance/profit-breakdown", controllers.GetProfitBreakdown)
			auth.GET("/finance/breakdown/:type", controllers.GetBreakdownDetail)
			auth.GET("/bookkeeping/orders", controllers.SearchBookkeepingOrders)
			auth.POST("/bookkeeping/entries", middlewares.AdminAuthRequired(), controllers.CreateBookkeepingEntry)
			auth.POST("/payroll/confirm", middlewares.AdminAuthRequired(), controllers.ConfirmPayroll)
			auth.GET("/payroll/confirmations", controllers.GetPayrollConfirmations)
			auth.GET("/stats/monthly", controllers.GetMonthlyStats)
			auth.GET("/finance/real-profit", controllers.GetRealProfit)
			auth.GET("/finance/workspace", controllers.GetFinanceWorkspace)
			auth.GET("/finance/sales-analysis", controllers.GetFinanceSalesAnalysis)
			auth.GET("/accounts", controllers.GetAccounts)
			auth.POST("/accounts", middlewares.AdminAuthRequired(), controllers.CreateAccount)
			auth.POST("/accounts/transfer", middlewares.AdminAuthRequired(), controllers.TransferFunds)
			auth.GET("/accounts/transfers", controllers.GetTransferRecords)
			auth.PUT("/accounts/:id/calibrate", middlewares.AdminAuthRequired(), controllers.CalibrateAccount)
			auth.PUT("/accounts/:id/set-default-expense", controllers.SetDefaultExpenseAccount)
			auth.GET("/employees", controllers.GetEmployees)
			auth.GET("/employees/commission-report", controllers.GetCommissionReport)
			auth.GET("/employees/:id/stats", controllers.GetEmployeeMeStats)
			// 员工自助打卡与考勤查询保持员工可用；管理性质的档案与考勤修改收归管理员。
			auth.POST("/employees/check-in", controllers.CheckIn)
			auth.POST("/employees/check-out", controllers.CheckOut)
			auth.GET("/employees/attendance-logs", controllers.GetDailyAttendance)
			auth.PUT("/admin/password", middlewares.AdminAuthRequired(), controllers.UpdateAdminPassword)
			auth.GET("/subsidy/summary", controllers.GetSubsidySummary)
			auth.GET("/subsidy/settlements", controllers.GetSubsidySettlements)
			auth.POST("/subsidy/settle", middlewares.AdminAuthRequired(), controllers.CreateSubsidySettlement)
			auth.POST("/subsidy/allocate", middlewares.AdminAuthRequired(), controllers.AllocateSubsidyReceipt)
			auth.GET("/subsidy/receipts", controllers.GetSubsidyReceipts)

			// 导出相关接口
			auth.GET("/orders/export", controllers.ExportOrders)
			auth.GET("/products/export", controllers.ExportProducts)
			auth.GET("/employees/export", controllers.ExportEmployees)
			auth.GET("/finance/logs/export", controllers.ExportFinancialLogs)
		}

		// 需要 Admin 权限的接口
		adminAuth := api.Group("")
		adminAuth.Use(middlewares.AdminAuthRequired())
		adminAuth.Use(middlewares.OperationHistoryMiddleware())
		{
			adminAuth.Any("/ai/manager/*path", func(c *gin.Context) {
				c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "msg": "该 Agent 工作台功能已停用"})
			})
			adminAuth.GET("/system/backup", controllers.ExportBackup)
			adminAuth.POST("/system/restore", controllers.ImportBackup)
			adminAuth.GET("/operations", controllers.GetOperationHistory)
			adminAuth.POST("/operations/:id/undo", controllers.UndoOperation)
			adminAuth.PUT("/orders/:id/referrer", controllers.UpdateOrderReferrer)
			adminAuth.PUT("/order-items/:item_id/delivery-fee", controllers.UpdateItemDeliveryFee)
			adminAuth.PUT("/order-items/:item_id/installation-fee", controllers.UpdateItemInstallationFee)
			adminAuth.PUT("/products/:id/transfer", controllers.TransferProduct)
			adminAuth.PUT("/products/:id", controllers.UpdateProduct)
			adminAuth.POST("/products/recalculate-costs", controllers.RecalculateAllAvgCosts)
			adminAuth.POST("/inbounds", controllers.CreateInbound)
			adminAuth.POST("/inbounds/cancel-by-log/:log_id", controllers.CancelInboundByLog)
			adminAuth.POST("/inbounds/update-price/:log_id", controllers.UpdateInventoryLogPrice)
			adminAuth.GET("/inventory/logs/export", controllers.ExportInventoryLogs)
			adminAuth.GET("/pending-inbounds", controllers.ListPendingInbounds)
			adminAuth.POST("/pending-inbounds/:id/approve", controllers.ApprovePendingInbound)
			adminAuth.POST("/pending-inbounds/:id/reject", controllers.RejectPendingInbound)
			adminAuth.GET("/pending-stocktakes", controllers.ListPendingStocktakes)
			adminAuth.POST("/pending-stocktakes/:id/approve", controllers.ApprovePendingStocktake)
			adminAuth.POST("/pending-stocktakes/:id/reject", controllers.RejectPendingStocktake)
			adminAuth.GET("/system/historical-cost-repair/candidates", controllers.ListHistoricalCostRepairCandidates)
			adminAuth.POST("/system/historical-cost-repair", controllers.RepairHistoricalCost)
			adminAuth.POST("/employees/:id/offboardings", controllers.CreateEmployeeOffboarding)
			adminAuth.GET("/employees/:id/offboardings", controllers.GetEmployeeOffboarding)
			// 工资条：按年逐月应发/实付/差额与提成订单明细，含确认发放定稿快照。
			adminAuth.GET("/employees/:id/payroll-history", controllers.GetPayrollHistory)
			// 员工档案写操作与考勤修正属于管理行为，统一收归管理员权限。
			adminAuth.POST("/employees", controllers.CreateEmployee)
			adminAuth.PUT("/employees/:id", controllers.UpdateEmployee)
			adminAuth.DELETE("/employees/:id", controllers.DeleteEmployee)
			adminAuth.POST("/employees/attendance", controllers.UpdateAttendance)
			adminAuth.GET("/employee-offboarding-alerts", controllers.ListEmployeeOffboardingAlerts)
			adminAuth.PUT("/employee-offboardings/:id/tasks/:task_id", controllers.UpdateEmployeeHandoverTask)
			adminAuth.POST("/employee-offboardings/:id/settlement", controllers.ConfirmEmployeeOffboardingSettlement)
			adminAuth.POST("/employee-offboarding-alerts/:id/resolve", controllers.ResolveEmployeeOffboardingAlert)

			// 工程模块（engineering）：独立业务线，仅管理端可见，与国补/零售订单零耦合。
			// 该分组已挂 AdminAuthRequired + OperationHistoryMiddleware，员工 token 一律 403。
			// 接口契约：docs/tech/2026-10-29-engineering-module-api-contract.md（16 个接口）
			adminAuth.GET("/admin/engineering/summary", controllers.GetEngineeringSummary)
			adminAuth.GET("/admin/engineering/projects", controllers.ListEngineeringProjects)
			adminAuth.POST("/admin/engineering/projects", controllers.CreateEngineeringProject)
			adminAuth.GET("/admin/engineering/projects/:id", controllers.GetEngineeringProject)
			adminAuth.PUT("/admin/engineering/projects/:id", controllers.UpdateEngineeringProject)
			adminAuth.DELETE("/admin/engineering/projects/:id", controllers.DeleteEngineeringProject)
			adminAuth.PUT("/admin/engineering/projects/:id/status", controllers.UpdateEngineeringProjectStatus)
			adminAuth.PUT("/admin/engineering/projects/:id/items", controllers.SaveEngineeringItems)
			adminAuth.PUT("/admin/engineering/projects/:id/payment-terms", controllers.SaveEngineeringTerms)
			adminAuth.POST("/admin/engineering/projects/:id/receipts", controllers.CreateEngineeringReceipt)
			adminAuth.POST("/admin/engineering/projects/:id/receipts/:rid/reverse", controllers.ReverseEngineeringReceipt)
			adminAuth.POST("/admin/engineering/projects/:id/shipments", controllers.CreateEngineeringShipment)
			// 费用接口路径不带 /projects/:id 前缀：公共费用没有项目，避免同一弹窗两套提交逻辑。
			// 注意与经营开销接口 /api/admin/expenses 是两套账，不可混用。
			adminAuth.GET("/admin/engineering/expenses", controllers.ListEngineeringExpenses)
			adminAuth.POST("/admin/engineering/expenses", controllers.CreateEngineeringExpense)
			adminAuth.PUT("/admin/engineering/expenses/:eid", controllers.UpdateEngineeringExpense)
			adminAuth.DELETE("/admin/engineering/expenses/:eid", controllers.DeleteEngineeringExpense)
		}
	}

	// 所有非 API 请求都返回 index.html (支持 SPA 路由)
	// 例外：pi 核心 Agent 服务（agent-service，同容器 127.0.0.1:8082）。
	// 开发态由 vite 代理 /api/ai/agent(?!-tools) → 8082；生产容器没有 vite，
	// 8082 也不映射宿主端口，因此必须在 Go 侧反向代理承接，否则前端 AI 店长
	// 拿到的是 SPA 兜底页。鉴权双重保险：Go 侧要求管理员 JWT，agent-service
	// 侧也会再验一次。
	agentServiceURL := os.Getenv("ERP_AGENT_SERVICE_URL")
	if strings.TrimSpace(agentServiceURL) == "" {
		agentServiceURL = "http://127.0.0.1:8082"
	}
	agentTarget, err := url.Parse(agentServiceURL)
	if err == nil {
		agentProxy := httputil.NewSingleHostReverseProxy(agentTarget)
		// SSE 流式响应必须立即刷新，否则消息逐段输出会被缓冲。
		agentProxy.FlushInterval = -1

		isAgentServicePath := func(p string) bool {
			if p == "/api/ai/agent" || strings.HasPrefix(p, "/api/ai/agent/") {
				// /api/ai/agent-tools 仍是 Go 后端的数据工具接口，不转发。
				return !strings.HasPrefix(p, "/api/ai/agent-tools")
			}
			return false
		}

		r.NoRoute(func(c *gin.Context) {
			if isAgentServicePath(c.Request.URL.Path) {
				middlewares.AuthRequired()(c)
				if c.IsAborted() {
					return
				}
				middlewares.AdminAuthRequired()(c)
				if c.IsAborted() {
					return
				}
				agentProxy.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.File("./dist/index.html")
		})
	} else {
		r.NoRoute(func(c *gin.Context) {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.File("./dist/index.html")
		})
	}

	return r
}

// RegisterDevRoutes 注册开发/测试专用运维接口（如重置订单计数器）。
// 独立于 SetupRouter：由 main.go 在 APP_ENV 为 development/test 时调用，
// 生产环境不暴露该路由；同时保证 SetupRouter 在所有环境下注册的路由集合
// 恒定，避免 swagger 契约随环境漂移（契约测试仅覆盖 SetupRouter）。
func RegisterDevRoutes(r *gin.Engine) {
	auth := r.Group("/api")
	// RegisterDevRoutes 在 SetupRouter 之后调用，gin 的引擎级中间件
	// 只对注册时已存在的 handler 链生效，因此这里需显式补挂 CORS。
	auth.Use(Cors())
	auth.Use(middlewares.AuthRequired())
	auth.Use(middlewares.OperationHistoryMiddleware())
	{
		auth.POST("/dev/reset-order-counters", controllers.ResetOrderCounters)
	}
}

// Cors 跨域中间件：仅对白名单内的 Origin 反射 CORS 头。
// 白名单通过 CORS_ALLOW_ORIGINS 环境变量配置（逗号分隔）；
// 无 Origin（同源/服务端请求）直接放行，不添加 CORS 头；
// 白名单外的跨域请求不添加 CORS 头（浏览器将拦截跨域响应），预检请求直接拒绝。
func Cors() gin.HandlerFunc {
	allowed := corsAllowOrigins()
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		originAllowed := false
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				originAllowed = true
				// 反射白名单 Origin，从而支持 Credentials: true，并避免与通配符 * 冲突
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
				c.Header("Access-Control-Allow-Headers", "Content-Type, AccessToken, X-CSRF-Token, Authorization, Token")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Disposition, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
			}
		}

		if method == "OPTIONS" {
			if originAllowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}
		c.Next()
	}
}
