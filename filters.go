package sgin

import "github.com/gin-gonic/gin"

// FilterBackend 将请求参数转换为 Query 条件。
// 具体 SQL/ORM 查询仍由用户自己的 Repository 决定，框架只负责统一解析。
type FilterBackend interface {
	Apply(c *gin.Context, q Query) Query
}

// SearchFilter 解析 ?search=tom。
type SearchFilter struct{}

// Apply 写入 Query.Search。
func (SearchFilter) Apply(c *gin.Context, q Query) Query {
	if value := c.Query("search"); value != "" {
		q.Search = value
	}
	return q
}

// OrderingFilter 解析 ?ordering=-created_at。
type OrderingFilter struct{}

// Apply 写入 Query.OrderBy。
func (OrderingFilter) Apply(c *gin.Context, q Query) Query {
	if value := c.Query("ordering"); value != "" {
		q.OrderBy = value
	}
	return q
}

// FieldFilter 将普通查询参数放入 Query.Filters。
// page、page_size、search、ordering 属于框架保留参数，不会写入 Filters。
type FieldFilter struct{}

// Apply 写入字段过滤条件。
func (FieldFilter) Apply(c *gin.Context, q Query) Query {
	values := c.Request.URL.Query()
	if len(values) == 0 {
		return q
	}
	if q.Filters == nil {
		q.Filters = map[string]string{}
	}
	for key, items := range values {
		if isReservedQueryParam(key) || len(items) == 0 {
			continue
		}
		q.Filters[key] = items[0]
	}
	return q
}

// isReservedQueryParam 判断参数是否由框架内置解析器处理。
func isReservedQueryParam(key string) bool {
	switch key {
	case "page", "page_size", "search", "ordering":
		return true
	default:
		return false
	}
}
