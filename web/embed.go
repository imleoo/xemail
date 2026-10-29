// Package web 嵌入前端构建产物，运行 `make web` 后生成 dist。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func Dist() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
