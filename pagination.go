package sgin

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Pagination 定义分页器接口。
// Parse 负责从请求中解析 Query，Wrap 负责把列表数据包装成统一结构。
type Pagination interface {
	Parse(c *gin.Context, cfg Config) Query
	Wrap(c *gin.Context, items any, total int64, q Query) any
}

// PageNumberPagination 实现 page/page_size 分页。
type PageNumberPagination struct{}

// Parse 从 ?page=1&page_size=20 中解析分页参数。
func (PageNumberPagination) Parse(c *gin.Context, cfg Config) Query {
	pageSize := cfg.REST.DefaultPageSize
	maxPageSize := cfg.REST.MaxPageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if maxPageSize <= 0 {
		maxPageSize = 100
	}

	if raw := c.Query("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	page := cfg.REST.DefaultPage
	if page <= 0 {
		page = 1
	}
	if raw := c.Query("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}

	return Query{
		Limit:   pageSize,
		Offset:  (page - 1) * pageSize,
		Filters: map[string]string{},
	}
}

// Wrap 将列表数据包装为统一分页响应。
func (PageNumberPagination) Wrap(c *gin.Context, items any, total int64, q Query) any {
	pageSize := q.Limit
	if pageSize <= 0 {
		pageSize = 1
	}
	page := q.Offset/pageSize + 1
	return H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"results":   items,
	}
}
