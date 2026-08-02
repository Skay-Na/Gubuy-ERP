package utils

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// GetPinyin 获取中文的全拼和首字母
// 返回值: 全拼 (如 zhangsan), 首字母 (如 zs)
func GetPinyin(text string) (string, string) {
	if text == "" {
		return "", ""
	}

	args := pinyin.NewArgs()
	// 获取全拼
	args.Style = pinyin.Normal
	pinyinArr := pinyin.Pinyin(text, args)
	var fullPinyin strings.Builder
	var initialPinyin strings.Builder

	for _, p := range pinyinArr {
		if len(p) > 0 {
			fullPinyin.WriteString(p[0])
			initialPinyin.WriteString(string(p[0][0]))
		}
	}

	return fullPinyin.String(), initialPinyin.String()
}
