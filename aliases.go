package sgin

import "github.com/gin-gonic/gin"

// H 是 gin.H 的别名，用于降低从 Gin 迁移到 sgin 的心智成本。
// 用户可以继续写 sgin.H{"message": "ok"} 来构造 JSON 对象。
type H = gin.H

// Context 是 gin.Context 的别名。
// sgin 不替换 Gin 的上下文对象，因此 Gin 原有的绑定、响应、中间件能力都能直接使用。
type Context = gin.Context
