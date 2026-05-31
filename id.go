package sgin

import (
	"fmt"
	"reflect"
	"strconv"
)

// ParseID 将路由中的字符串 ID 转为 Repository 使用的泛型 ID。
// MVP 支持 string、int、int64、uint、uint64，并兼容这些基础类型的自定义别名。
func ParseID[ID comparable](raw string) (ID, error) {
	var zero ID
	t := reflect.TypeOf(zero)
	if t == nil {
		return zero, fmt.Errorf("%w: unsupported nil id type", ErrBadRequest)
	}

	value := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		value.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return zero, fmt.Errorf("%w: invalid id %q", ErrBadRequest, raw)
		}
		value.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return zero, fmt.Errorf("%w: invalid id %q", ErrBadRequest, raw)
		}
		value.SetUint(parsed)
	default:
		return zero, fmt.Errorf("%w: unsupported id type %s", ErrBadRequest, t.String())
	}
	return value.Interface().(ID), nil
}
