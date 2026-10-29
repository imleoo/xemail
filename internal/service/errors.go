package service

import "errors"

var (
	ErrInvalidCredentials = errors.New("账号或密码错误")
	ErrLocked             = errors.New("尝试次数过多，请稍后再试")
	ErrUnauthorized       = errors.New("未登录或会话已过期")
	ErrInvalidAddress     = errors.New("邮箱地址格式不正确或不属于收信域名")
	ErrInvalidInput       = errors.New("输入不合法")
	ErrConflict           = errors.New("邮箱地址已存在")
	ErrNotFound           = errors.New("账号不存在")
	ErrInvalidDomain      = errors.New("域名格式不正确")
	ErrDomainExists       = errors.New("域名已存在")
	ErrDomainInUse        = errors.New("该域名下还有账号，请先删除账号")
	ErrDomainNotFound     = errors.New("域名不存在")
)
