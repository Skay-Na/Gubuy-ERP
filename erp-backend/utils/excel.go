package utils

import (
	"fmt"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// ExportToExcel 将二维数组数据导出为 Excel 文件并直接响应给前端
func ExportToExcel(c *gin.Context, filename string, headers []string, data [][]interface{}) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Println("关闭 Excel 文件失败:", err)
		}
	}()

	sheetName := "Sheet1"
	// 设置工作表名称
	f.SetSheetName(f.GetSheetName(0), sheetName)

	// 写入表头
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// 写入数据
	for i, row := range data {
		for j, val := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(sheetName, cell, val)
		}
	}

	// 生成带时间戳的文件名
	finalFilename := fmt.Sprintf("%s_%s.xlsx", filename, time.Now().Format("20060102150405"))
	encodedFilename := url.QueryEscape(finalFilename)

	// 设置响应头
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=utf-8''%s", encodedFilename, encodedFilename))
	c.Header("Content-Transfer-Encoding", "binary")
	// 暴露 Content-Disposition 给前端以便读取文件名
	c.Header("Access-Control-Expose-Headers", "Content-Disposition")

	// 写入响应流
	if err := f.Write(c.Writer); err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "导出 Excel 文件失败: " + err.Error()})
		return
	}
}
