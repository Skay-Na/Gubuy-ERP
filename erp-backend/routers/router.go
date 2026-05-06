package routers

import (
	"erp-backend/controllers"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupRouter 初始化路由配置
func SetupRouter() *gin.Engine {
	r := gin.Default()

	// 全局跨域中间件
	r.Use(Cors())

	api := r.Group("/api")
	{
		api.GET("/products", controllers.GetProducts)
		api.POST("/products", controllers.CreateProduct)
		api.PUT("/products/:id", controllers.UpdateProduct)
		api.GET("/categories", controllers.GetCategories)
		api.POST("/categories", controllers.CreateCategory)
		api.PUT("/categories/:id", controllers.UpdateCategory)
		api.DELETE("/categories/:id", controllers.DeleteCategory)
		api.PUT("/products/:id/transfer", controllers.TransferProduct)
		api.POST("/products/:id/stocktake", controllers.ProductStocktake)
		api.GET("/products/:id/logs", controllers.GetInventoryLogs)
		api.POST("/orders", controllers.CreateOrder)
		api.GET("/orders", controllers.GetOrders)
		api.PUT("/orders/:id/subsidy", controllers.UpdateSubsidyStatus)
		api.PUT("/orders/:id/balance", controllers.PayOrderBalance)
		api.PUT("/orders/:id/cancel", controllers.CancelOrder)
		api.PUT("/orders/:id/install", controllers.UpdateInstallStatus)
		api.POST("/inbounds", controllers.CreateInbound)
		api.GET("/inventory/logs", controllers.GetGlobalInventoryLogs)
		api.GET("/expenses", controllers.GetExpenses)
		api.POST("/expenses", controllers.CreateExpense)
		api.GET("/finance/logs", controllers.GetFinancialLogs)
		api.GET("/finance/summary", controllers.GetFinanceSummary)
		api.GET("/accounts", controllers.GetAccounts)
		api.POST("/accounts", controllers.CreateAccount)
		api.POST("/accounts/transfer", controllers.TransferFunds)
		api.GET("/accounts/transfers", controllers.GetTransferRecords)
		api.PUT("/accounts/:id/calibrate", controllers.CalibrateAccount)
		api.GET("/employees", controllers.GetEmployees)
		api.POST("/employees", controllers.CreateEmployee)
		api.PUT("/employees/:id", controllers.UpdateEmployee)
		api.DELETE("/employees/:id", controllers.DeleteEmployee)
		api.POST("/employees/attendance", controllers.UpdateAttendance)
		api.GET("/employees/commission-report", controllers.GetCommissionReport)
		api.POST("/employees/verify-pin", controllers.VerifyPin)
		api.GET("/employees/:id/stats", controllers.GetEmployeeMeStats)
		api.POST("/employees/check-in", controllers.CheckIn)
		api.POST("/employees/check-out", controllers.CheckOut)
		api.GET("/employees/attendance-logs", controllers.GetDailyAttendance)
		api.PUT("/admin/password", controllers.UpdateAdminPassword)
		api.POST("/admin/verify", controllers.VerifyAdminPassword)
		api.GET("/admin/status", controllers.CheckAdminStatus)
		api.POST("/admin/init", controllers.InitAdmin)
		api.POST("/system/reset", controllers.ResetSystem)
	}

	return r
}

// Cors 跨域中间件
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", "*") // 或者指定特定的 Origin
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
			c.Header("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token, Authorization, Token")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		}

		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
		}
		c.Next()
	}
}
