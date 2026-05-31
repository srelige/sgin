package sgin

import "context"

// Repository 是 ModelViewSet 依赖的数据访问接口。
// sgin 不绑定 ORM；用户可以用 GORM、SQL、内存 Map 或远程服务实现该接口。
type Repository[T any, ID comparable] interface {
	List(ctx context.Context, q Query) ([]T, error)
	Find(ctx context.Context, id ID) (*T, error)
	Create(ctx context.Context, obj *T) error
	Update(ctx context.Context, obj *T) error
	Delete(ctx context.Context, id ID) error
}

// CountRepository 是可选接口，用于分页开启时返回过滤后、分页前的总数。
type CountRepository[T any, ID comparable] interface {
	Count(ctx context.Context, q Query) (int64, error)
}
