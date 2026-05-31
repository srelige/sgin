package sgin

import "github.com/gin-gonic/gin"

// Serializer 负责请求绑定和响应转换。
// 用户可以自定义 Serializer 做字段裁剪、DTO 转换或输入校验。
type Serializer[T any] interface {
	BindCreate(c *gin.Context) (*T, error)
	BindUpdate(c *gin.Context, obj *T) (*T, error)
	ToListResponse(c *gin.Context, items []T) any
	ToRetrieveResponse(c *gin.Context, obj *T) any
	ToCreateResponse(c *gin.Context, obj *T) any
	ToUpdateResponse(c *gin.Context, obj *T) any
	ToPartialUpdateResponse(c *gin.Context, obj *T) any
}

// DefaultSerializer 是默认 JSON Serializer。
// 它直接把请求 JSON 绑定到模型，并直接把模型作为响应数据返回。
type DefaultSerializer[T any] struct{}

// BindCreate 绑定创建请求体。
func (s DefaultSerializer[T]) BindCreate(c *gin.Context) (*T, error) {
	var obj T
	if err := c.ShouldBindJSON(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

// BindUpdate 将更新请求体绑定到已加载对象。
// 绑定到已有对象可以在 JSON 缺少字段时尽量保留旧值。
func (s DefaultSerializer[T]) BindUpdate(c *gin.Context, obj *T) (*T, error) {
	if err := c.ShouldBindJSON(obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// ToListResponse 将对象列表转换为响应数据。
func (s DefaultSerializer[T]) ToListResponse(c *gin.Context, items []T) any {
	return items
}

// ToRetrieveResponse 将详情对象转换为响应数据。
func (s DefaultSerializer[T]) ToRetrieveResponse(c *gin.Context, obj *T) any {
	return obj
}

// ToCreateResponse 将创建成功对象转换为响应数据。
func (s DefaultSerializer[T]) ToCreateResponse(c *gin.Context, obj *T) any {
	return obj
}

// ToUpdateResponse 将全量更新成功对象转换为响应数据。
func (s DefaultSerializer[T]) ToUpdateResponse(c *gin.Context, obj *T) any {
	return obj
}

// ToPartialUpdateResponse 将局部更新成功对象转换为响应数据。
func (s DefaultSerializer[T]) ToPartialUpdateResponse(c *gin.Context, obj *T) any {
	return obj
}
