package routers

import (
	"erp-backend/controllers"
	"erp-backend/middlewares"
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRouter 初始化路由配置
func SetupRouter() *gin.Engine {
	r := gin.Default()

	// 全局跨域中间件
	r.Use(Cors())

	// 静态文件服务 (Vue/Vite 编译后的 dist 目录)
	r.Static("/assets", "./dist/assets")
	r.StaticFile("/favicon.ico", "./dist/favicon.ico")

	// Swagger UI 路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
			auth.POST("/dev/reset-order-counters", controllers.ResetOrderCounters)
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
			auth.POST("/ai/agent/sessions", middlewares.AdminAuthRequired(), controllers.CreateAIAgentSession)
			auth.GET("/ai/agent/sessions/:id", middlewares.AdminAuthRequired(), controllers.GetAIAgentSession)
			auth.POST("/ai/agent/sessions/:id/messages", middlewares.AdminAuthRequired(), controllers.SendAIAgentMessage)
			auth.POST("/ai/agent/sessions/:id/planner-tasks", middlewares.AdminAuthRequired(), controllers.CreateAIAgentPlannerTask)
			auth.GET("/ai/agent/memories", middlewares.AdminAuthRequired(), controllers.ListAIAgentMemories)
			auth.POST("/ai/agent/memories", middlewares.AdminAuthRequired(), controllers.CreateAIAgentMemory)
			auth.PUT("/ai/agent/memories/:memory_id", middlewares.AdminAuthRequired(), controllers.UpdateAIAgentMemory)
			auth.DELETE("/ai/agent/memories/:memory_id", middlewares.AdminAuthRequired(), controllers.DeleteAIAgentMemory)
			auth.POST("/ai/agent/actions/:id/confirm", middlewares.AdminAuthRequired(), middlewares.OperationHistoryMiddleware(), controllers.ConfirmAIAgentAction)
			auth.GET("/ai/agent/messages/:message_id/trace", middlewares.AdminAuthRequired(), controllers.GetAIAgentMessageTrace)
			auth.POST("/ai/agent/messages/:message_id/feedback", middlewares.AdminAuthRequired(), controllers.SubmitAIAgentFeedback)
			auth.GET("/ai/agent/cases", middlewares.AdminAuthRequired(), controllers.ListAIAgentCases)
			auth.PUT("/ai/agent/cases/:case_id", middlewares.AdminAuthRequired(), controllers.UpdateAIAgentCase)
			auth.POST("/ai/agent/predictions/evaluate", middlewares.AdminAuthRequired(), controllers.EvaluateAIAgentPredictions)
			auth.GET("/ai/agent/predictions", middlewares.AdminAuthRequired(), controllers.ListAIAgentPredictions)
			auth.GET("/ai/agent/knowledge", middlewares.AdminAuthRequired(), controllers.ListAIAgentKnowledge)
			auth.POST("/ai/agent/knowledge", middlewares.AdminAuthRequired(), controllers.CreateAIAgentKnowledge)
			auth.PUT("/ai/agent/knowledge/:knowledge_id", middlewares.AdminAuthRequired(), controllers.UpdateAIAgentKnowledge)
			auth.GET("/ai/agent/brief", middlewares.AdminAuthRequired(), controllers.GetAIAgentBrief)
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
			auth.POST("/expenses", controllers.CreateExpense)
			auth.PUT("/expenses/:id", controllers.UpdateExpense)
			auth.GET("/search/related", controllers.SearchRelated)
			auth.GET("/finance/logs", controllers.GetFinancialLogs)
			auth.GET("/finance/summary", controllers.GetFinanceSummary)
			auth.GET("/bookkeeping/orders", controllers.SearchBookkeepingOrders)
			auth.POST("/bookkeeping/entries", controllers.CreateBookkeepingEntry)
			auth.GET("/stats/monthly", controllers.GetMonthlyStats)
			auth.GET("/accounts", controllers.GetAccounts)
			auth.POST("/accounts", controllers.CreateAccount)
			auth.POST("/accounts/transfer", controllers.TransferFunds)
			auth.GET("/accounts/transfers", controllers.GetTransferRecords)
			auth.PUT("/accounts/:id/calibrate", controllers.CalibrateAccount)
			auth.PUT("/accounts/:id/set-default-expense", controllers.SetDefaultExpenseAccount)
			auth.GET("/employees", controllers.GetEmployees)
			auth.POST("/employees", controllers.CreateEmployee)
			auth.PUT("/employees/:id", controllers.UpdateEmployee)
			auth.DELETE("/employees/:id", controllers.DeleteEmployee)
			auth.POST("/employees/attendance", controllers.UpdateAttendance)
			auth.GET("/employees/commission-report", controllers.GetCommissionReport)
			auth.GET("/employees/:id/stats", controllers.GetEmployeeMeStats)
			auth.POST("/employees/check-in", controllers.CheckIn)
			auth.POST("/employees/check-out", controllers.CheckOut)
			auth.GET("/employees/attendance-logs", controllers.GetDailyAttendance)
			auth.PUT("/admin/password", controllers.UpdateAdminPassword)
			auth.GET("/subsidy/summary", controllers.GetSubsidySummary)
			auth.GET("/subsidy/settlements", controllers.GetSubsidySettlements)
			auth.POST("/subsidy/settle", controllers.CreateSubsidySettlement)

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
			adminAuth.PUT("/orders/:id/referral-fee", controllers.UpdateOrderReferralFee)
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
			adminAuth.GET("/employee-offboarding-alerts", controllers.ListEmployeeOffboardingAlerts)
			adminAuth.PUT("/employee-offboardings/:id/tasks/:task_id", controllers.UpdateEmployeeHandoverTask)
			adminAuth.POST("/employee-offboardings/:id/settlement", controllers.ConfirmEmployeeOffboardingSettlement)
			adminAuth.POST("/employee-offboarding-alerts/:id/resolve", controllers.ResolveEmployeeOffboardingAlert)
		}
	}

	// 所有非 API 请求都返回 index.html (支持 SPA 路由)
	r.NoRoute(func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.File("./dist/index.html")
	})

	return r
}

// Cors 跨域中间件
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			// 动态将请求 Origin 写入 Allow-Origin，从而支持 Credentials: true，并避免与通配符 * 冲突
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
			c.Header("Access-Control-Allow-Headers", "Content-Type, AccessToken, X-CSRF-Token, Authorization, Token")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Disposition, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		}

		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
