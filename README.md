# sgin

`sgin` 是一个基于 `github.com/gin-gonic/gin` 的增强型 Go Web API 包。它保留 Gin 的原生用法，同时提供配置自动加载、统一响应、权限系统、分页过滤和 DRF-like 的泛型 `ModelViewSet`。

## 当前功能

- 兼容 sgin 原生路由：`App` 嵌入 `*gin.Engine`，可直接使用 `GET`、`POST`、`Use`、`Group` 等方法。
- 配置加载：默认读取 `config.yaml`；不存在时读取 `config.example.yaml`；两者都不存在时生成 `config.example.yaml` 并按它启动。
- 配置优先级：代码显式配置 > 环境变量 > `config.yaml` > `config.example.yaml` > 默认配置。
- 目录初始化：`app.InitDir()` 可创建推荐业务目录结构。
- 统一响应：业务接口可以用框架 helper 返回一致的 `code/message/data/error` JSON 结构，避免每个 handler 自己拼响应格式。
- 自动 CRUD：普通数据库模型注册后即可获得列表、详情、创建、更新、删除接口；只读资源和单 URL 数据接口也有对应封装。
- 数据访问扩展：默认走 GORM；需要接入非 GORM 存储、远程服务或特殊查询时，可以替换数据访问实现。
- 输入输出转换：默认直接绑定和返回 JSON；需要隐藏字段或让列表、详情、创建、更新返回不同结构时，可以自定义转换逻辑。
- 权限系统：支持用户、用户组、角色、权限点、动态路由权限，以及 `LoadAccess`、`RequireAnyGroup`、`RequireAnyRole` 等中间件。
- 用户登录：`user.enabled=true` 时提供登录、JWT access token、refresh token 和管理员账号初始化。
- 极简管理界面：可选开启内置 Admin UI，用于创建账号、配置用户组/角色/权限点和动态路由权限。
- 分页过滤：可选启用 page/page_size 分页，解析 `search`、`ordering` 和字段过滤参数。

## 安装依赖

```bash
go get github.com/srelige/sgin
```

当前模块名是：

```txt
github.com/srelige/sgin
```

## 最小示例

```go
package main

import "github.com/srelige/sgin"

func main() {
	app := sgin.New()

	app.GET("/ping", func(c *sgin.Context) {
		c.JSON(200, sgin.H{"message": "pong"})
	})

	app.Run()
}
```

第一次启动且当前目录没有 `config.yaml` 和 `config.example.yaml` 时，框架会自动生成 `config.example.yaml`，但不会生成 `config.yaml`。
之后如果仍没有 `config.yaml`，框架会继续读取 `config.example.yaml`，所以其中的 `jwt.secret` 会保持稳定。

可选初始化业务目录：

```go
app := sgin.New()
_ = app.InitDir()
```

会创建一套推荐业务目录；这只是组织建议，用户可以按自己的项目习惯调整：

```txt
dao/
handlers/
middlewares/
models/
routers/
serializers/
services/
utils/
```

建议职责划分：

```txt
dao/          数据访问层，放 GORM/SQL 查询、Repository 实现、数据库读写封装。
handlers/     HTTP 入口层，放 Gin handler，负责解析请求、调用 service、返回响应。
middlewares/  请求中间件，放租户加载、用户组注入、审计、限流、请求上下文准备等逻辑。
models/       数据模型，放 GORM 模型和业务实体等。
routers/      路由组织入口，放普通 sgin 路由分组、ViewSet 注册聚合等。
serializers/  输入输出转换层，放 ModelViewSet Serializer 实现等。
services/     业务层，放业务规则、流程编排、事务、跨 DAO 调用和外部服务调用。
ui/           可选静态资源目录，通常只放业务自己的页面资源；sgin 内置 Admin UI 已经通过 embed 打进包里。
utils/        通用工具，放与具体业务边界无关的辅助函数。
```

## 配置文件

示例配置：

```yaml
app:
  name: sgin-app
  env: development
  debug: true

server:
  addr: ":8080"
  mode: debug

database:
  driver: sqlite
  dsn: "./app.db"
  auto_migrate: false

redis:
  enabled: false
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

rest:
  pagination: false
  default_page: 1
  default_page_size: 20
  max_page_size: 100
  static_dir: "./uploads"

admin:
  enabled: false
  path: "/sgin-admin"

user:
  enabled: true
  path: "/login"
  admin:
    init: true
    username: admin

jwt:
  secret: "启动生成的随机密钥"
  expired: 1
  refresh_expired: 168
```

支持的环境变量包括：

```txt
SGIN_APP_NAME
SGIN_APP_ENV
SGIN_APP_DEBUG
SGIN_SERVER_ADDR
SGIN_SERVER_MODE
SGIN_DATABASE_DRIVER
SGIN_DATABASE_DSN
SGIN_REDIS_ENABLED
SGIN_REDIS_ADDR
SGIN_ADMIN_ENABLED
SGIN_ADMIN_PATH
SGIN_REST_PAGINATION
SGIN_REST_DEFAULT_PAGE
SGIN_REST_DEFAULT_PAGE_SIZE
SGIN_REST_MAX_PAGE_SIZE
SGIN_REST_STATIC_DIR
SGIN_USER_ENABLED
SGIN_USER_PATH
SGIN_USER_ADMIN_INIT
SGIN_USER_ADMIN_USERNAME
SGIN_JWT_SECRET
SGIN_JWT_EXPIRED
SGIN_JWT_REFRESH_EXPIRED
```

## 用户登录

当配置中开启用户功能：

```yaml
user:
  enabled: true
  path: "/login"
  admin:
    init: true
    username: admin
jwt:
  secret: "your-secret"
  expired: 1
  refresh_expired: 168
```

框架启动时会通过 GORM 连接配置中的数据库，自动迁移内置用户和访问控制相关表。`user.admin.init=true` 且不存在管理员账号时，会插入一条管理员账号；密码用 bcrypt 哈希后入库，明文密码只会在首次创建成功时输出到日志。管理员初始化还会创建内置 `admin` 用户组和 `admin` 组，并把管理员账号加入 `admin` 组。

支持的 `database.driver`：`sqlite` / `sqlite3`、`mysql`、`pg` / `postgres` / `postgresql`。

登录接口：

```txt
POST /login
POST /login/refresh
```

登录成功返回 `access_token` 和 `refresh_token`。`jwt.expired` 是 access token 有效期，单位小时；`jwt.refresh_expired` 是 refresh token 有效期，单位小时。refresh token 只在数据库保存 SHA-256 摘要，刷新时会轮换。用户表包含 `enabled` 字段；账号被禁用时会在密码校验前拒绝登录。

框架不提供全局隐式认证配置；哪个接口需要 token，就在该路由上显式挂认证中间件。保护接口可使用：

```go
app.GET("/me", app.JWTAuth(), func(c *sgin.Context) {
	user, _ := c.Get("user")
	c.JSON(200, user)
})
```

## 极简 Admin UI

内置 Admin UI 默认关闭，适合开发期或内网环境快速配置账号、用户组、角色、权限点和动态路由权限。它不是完整后台系统，不建议直接暴露到公网。

开启配置：

```yaml
admin:
  enabled: true
  path: "/sgin-admin"
```

也可以用环境变量开启：

```txt
SGIN_ADMIN_ENABLED=true
SGIN_ADMIN_PATH=/sgin-admin
```

Admin UI 页面已嵌入到 sgin 包中，业务项目引用 sgin 后不需要复制静态文件。源码中的 `ui/admin.html` 只用于维护这个内置页面。开启后访问：

```txt
GET /sgin-admin
```

页面会调用受保护的 Admin API。使用前先通过登录接口获取 access token，并在页面里填入 token。Admin API 会要求当前用户属于 `admin` 组。

Admin UI 第一版提供这些能力：

```txt
创建用户
启用/禁用用户
创建用户组
创建角色
创建权限点
创建路由权限
绑定用户到用户组
绑定用户到角色
绑定用户组到角色
绑定角色到权限点
查询全部访问控制状态
按用户名、用户组、角色、权限码或路由快速筛选
```

通过 Admin UI 创建用户时会写入 bcrypt 密码哈希，并保留请求里显式传入的启用/禁用状态。

## 文件上传

`sgin` 的默认 CRUD 面向 JSON 请求。`multipart/form-data` 上传建议使用普通 Gin handler、APIView 或覆盖 ViewSet 的指定方法，在业务层读取文件、校验表单、保存到本地目录并决定入库字段。

本地落盘场景可以使用 `rest.static_dir` 作为统一上传目录：

```yaml
rest:
  static_dir: "./uploads"
```

`sgin` 不会自动保存上传文件，也不会自动把该目录注册成静态资源服务；文件保存逻辑由业务 handler 显式完成。

```go
app.POST("/upload", app.JWTAuth(), func(c *sgin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		sgin.JSON(c, http.StatusBadRequest, sgin.Fail(http.StatusBadRequest, "invalid file", err.Error()))
		return
	}

	cfg := app.Config()
	if err := os.MkdirAll(cfg.REST.StaticDir, 0o755); err != nil {
		sgin.HandleError(c, err)
		return
	}

	name := filepath.Base(file.Filename)
	dst := filepath.Join(cfg.REST.StaticDir, name)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		sgin.HandleError(c, err)
		return
	}

	sgin.JSON(c, http.StatusCreated, sgin.Created(sgin.H{"path": dst}))
})
```

## 显式配置

```go
cfg := sgin.DefaultConfig()
cfg.Server.Addr = ":9000"

app := sgin.New(
	sgin.WithConfig(cfg),
)
```

`WithConfig` 的优先级最高，不会再被配置文件或环境变量覆盖。

## 使用 ModelViewSet

`ModelViewSet` 是 DRF-like 的全功能 CRUD 入口。普通数据库模型不需要手写 `List`、`Find`、`Create`、`Update`、`Delete`，注册模型后会自动生成 RESTful 路由，并使用配置中的数据库完成增删改查。

下面用一个更接近正式项目的 `Book` 示例说明推荐分层。示例里 `POST /books` 会被 `Handlers` 接管，创建前走 service 层校验：只收录 1990 之后的书籍。

推荐目录：

```txt
dao/
  book_dao.go
handlers/
  book_handler.go
models/
  book.go
routers/
  book.go
services/
  book_service.go
```

模型层：

```go
package models

type Book struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name"`
	Info string `json:"info"`
	Year int    `json:"year"`
}
```

DAO 层负责数据库访问。这里用 GORM 举例，实际项目可以替换成自己的 Repository、SQL 或其他存储实现：

```go
package dao

import (
	"context"
	"github.com/srelige/sgin"
	"temp/models"
)

type BookDAO struct {
	app *sgin.App
}

func NewBookDAO(app *sgin.App) *BookDAO {
	return &BookDAO{app: app}
}

func (d *BookDAO) Create(ctx context.Context, book *models.Book) error {
	db, err := d.app.DB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(book).Error
}
```

Service 层负责业务规则和流程编排：

```go
package services

import (
	"context"
	"errors"
	"temp/dao"
	"temp/models"
)

var ErrBookYearTooOld = errors.New("book year too old")

type BookService struct {
	bookDAO *dao.BookDAO
}

func NewBookService(bookDAO *dao.BookDAO) *BookService {
	return &BookService{bookDAO: bookDAO}
}

func (s *BookService) Create(ctx context.Context, book *models.Book) error {
	if book.Year <= 1990 {
		return ErrBookYearTooOld
	}
	return s.bookDAO.Create(ctx, book)
}
```

Handler 层负责 HTTP 请求和响应转换，不承载核心业务规则：

```go
package handlers

import (
	"errors"
	"net/http"
	"github.com/srelige/sgin"
	"temp/models"
	"temp/services"

	"github.com/gin-gonic/gin"
)

type BookHandler struct {
	bookService *services.BookService
}

func NewBookHandler(bookService *services.BookService) *BookHandler {
	return &BookHandler{bookService: bookService}
}

func (h *BookHandler) Create(c *gin.Context) {
	var book models.Book
	if err := c.ShouldBindJSON(&book); err != nil {
		sgin.JSON(c, http.StatusBadRequest, sgin.Fail(http.StatusBadRequest, "invalid book", err.Error()))
		return
	}

	if err := h.bookService.Create(c.Request.Context(), &book); err != nil {
		if errors.Is(err, services.ErrBookYearTooOld) {
			sgin.JSON(c, http.StatusBadRequest, sgin.Fail(http.StatusBadRequest, "只收录1990之后的书籍", "book_year_too_old"))
			return
		}
		sgin.HandleError(c, err)
		return
	}

	sgin.JSON(c, http.StatusCreated, sgin.Created(book))
}
```

Router 层负责组装依赖和注册 ViewSet：

```go
package routers

import (
	"github.com/srelige/sgin"
	"temp/dao"
	"temp/handlers"
	"temp/models"
	"temp/services"

	"github.com/gin-gonic/gin"
)

func Book(app *sgin.App) sgin.ViewSet {
	bookDAO := dao.NewBookDAO(app)
	bookService := services.NewBookService(bookDAO)
	bookHandler := handlers.NewBookHandler(bookService)

	return &sgin.ModelViewSet[models.Book, uint]{
		BasePath: "/books",
		Handlers: map[string]gin.HandlerFunc{
			"post": bookHandler.Create,
		},
	}
}
```

应用入口注册路由和初始化表：

```go
package main

import (
	"github.com/srelige/sgin"
	"temp/models"
	"temp/routers"
)

func main() {
	app := sgin.New()

	if err := app.InitTable(&models.Book{}); err != nil {
		panic(err)
	}

	app.Register(routers.Book(app))
	app.Run()
}
```

注册后会生成以下路由：

```txt
GET     /books
POST    /books
GET     /books/:id
PUT     /books/:id
PATCH   /books/:id
DELETE  /books/:id
```

其中 `POST /books` 由 `Handlers` 中配置的自定义 handler 接管，其余方法继续使用 `ModelViewSet` 的默认 CRUD 行为。

### 额外动作路由

`ModelViewSet` 支持轻量额外动作路由，用于把 `reset-password`、`sync`、`export` 这类非 CRUD 动作继续聚合在资源 ViewSet 下。

```go
app.Register(&sgin.ModelViewSet[models.User, uint]{
	BasePath: "/users",
	Auth:     []string{"all"},
	Middlewares: []gin.HandlerFunc{
		app.LoadAccess(),
		app.RequireRoutePermission(),
	},
	ExtraActions: []sgin.ExtraAction{
		{
			Method: "post",
			Path:   "reset-password",
			Detail: true,
			Middlewares: []gin.HandlerFunc{
				app.RequireAnyGroup("admin", "ops"),
			},
			Handler: handlers.ResetPassword,
		},
		{
			Method:  "get",
			Path:    "export",
			Handler: handlers.ExportUsers,
		},
	},
})
```

上面对外提供：

```txt
POST /users/:id/reset-password
GET  /users/export
```

`Detail: true` 表示动作挂在单个对象上，路由里会带 `:id`；默认是集合动作，不带 `:id`。额外动作会继承 ViewSet 的 `Auth` 和 `Middlewares`，然后追加动作自己的 `Middlewares`。动态路由权限仍按实际注册路由配置，例如：

```txt
POST /users/:id/reset-password -> users.reset_password
GET  /users/export             -> users.export
```

为什么需要 dispatcher：Gin 不能在同一个 method 下同时直接注册 `GET /users/:id` 和 `GET /users/export`，因为 `:id` 是通配段；但这两类路由在业务上都合理，也和 DRF 的 `@action(detail=false)` 需求一致。因此 sgin 会把这类集合动作登记到 ViewSet 内部 dispatcher：请求进入 `GET /users/:id` 入口后，框架先判断这一段是否命中集合动作，命中则执行 `GET /users/export`，否则继续走默认详情查询。

为了避免歧义，集合动作 path 必须是单个普通路径段；当它能被当前 ViewSet 的 ID 类型解析成功时，启动注册阶段会直接失败。例如 `ModelViewSet[User, uint]` 不能注册 `GET /users/123` 作为集合动作，因为它和 `GET /users/:id` 无法区分；`GET /users/export` 可以正常工作。

数据库连接来自 `config.yaml` 的 `database.driver` 和 `database.dsn`，这份配置也会被内置用户初始化复用，代码里不需要重复写。

`app.InitTable` 用于显式初始化业务表，支持一次传入一个或多个模型；表已存在时会跳过，表不存在时创建。快速开发时也可以在配置文件里设置 `database.auto_migrate: true`，让默认 GORM 仓库在接口首次使用时自动迁移业务模型。

只读接口可以使用：

```go
app.Register(&sgin.ReadOnlyModelViewSet[models.Book, uint]{
	BasePath: "/books",
})
```

只会注册：

```txt
GET /books
GET /books/:id
```

### 默认 Repository 和自定义 Repository

普通 GORM 模型不需要手写数据访问层。默认情况下：

```go
app.Register(&sgin.ModelViewSet[Book, uint]{
	BasePath: "/books",
})
```

没有传 `Repository` 时，框架会自动使用默认 GORM Repository，并提供：

```txt
List
Count
Find
Create
Update
Delete
```

数据库连接来自配置中的 `database.driver` 和 `database.dsn`。

`database.auto_migrate` 只影响默认 GORM Repository：

```yaml
database:
  auto_migrate: true
```

开启后，即使没有显式调用 `app.InitTable(&Book{})`，当对应接口第一次使用默认 Repository 时，也会对模型执行 GORM `AutoMigrate`。正式项目更建议保持 `auto_migrate: false`，然后用 `app.InitTable(...)` 或自己的迁移工具显式初始化业务表。

自定义 Repository 指的是：不使用框架默认的 GORM CRUD，而是自己提供一套数据访问实现。例如：

```go
app.Register(&sgin.ModelViewSet[Book, uint]{
	BasePath:   "/books",
	Repository: myBookRepo,
})
```

这里的 `myBookRepo` 就是自定义 Repository。它需要实现框架的 Repository 接口，负责：

```txt
List(ctx, query)
Find(ctx, id)
Create(ctx, obj)
Update(ctx, obj)
Delete(ctx, id)
```

如果自定义 Repository 也希望在分页响应里返回准确 `total`，可以额外实现可选的 `Count(ctx, query)` 方法。没有实现时，框架会退回当前页数量；默认 GORM Repository 已经实现准确 total。

适合使用自定义 Repository 的场景：

```txt
数据来自 Redis、ES、Mongo 或外部 HTTP API
不是单表 CRUD，而是复杂查询
需要特殊 SQL
需要接入已有项目里的 DAO
需要绕过 GORM
需要多租户分库分表
List 或 Find 需要非常特殊的权限过滤或聚合
```

如果只是普通数据库表，建议直接使用默认 GORM Repository；只有存储来源或查询行为明显不同，才手写自定义 Repository。
## 使用 APIView

`APIView` 是 `ModelViewSet` 的单路由子集，适合只有一个 URL 或一个 HTTP method、但仍然需要数据库默认能力的接口。它会按 `Method` 和 `Path` 推断默认动作：

```txt
GET    /resources       -> list
GET    /resources/:id   -> retrieve
POST   /resources       -> create
PUT    /resources/:id   -> update
PATCH  /resources/:id   -> partial_update
DELETE /resources/:id   -> destroy
```

例如只需要 `GET /cars` 查询列表时，不需要手写查库 handler：

```go
package routers

import (
	"github.com/srelige/sgin"
	"temp/models"
)

func Cars(app *sgin.App) sgin.ViewSet {
	return &sgin.APIView[models.Car, uint]{
		Method: "get",
		Path:   "/cars",
	}
}
```

应用入口注册：

```go
app.InitTable(&models.Car{})
app.Register(routers.Cars(app))
```

`APIView` 也支持 `Auth`、`Middlewares`、`Repository`、`Serializer`、分页和过滤配置。需要对请求做二次处理时，可以设置 `Handler` 接管这一个路由；没有设置 `Handler` 时走默认数据库能力。

建议边界：只有需要和数据库交互、且不想手写普通查库/创建/更新/删除逻辑的单 URL 接口，才使用 `APIView`。如果只是 `/ping` 这类不需要数据库的简单接口，建议直接使用 sgin 原生路由，也就是在 `sgin.App` 上直接调用 `GET`、`POST` 等方法。

```go
app.GET("/ping", func(c *gin.Context) {
	c.JSON(200, gin.H{"message": "pong"})
})
```

## 选型与实践指南

sgin 的核心不是把所有接口都塞进 ViewSet，而是给常见资源接口一个省力入口，同时保留 Gin 原生 handler 处理复杂业务。选型时优先看接口是不是围绕单一资源集合展开、输入输出是否接近模型、是否需要默认 Repository 能力。

```txt
普通资源 CRUD                         ModelViewSet
只读资源列表/详情                      ReadOnlyModelViewSet
资源内非 CRUD 动作                     ModelViewSet ExtraActions
单 URL 且想复用默认数据库能力           APIView
上传、导入、支付、审批、批处理          Gin handler + service
简单健康检查、回调、Webhook            Gin handler
跨多个模型或外部系统的业务流程          Gin handler + service
非 GORM、远程服务或特殊查询             自定义 Repository
```

### 业务动作怎么写

业务动作先判断它属于哪个资源边界。

```txt
POST /users/:id/reset-password     用户资源内动作，适合 ExtraActions detail
GET  /books/export                 图书集合动作，适合 ExtraActions collection
POST /orders/:id/pay               如果只是订单内动作，可用 ExtraActions detail
POST /checkout                     跨购物车、订单、支付，适合 Gin handler + service
POST /imports/books                导入流程，适合 Gin handler + service
```

推荐分层是：handler 只处理 HTTP 输入输出，service 承载业务规则、事务和外部服务调用，DAO/Repository 负责数据访问。动作里如果出现多模型事务、远程调用、状态机流转、重试补偿或审计写入，优先把核心逻辑放到 service，不要堆在 ViewSet handler 里。

### 上传怎么写

默认 CRUD 面向 JSON，不负责 multipart。上传场景建议单独设计入口：

```txt
POST /files                         普通文件上传，Gin handler
POST /assets/:id/attachments        资源附件，ExtraActions detail 或 Gin handler
POST /imports/books                 导入文件，Gin handler + service
```

上传 handler 的职责通常包括：读取 `FormFile`、校验文件大小、校验扩展名或 MIME、保存到 `rest.static_dir` 或对象存储、只把文件元数据或对象存储 key 入库。不要让默认 `ModelViewSet.Create` 同时承担 JSON CRUD 和 multipart 上传两种职责。

### 权限怎么组合

认证和组织级访问控制优先用 middleware，ViewSet 权限接口只处理更靠近资源的轻量规则。

```txt
Auth / JWTAuth                 校验身份，失败返回 401
LoadAccess                     加载用户组、角色、权限点
RequireAnyGroup                按用户组限制访问
RequireAnyRole                 按角色限制访问
RequireRoutePermission         按 method + path 查动态权限
Permissions / ActionPermissions 代码内固定请求级规则
ObjectPermissions              单对象级授权
QueryPermissions               列表查询范围收窄
```

推荐顺序是先认证，再加载访问上下文，再做用户组、角色或动态路由权限判断，最后进入 handler。需要后台可配置权限时优先用 `RequireRoutePermission`；只在代码里固定的简单规则可以用 `ActionPermissions`；对象归属、租户隔离这类和数据本身相关的判断放到 `ObjectPermissions` 或 `QueryPermissions`。

### APIView 什么时候用

`APIView` 适合“单 URL + 仍想复用默认数据库能力”的接口。例如只暴露一个列表、一个详情或一个单独创建入口，但不想手写 Repository 调用、分页和过滤。

不适合 APIView 的场景：

```txt
/ping、/health 这类无数据库接口
完整资源 CRUD
资源内额外动作
支付、审批、导入、批处理这类流程接口
输入输出和模型差异很大的接口
```

这些场景分别使用普通 Gin handler、`ModelViewSet`、`ExtraActions` 或 service 驱动的业务 handler。

### ViewSet 什么时候不用

如果接口已经明显不是“围绕一个模型做 CRUD”，不要为了复用 ViewSet 而扭曲业务边界。

```txt
请求体和数据库模型差异很大
一个请求要写多个模型或多个库
需要复杂事务、补偿、审计或外部服务调用
需要处理大文件、流式响应或长时间任务
权限和数据范围高度定制，默认 Repository 反而碍事
接口是业务流程入口，而不是资源集合入口
```

这类接口用普通 Gin handler 更直接。sgin 的 `App` 嵌入 `*gin.Engine`，所以原生 Gin 路由和 ViewSet 可以自然混用。
## 权限示例

sgin 的访问控制模型由五类数据组成：

```txt
users              登录主体
access groups      用户组，例如 dev、ops、cmdb
access roles       角色，例如 admin、cmdb_reader、book_editor
permissions        权限点，例如 cmdb.view、book.create
route permissions  method + path 到权限点的绑定
```

关系建议：

```txt
user -> groups
user -> roles
group -> roles
role -> permissions
route -> permission
```

`admin` 是内置 admin 组。只要用户通过直接角色或用户组间接拥有 `admin` role，访问控制中间件会直接放行。

### 基础认证

`Auth` 只负责 token 校验。访问受保护接口时，请求头需要携带登录返回的 access token：

```txt
Authorization: Bearer <access_token>
```

```go
app.Register(&sgin.ModelViewSet[Book, uint]{
	BasePath: "/books",
	Auth:     []string{"all"},
})
```

### 自定义认证 middleware

sgin 不提供 AuthRegistry，也不复制 DRF 的 authentication classes。框架底层是 Gin，自定义认证应直接写 Gin middleware：API Key、内部服务 token、自定义 Bearer token、签名认证、租户认证都可以按普通 middleware 组合。

内置 `Auth` 字段只表示是否使用 sgin 用户系统的 `JWTAuth()`。如果接口使用 API Key 或其他认证方式，不需要设置 `Auth`，直接把认证 middleware 放进 `Middlewares` 或普通 Gin 路由链。

API Key 示例：

```go
func APIKeyAuth(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != expected {
			sgin.JSON(c, http.StatusUnauthorized, sgin.Fail(http.StatusUnauthorized, "invalid api key", "invalid_token"))
			c.Abort()
			return
		}
		c.Next()
	}
}

app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Middlewares: []gin.HandlerFunc{
		APIKeyAuth("dev-secret"),
	},
})
```

内部服务 token 示例：

```go
func InternalTokenAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Internal-Token") != secret {
			sgin.JSON(c, http.StatusUnauthorized, sgin.Fail(http.StatusUnauthorized, "invalid internal token", "invalid_token"))
			c.Abort()
			return
		}
		c.Set("internal_service", true)
		c.Next()
	}
}

app.POST("/jobs/rebuild-index", InternalTokenAuth("internal-secret"), func(c *gin.Context) {
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
})
```

如果自定义认证希望继续复用 `LoadAccess()`、`RequireAnyGroup()`、`RequireAnyRole()` 或 `RequireRoutePermission()`，认证成功后需要写入 sgin 能识别的 `user`。最简单做法是查到内置 `UserAccount` 后写入 Gin Context：

```go
func CustomBearerAuth(app *sgin.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		userID, ok := verifyCustomToken(token)
		if !ok {
			sgin.JSON(c, http.StatusUnauthorized, sgin.Fail(http.StatusUnauthorized, "invalid token", "invalid_token"))
			c.Abort()
			return
		}

		db, err := app.DB()
		if err != nil {
			sgin.HandleError(c, err)
			c.Abort()
			return
		}
		var user sgin.UserAccount
		if err := db.First(&user, userID).Error; err != nil {
			sgin.JSON(c, http.StatusUnauthorized, sgin.Fail(http.StatusUnauthorized, "invalid token", "invalid_token"))
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Middlewares: []gin.HandlerFunc{
		CustomBearerAuth(app),
		app.LoadAccess(),
		app.RequireAnyGroup("dev", "ops"),
	},
})
```

推荐边界是：认证方式由 middleware 决定，认证成功后按需写入 `user`；权限仍然用 `LoadAccess()`、用户组/角色/动态路由权限或 ViewSet 权限接口组合。

### 加载访问上下文

`LoadAccess` 会读取当前用户的用户组、角色和权限点，并写入请求上下文。后续 middleware、handler 或 service 可以通过访问上下文继续判断。

```go
app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Auth:   []string{"all"},
	Middlewares: []gin.HandlerFunc{
		app.LoadAccess(),
	},
})
```

### 按用户组限制访问

`RequireAnyGroup` 接收一个或多个用户组；命中任意一个就放行。拥有 `admin` 组 的用户会自动放行。

```go
app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Auth:   []string{"all"},
	Middlewares: []gin.HandlerFunc{
		app.LoadAccess(),
		app.RequireAnyGroup("dev", "ops"),
	},
})
```

### 按角色限制访问

`RequireAnyRole` 接收一个或多个角色；命中任意一个就放行。拥有 `admin` 组 的用户会自动放行。

```go
app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Auth:   []string{"all"},
	Middlewares: []gin.HandlerFunc{
		app.LoadAccess(),
		app.RequireAnyRole("cmdb_reader", "cmdb_admin"),
	},
})
```

### 动态路由权限

动态路由权限通过数据库中的 `method + path -> permission` 配置控制访问。请求进入时，`RequireRoutePermission` 会按当前路由查找需要的权限点，再判断当前用户是否通过角色拥有该权限点。

```go
app.Register(&sgin.APIView[models.CMDBAsset, uint]{
	Method: "get",
	Path:   "/cmdb",
	Auth:   []string{"all"},
	Middlewares: []gin.HandlerFunc{
		app.LoadAccess(),
		app.RequireRoutePermission(),
	},
})
```

数据库中可配置：

```txt
GET /cmdb -> cmdb.view
```

然后让角色拥有该权限点：

```txt
cmdb_reader -> cmdb.view
dev group -> cmdb_reader role
zhangsan -> dev group
```

最终效果是：张三访问 `GET /cmdb` 会被允许；没有 `cmdb.view` 权限点的用户会被拒绝；拥有 `admin` 组 的用户会直接放行。

### 简单权限接口

权限不提供全局隐式默认配置；需要权限校验时，应在具体 ViewSet、APIView 或路由注册处显式声明。`Permissions`、`ActionPermissions`、`ObjectPermissions` 和 `QueryPermissions` 可用于 ViewSet 内的轻量授权、对象级授权和列表查询范围收窄。复杂的用户组、角色和动态路由权限建议优先使用 middleware。

内置简单权限名称：

```txt
allow_any
authenticated
admin
read_only
```

### 认证与权限错误语义

sgin 固定区分认证失败和权限失败，方便客户端稳定判断是重新登录还是展示无权限提示。

`401 Unauthorized` 表示身份没有成立：

```txt
authentication_required  未提供认证信息
invalid_authorization    Authorization 请求头格式错误
invalid_token            token 无效或对应用户不存在
token_expired            access token 已过期
invalid_token_type       token 类型不符合当前接口要求
account_disabled         账号已禁用
invalid_credentials      登录用户名或密码错误
invalid_refresh_token    refresh token 无效或已被轮换
refresh_token_expired    refresh token 已过期
```

`403 Forbidden` 表示身份已经成立，但权限不足：

```txt
permission_denied                 通用权限拒绝
admin_required                    需要管理员权限
group_required                    需要命中指定用户组
role_required                     需要命中指定角色
route_permission_required         当前用户缺少路由要求的权限点
route_permission_not_configured   当前路由没有配置动态权限
```

这些 code 放在统一响应的 `error` 字段里；HTTP 状态码用于粗粒度语义，`error` 用于前端分支和日志检索。业务自定义 `Permission` 仍然可以返回自己的 `Decision.Code`，框架不会强行覆盖。
## 分页和过滤

默认情况下 `rest.pagination=false`，列表接口不分页。即使请求里传了 `page` / `page_size`，框架也会忽略分页参数，直接返回完整列表。业务过滤条件由 `FilterFields` 显式声明：

```go
app.Register(&sgin.ModelViewSet[Book, uint]{
	BasePath:        "/books",
	FilterFields:    []string{"name", "info", "year"},
	OrderingFields:  []string{"name", "year", "id"},
	DefaultOrdering: "-id",
})
```

声明后，请求可以不带业务过滤条件，也可以带一个或多个业务过滤条件：

```txt
GET /books
GET /books?info=aa
GET /books?year=1111
GET /books?info=aa&year=1111
GET /books?year__gte=1990&year__lte=2024
GET /books?name__contains=go
GET /books?info__in=aa,bb
```

多个业务过滤条件默认是 AND 关系。没有声明在 `FilterFields` 里的字段不能用于默认 GORM 过滤。默认 GORM Repository 支持这些过滤写法：

```txt
field=value           等值
field__exact=value    等值
field__gt=value       大于
field__gte=value      大于等于
field__lt=value       小于
field__lte=value      小于等于
field__contains=value 包含，映射为 LIKE %value%
field__in=a,b,c       IN 查询
```

排序字段也必须在 `OrderingFields` 里声明。`ordering=-year,name` 表示先按 `year` 倒序，再按 `name` 正序。用户没有传 `ordering` 时，`DefaultOrdering` 会生效；上面的例子默认按 `id` 倒序。

开启全局分页：

```yaml
rest:
  pagination: true
  default_page: 1
  default_page_size: 20
  max_page_size: 100
```

开启后列表接口会解析：

```txt
GET /books?page=1&page_size=20&search=go&ordering=-year,name&info=aa&year__gte=1990
```

`page` 默认由 `rest.default_page` 控制，默认值是 1；`page_size` 会受 `rest.default_page_size` 和 `rest.max_page_size` 控制。分页参数、`search` 和 `ordering` 是框架保留参数，不会进入业务过滤条件。业务过滤和分页同时存在时，先按业务条件缩小结果集，再对结果集分页。默认 GORM 仓库会应用等价的查询描述；自定义 Repository 也会收到同样的内部 `Query`：

```go
sgin.Query{
	Limit: 20,
	Offset: 0,
	Search: "go",
	OrderBy: "-year,name",
	Filters: map[string]string{"info": "aa", "year__gte": "1990"},
}
```

分页响应中的 `total` 表示当前查询条件下、分页前的总数。没有业务过滤条件时，它是当前可见数据总数；有 `info/year/search` 或权限查询收窄时，它是收窄后的总数。`page/page_size` 不影响 `total`。

如果全局开启了分页，但某个 ViewSet 不想分页，可以在路由注册处显式关闭：

```go
app.Register(&sgin.ModelViewSet[Book, uint]{
	BasePath:          "/books",
	DisablePagination: true,
})
```

## 测试

当前测试覆盖配置加载和校验、目录和表初始化、用户登录和 refresh token 轮换、认证/权限稳定错误码、管理员初始化、动态访问控制、Admin UI API、ViewSet CRUD、分页过滤、默认 GORM Repository、权限组合、对象权限和查询权限等关键路径。

```bash
go test ./...
```

查看覆盖率：

```bash
go test -cover ./...
```
