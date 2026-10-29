// Package migrations 嵌入按文件名排序执行的 SQL 迁移。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
