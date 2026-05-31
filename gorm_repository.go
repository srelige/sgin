package sgin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GORMRepository 是默认的 GORM 数据访问实现。
// ModelViewSet 未显式设置 Repository 时，会按 App 的 database 配置创建该仓库。
type GORMRepository[T any, ID comparable] struct {
	DB *gorm.DB

	LookupField    string
	SearchFields   []string
	OrderingFields []string
	FilterFields   []string
}

// NewGORMRepository 创建一个可直接传给 ModelViewSet 的 GORM Repository。
func NewGORMRepository[T any, ID comparable](db *gorm.DB) *GORMRepository[T, ID] {
	return &GORMRepository[T, ID]{
		DB: db,
	}
}

// AutoMigrate 对当前模型执行 GORM 自动迁移。
func (r *GORMRepository[T, ID]) AutoMigrate() error {
	db, err := r.rawDB(context.Background())
	if err != nil {
		return err
	}
	return db.AutoMigrate(new(T))
}

// List 查询模型列表，并应用 Query 中的过滤、搜索、排序和分页。
func (r *GORMRepository[T, ID]) List(ctx context.Context, q Query) ([]T, error) {
	db, err := r.queryDB(ctx, q, true)
	if err != nil {
		return nil, err
	}
	if q.Offset > 0 {
		db = db.Offset(q.Offset)
	}
	if q.Limit > 0 {
		db = db.Limit(q.Limit)
	}

	var items []T
	if err := db.Find(&items).Error; err != nil {
		return nil, mapGormError(err)
	}
	return items, nil
}

// Count 查询过滤后、分页前的总数。
func (r *GORMRepository[T, ID]) Count(ctx context.Context, q Query) (int64, error) {
	db, err := r.queryDB(ctx, q, false)
	if err != nil {
		return 0, err
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, mapGormError(err)
	}
	return total, nil
}

// Find 按 lookup 字段读取单个模型，默认 lookup 字段是 id。
func (r *GORMRepository[T, ID]) Find(ctx context.Context, id ID) (*T, error) {
	db, err := r.baseDB(ctx)
	if err != nil {
		return nil, err
	}

	var obj T
	if err := db.Where(r.lookupEq(id)).First(&obj).Error; err != nil {
		return nil, mapGormError(err)
	}
	return &obj, nil
}

// Create 创建模型。
func (r *GORMRepository[T, ID]) Create(ctx context.Context, obj *T) error {
	db, err := r.rawDB(ctx)
	if err != nil {
		return err
	}
	return mapGormError(db.Create(obj).Error)
}

// Update 保存模型。
func (r *GORMRepository[T, ID]) Update(ctx context.Context, obj *T) error {
	db, err := r.rawDB(ctx)
	if err != nil {
		return err
	}
	return mapGormError(db.Save(obj).Error)
}

// Delete 按 lookup 字段删除模型，默认 lookup 字段是 id。
func (r *GORMRepository[T, ID]) Delete(ctx context.Context, id ID) error {
	db, err := r.baseDB(ctx)
	if err != nil {
		return err
	}
	tx := db.Where(r.lookupEq(id)).Delete(new(T))
	if tx.Error != nil {
		return mapGormError(tx.Error)
	}
	if tx.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GORMRepository[T, ID]) rawDB(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("sgin: gorm db is not set")
	}
	return r.DB.WithContext(ctx), nil
}

func (r *GORMRepository[T, ID]) baseDB(ctx context.Context) (*gorm.DB, error) {
	db, err := r.rawDB(ctx)
	if err != nil {
		return nil, err
	}
	return db.Model(new(T)), nil
}

func (r *GORMRepository[T, ID]) queryDB(ctx context.Context, q Query, includeOrdering bool) (*gorm.DB, error) {
	db, err := r.baseDB(ctx)
	if err != nil {
		return nil, err
	}

	for key, value := range q.Filters {
		filter, err := parseFilterExpr(key, value)
		if err != nil {
			return nil, err
		}
		if !fieldAllowed(filter.Field, r.FilterFields) {
			return nil, fmt.Errorf("%w: invalid filter field %q", ErrBadRequest, key)
		}
		db, err = applyFilterExpr(db, filter)
		if err != nil {
			return nil, err
		}
	}

	if q.Search != "" && len(r.SearchFields) > 0 {
		searchDB := db.Session(&gorm.Session{NewDB: true})
		applied := false
		for _, field := range r.SearchFields {
			if !fieldAllowed(field, r.SearchFields) {
				return nil, fmt.Errorf("%w: invalid search field %q", ErrBadRequest, field)
			}
			expr := clause.Like{Column: clause.Column{Name: field}, Value: "%" + q.Search + "%"}
			if !applied {
				searchDB = searchDB.Where(expr)
				applied = true
				continue
			}
			searchDB = searchDB.Or(expr)
		}
		if applied {
			db = db.Where(searchDB)
		}
	}

	if includeOrdering && q.OrderBy != "" {
		ordering, err := parseOrderBy(q.OrderBy)
		if err != nil {
			return nil, err
		}
		for _, item := range ordering {
			if !fieldAllowed(item.Field, r.OrderingFields) {
				return nil, fmt.Errorf("%w: invalid ordering field %q", ErrBadRequest, item.Field)
			}
			db = db.Order(clause.OrderByColumn{Column: clause.Column{Name: item.Field}, Desc: item.Desc})
		}
	}

	return db, nil
}

func (r *GORMRepository[T, ID]) lookupEq(id ID) clause.Eq {
	return clause.Eq{
		Column: clause.Column{Name: r.lookupField()},
		Value:  id,
	}
}

func (r *GORMRepository[T, ID]) lookupField() string {
	if r != nil && strings.TrimSpace(r.LookupField) != "" {
		return strings.TrimSpace(r.LookupField)
	}
	return "id"
}

type filterExpr struct {
	Field string
	Op    string
	Value string
}

type orderExpr struct {
	Field string
	Desc  bool
}

func parseFilterExpr(key string, value string) (filterExpr, error) {
	key = strings.TrimSpace(key)
	field := key
	op := "eq"
	if strings.Contains(key, "__") {
		parts := strings.SplitN(key, "__", 2)
		field = strings.TrimSpace(parts[0])
		op = strings.TrimSpace(parts[1])
	}
	switch op {
	case "eq", "exact", "gt", "gte", "lt", "lte", "contains", "in":
	default:
		return filterExpr{}, fmt.Errorf("%w: unsupported filter operator %q", ErrBadRequest, op)
	}
	return filterExpr{Field: field, Op: op, Value: value}, nil
}

func applyFilterExpr(db *gorm.DB, filter filterExpr) (*gorm.DB, error) {
	column := clause.Column{Name: filter.Field}
	switch filter.Op {
	case "eq", "exact":
		return db.Where(clause.Eq{Column: column, Value: filter.Value}), nil
	case "gt":
		return db.Where(clause.Gt{Column: column, Value: filter.Value}), nil
	case "gte":
		return db.Where(clause.Gte{Column: column, Value: filter.Value}), nil
	case "lt":
		return db.Where(clause.Lt{Column: column, Value: filter.Value}), nil
	case "lte":
		return db.Where(clause.Lte{Column: column, Value: filter.Value}), nil
	case "contains":
		return db.Where(clause.Like{Column: column, Value: "%" + filter.Value + "%"}), nil
	case "in":
		values := splitCSV(filter.Value)
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: filter %q requires at least one value", ErrBadRequest, filter.Field+"__in")
		}
		return db.Where(clause.IN{Column: column, Values: values}), nil
	default:
		return nil, fmt.Errorf("%w: unsupported filter operator %q", ErrBadRequest, filter.Op)
	}
}

func splitCSV(raw string) []any {
	parts := strings.Split(raw, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseOrderBy(raw string) ([]orderExpr, error) {
	parts := strings.Split(raw, ",")
	ordering := make([]orderExpr, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			return nil, fmt.Errorf("%w: invalid ordering field %q", ErrBadRequest, part)
		}
		desc := strings.HasPrefix(field, "-")
		if desc {
			field = strings.TrimSpace(strings.TrimPrefix(field, "-"))
		}
		if field == "" {
			return nil, fmt.Errorf("%w: invalid ordering field %q", ErrBadRequest, part)
		}
		ordering = append(ordering, orderExpr{Field: field, Desc: desc})
	}
	return ordering, nil
}

func fieldAllowed(field string, allowed []string) bool {
	field = strings.TrimSpace(field)
	if !isSafeDBField(field) {
		return false
	}
	if len(allowed) == 0 {
		return false
	}
	for _, item := range allowed {
		if field == strings.TrimSpace(item) {
			return true
		}
	}
	return false
}

func isSafeDBField(field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	for index, item := range field {
		if item == '_' {
			continue
		}
		if unicode.IsLetter(item) {
			continue
		}
		if unicode.IsDigit(item) && index > 0 {
			continue
		}
		return false
	}
	return true
}

func mapGormError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
