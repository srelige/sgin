package sgin

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Serializer[T any] interface {
	BindCreate(c *gin.Context) (*T, error)
	BindUpdate(c *gin.Context, obj *T) (*T, error)
	ToListResponse(c *gin.Context, items []T) any
	ToRetrieveResponse(c *gin.Context, obj *T) any
	ToCreateResponse(c *gin.Context, obj *T) any
	ToUpdateResponse(c *gin.Context, obj *T) any
	ToPartialUpdateResponse(c *gin.Context, obj *T) any
}

type serializerValidator interface {
	ValidateSerializer(actions ...string) error
}

func validateSerializer[T any](serializer Serializer[T], actions ...string) {
	if serializer == nil {
		panic("sgin: Serializer is required; choose FullModelSerializer or provide a custom Serializer")
	}
	if validator, ok := serializer.(serializerValidator); ok {
		if err := validator.ValidateSerializer(actions...); err != nil {
			panic(err)
		}
	}
}

type FullModelSerializer[T any] struct{}

func (s FullModelSerializer[T]) BindCreate(c *gin.Context) (*T, error) {
	var obj T
	if err := c.ShouldBindJSON(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (s FullModelSerializer[T]) BindUpdate(c *gin.Context, obj *T) (*T, error) {
	if err := c.ShouldBindJSON(obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (s FullModelSerializer[T]) ToListResponse(c *gin.Context, items []T) any {
	return items
}

func (s FullModelSerializer[T]) ToRetrieveResponse(c *gin.Context, obj *T) any {
	return obj
}

func (s FullModelSerializer[T]) ToCreateResponse(c *gin.Context, obj *T) any {
	return obj
}

func (s FullModelSerializer[T]) ToUpdateResponse(c *gin.Context, obj *T) any {
	return obj
}

func (s FullModelSerializer[T]) ToPartialUpdateResponse(c *gin.Context, obj *T) any {
	return obj
}

func (s FullModelSerializer[T]) ValidateSerializer(actions ...string) error {
	return nil
}

type ModelSerializer[T any] struct {
	ReadFields    []string
	WriteFields   []string
	ExcludeFields []string
}

func (s ModelSerializer[T]) BindCreate(c *gin.Context) (*T, error) {
	if err := s.validateWritePayload(c); err != nil {
		return nil, err
	}
	var obj T
	if err := c.ShouldBindBodyWith(&obj, binding.JSON); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (s ModelSerializer[T]) BindUpdate(c *gin.Context, obj *T) (*T, error) {
	if err := s.validateWritePayload(c); err != nil {
		return nil, err
	}
	if err := c.ShouldBindBodyWith(obj, binding.JSON); err != nil {
		return nil, err
	}
	return obj, nil
}

func (s ModelSerializer[T]) ToListResponse(c *gin.Context, items []T) any {
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, s.objectResponse(&items[i]))
	}
	return out
}

func (s ModelSerializer[T]) ToRetrieveResponse(c *gin.Context, obj *T) any {
	return s.objectResponse(obj)
}

func (s ModelSerializer[T]) ToCreateResponse(c *gin.Context, obj *T) any {
	return s.objectResponse(obj)
}

func (s ModelSerializer[T]) ToUpdateResponse(c *gin.Context, obj *T) any {
	return s.objectResponse(obj)
}

func (s ModelSerializer[T]) ToPartialUpdateResponse(c *gin.Context, obj *T) any {
	return s.objectResponse(obj)
}

func (s ModelSerializer[T]) ValidateSerializer(actions ...string) error {
	if len(serializerFieldMap[T]()) == 0 {
		return fmt.Errorf("sgin: ModelSerializer requires a struct model")
	}
	if err := s.validateDeclaredFields(); err != nil {
		return err
	}
	if !s.hasReadStrategy() {
		return fmt.Errorf("sgin: ModelSerializer requires ReadFields or ExcludeFields")
	}
	if needsWriteStrategy(actions) && !s.hasWriteStrategy() {
		return fmt.Errorf("sgin: ModelSerializer requires WriteFields or ExcludeFields for write actions")
	}
	return nil
}

func (s ModelSerializer[T]) validateDeclaredFields() error {
	known := serializerFieldMap[T]()
	for _, group := range [][]string{s.ReadFields, s.WriteFields, s.ExcludeFields} {
		for _, field := range normalizeSerializerFields(group) {
			if _, ok := known[field]; !ok {
				return fmt.Errorf("sgin: serializer field %q does not exist", field)
			}
		}
	}
	return nil
}

func (s ModelSerializer[T]) validateWritePayload(c *gin.Context) error {
	allowed, err := s.allowedWriteFields()
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := c.ShouldBindBodyWith(&payload, binding.JSON); err != nil {
		return err
	}
	for field := range payload {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%w: field %q is not writable", ErrBadRequest, field)
		}
	}
	return nil
}

func (s ModelSerializer[T]) objectResponse(obj *T) map[string]any {
	fields := s.readFields()
	out := make(map[string]any, len(fields))
	value := reflect.Indirect(reflect.ValueOf(obj))
	for _, field := range fields {
		if meta, ok := serializerFieldMap[T]()[field]; ok {
			out[field] = value.FieldByIndex(meta.Index).Interface()
		}
	}
	return out
}

func (s ModelSerializer[T]) readFields() []string {
	fields := normalizeSerializerFields(s.ReadFields)
	if len(fields) == 0 {
		fields = allSerializerFields[T]()
	}
	excluded := serializerFieldSet(s.ExcludeFields)
	out := fields[:0]
	for _, field := range fields {
		if _, ok := excluded[field]; !ok {
			out = append(out, field)
		}
	}
	return out
}

func (s ModelSerializer[T]) allowedWriteFields() (map[string]struct{}, error) {
	fields := normalizeSerializerFields(s.WriteFields)
	if len(fields) == 0 {
		fields = allSerializerFields[T]()
	}
	known := serializerFieldMap[T]()
	allowed := map[string]struct{}{}
	excluded := serializerFieldSet(s.ExcludeFields)
	for _, field := range fields {
		if _, ok := known[field]; !ok {
			return nil, fmt.Errorf("sgin: serializer field %q does not exist", field)
		}
		if _, ok := excluded[field]; ok {
			continue
		}
		allowed[field] = struct{}{}
	}
	return allowed, nil
}

func (s ModelSerializer[T]) hasReadStrategy() bool {
	return len(s.ReadFields) > 0 || len(s.ExcludeFields) > 0
}

func (s ModelSerializer[T]) hasWriteStrategy() bool {
	return len(s.WriteFields) > 0 || len(s.ExcludeFields) > 0
}

func allSerializerFields[T any]() []string {
	items := make([]string, 0, len(serializerFieldMap[T]()))
	for _, meta := range serializerFields[T]() {
		items = append(items, meta.Name)
	}
	return items
}

func serializerFieldSet(fields []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, field := range normalizeSerializerFields(fields) {
		set[field] = struct{}{}
	}
	return set
}

func normalizeSerializerFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

type serializerField struct {
	Name  string
	Index []int
}

func serializerFields[T any]() []serializerField {
	typ := reflect.TypeOf((*T)(nil)).Elem()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	fields := make([]serializerField, 0, typ.NumField())
	collectSerializerFields(typ, nil, &fields)
	return fields
}

func serializerFieldMap[T any]() map[string]serializerField {
	fields := serializerFields[T]()
	out := make(map[string]serializerField, len(fields))
	for _, field := range fields {
		out[field.Name] = field
	}
	return out
}

func collectSerializerFields(typ reflect.Type, parent []int, fields *[]serializerField) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		index := append(append([]int{}, parent...), i)
		name, inline, ok := serializerJSONName(field)
		if !ok {
			continue
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if inline && fieldType.Kind() == reflect.Struct {
			collectSerializerFields(fieldType, index, fields)
			continue
		}
		*fields = append(*fields, serializerField{Name: name, Index: index})
	}
}

func serializerJSONName(field reflect.StructField) (name string, inline bool, ok bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		if field.Anonymous {
			return "", true, true
		}
		return field.Name, false, true
	}
	return parts[0], false, true
}

func needsWriteStrategy(actions []string) bool {
	for _, action := range actions {
		switch normalizeActionKey(action) {
		case ActionCreate, ActionUpdate, ActionPartialUpdate:
			return true
		}
	}
	return false
}
