package sgin

// Query 是 Repository 层接收的统一查询描述。
// 框架只负责解析 HTTP 查询参数，具体如何映射到 SQL、ORM 或其他存储由 Repository 决定。
type Query struct {
	Limit   int
	Offset  int
	Search  string
	OrderBy string
	Filters map[string]string
}
