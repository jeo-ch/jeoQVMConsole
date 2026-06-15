package template

import (
	"fmt"
	"strings"
)

// ValidateTemplateCategory validates the template category against its type.
func ValidateTemplateCategory(templateType, category string) error {
	normalizedType := normalizeTemplateType(templateType)
	category = strings.TrimSpace(category)
	if normalizedType != "linux" && normalizedType != "windows" {
		if category != "" {
			return fmt.Errorf("仅 Linux 和 Windows 模板支持设置二级分类")
		}
		return nil
	}
	if category != "" {
		allowedCategories := linuxTemplateCategories
		if normalizedType == "windows" {
			allowedCategories = windowsTemplateCategories
		}
		for _, allowed := range allowedCategories {
			if strings.EqualFold(category, allowed) {
				return nil
			}
		}
		if normalizedType == "windows" {
			return fmt.Errorf("Windows 模板分类仅支持 WindowsServer2022、Windows10、WindowsServer2012R2 或 其它")
		}
		return fmt.Errorf("Linux 模板分类仅支持 Ubuntu、Debian 或 CentOS")
	}
	return nil
}
