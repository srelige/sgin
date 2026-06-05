package sgin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// App 是 sgin 的主应用对象。
// 它嵌入 *gin.Engine，因此 app.GET、app.POST、app.Use、app.Group 等 Gin 原生方法可以直接调用。
type App struct {
	*gin.Engine

	// config 保存应用启动时合并后的配置。
	// 对外通过 Config/SetConfig 访问，避免外部直接修改内部状态。
	config Config

	// permissions 保存框架内置和用户自定义的权限工厂。
	permissions *PermissionRegistry

	// userBootstrap 保存启动时内置用户系统初始化结果。
	userBootstrap UserBootstrapResult

	anonymousMu     sync.RWMutex
	anonymousRoutes map[string]struct{}
	defaultAuth     bool

	// db 是按 database 配置延迟打开的默认 GORM 连接，供默认 ModelViewSet 使用。
	dbMu sync.Mutex
	db   *gorm.DB
}

// Option 用于定制 New/NewE 的初始化行为。
type Option func(*options)

// options 是 NewE 内部使用的启动参数集合。
type options struct {
	config         *Config
	loadOptions    LoadOptions
	autoLoadConfig bool
}

// defaultOptions 返回框架默认启动选项。
func defaultOptions() options {
	return options{
		loadOptions:    DefaultLoadOptions(),
		autoLoadConfig: true,
	}
}

// New 创建 App；如果初始化失败会 panic。
// 这是面向业务代码的便捷入口，适合希望像 gin.Default() 一样快速启动的场景。
func New(opts ...Option) *App {
	app, err := NewE(opts...)
	if err != nil {
		panic(err)
	}
	return app
}

// NewE 创建 App 并返回初始化错误。
// 默认行为会尝试读取 ./config.yaml；如果不存在，会生成 ./config.example.yaml 并使用默认配置继续启动。
func NewE(opts ...Option) (*App, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	var cfg Config
	var err error
	if o.config != nil {
		// 显式传入的配置优先级最高，不再被配置文件或环境变量覆盖。
		cfg = *o.config
		if err = ValidateConfig(cfg); err != nil {
			return nil, err
		}
	} else if o.autoLoadConfig {
		cfg, err = LoadConfig(o.loadOptions)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = DefaultConfig()
	}

	if err := setGinMode(cfg.Server.Mode); err != nil {
		return nil, err
	}

	userBootstrap, err := bootstrapUserSystem(cfg)
	if err != nil {
		return nil, err
	}
	if userBootstrap.AdminCreated {
		log.Printf("sgin: admin account created username=%q password=%q", userBootstrap.AdminUsername, userBootstrap.AdminPassword)
	}

	app := &App{
		Engine:          gin.Default(),
		config:          cfg,
		permissions:     NewPermissionRegistry(),
		userBootstrap:   userBootstrap,
		anonymousRoutes: map[string]struct{}{},
	}
	registerUserRoutes(app)
	registerAdminRoutes(app)
	if cfg.Auth.Required {
		app.defaultAuth = true
		app.Use(app.defaultAuthMiddleware())
	}
	return app, nil
}

// WithConfig 使用代码显式传入配置。
// 该配置具有最高优先级，不会再读取配置文件或应用环境变量覆盖。
func WithConfig(cfg Config) Option {
	return func(o *options) {
		o.config = &cfg
		o.autoLoadConfig = false
	}
}

// WithConfigFile 指定配置文件路径，默认是 config.yaml。
func WithConfigFile(path string) Option {
	return func(o *options) {
		o.loadOptions.ConfigFile = path
	}
}

// WithExampleConfigFile 指定自动生成的示例配置文件路径，默认是 config.example.yaml。
func WithExampleConfigFile(path string) Option {
	return func(o *options) {
		o.loadOptions.ExampleConfigFile = path
	}
}

// WithAutoGenerateExample 控制缺少配置文件时是否自动生成 config.example.yaml。
func WithAutoGenerateExample(enabled bool) Option {
	return func(o *options) {
		o.loadOptions.AutoGenerateExample = enabled
	}
}

// WithStrictConfig 开启严格配置模式。
// 严格模式下找不到配置文件会返回 ErrConfigNotFound，YAML 中出现未知字段也会返回错误。
func WithStrictConfig(enabled bool) Option {
	return func(o *options) {
		o.loadOptions.Strict = enabled
	}
}

// WithEnv 控制是否允许 SGIN_* 环境变量覆盖配置文件。
func WithEnv(enabled bool) Option {
	return func(o *options) {
		o.loadOptions.UseEnv = enabled
	}
}

// Run 启动 HTTP 服务。
// 如果调用方传入地址，则使用传入地址；否则使用配置中的 server.addr；再为空时回退到 :8080。
func (a *App) Run(addr ...string) error {
	if len(addr) > 0 {
		return a.Engine.Run(addr...)
	}
	runAddr := a.config.Server.Addr
	if runAddr == "" {
		runAddr = ":8080"
	}
	return a.Engine.Run(runAddr)
}

// Config 返回当前配置副本。
// 返回副本可以避免调用方误改 App 内部配置；需要修改时应调用 SetConfig。
func (a *App) Config() Config {
	return a.config
}

func (a *App) AllowAnonymous(method string, path string) {
	if a == nil {
		return
	}
	a.markAnonymousRoute(method, path)
}

// SetConfig 替换 App 当前配置。
// 如果 server.mode 有效，会同步更新 Gin 的运行模式。
func (a *App) SetConfig(cfg Config) {
	a.config = cfg
	_ = setGinMode(cfg.Server.Mode)
}

// DB 返回按 database 配置打开的默认 GORM 连接。
// ModelViewSet 在未显式传入 Repository 时会使用该连接提供默认 CRUD。
func (a *App) DB() (*gorm.DB, error) {
	if a == nil {
		return nil, fmt.Errorf("sgin: app is nil")
	}
	a.dbMu.Lock()
	defer a.dbMu.Unlock()

	if a.db != nil {
		return a.db, nil
	}
	db, err := openGormDB(a.config)
	if err != nil {
		return nil, err
	}
	a.db = db
	return a.db, nil
}

// CloseDB 关闭 App 延迟打开的默认 GORM 连接。
func (a *App) CloseDB() {
	if a == nil {
		return
	}
	a.dbMu.Lock()
	defer a.dbMu.Unlock()

	closeGormDB(a.db)
	a.db = nil
}

// InitTable 按配置中的数据库初始化一个或多个模型表。
// 表已存在时跳过；表不存在时创建，不对已存在表执行迁移。
func (a *App) InitTable(models ...any) error {
	if len(models) == 0 {
		return nil
	}
	db, err := a.DB()
	if err != nil {
		return err
	}

	migrator := db.Migrator()
	for _, model := range models {
		if isNilModel(model) {
			return fmt.Errorf("sgin: init table model cannot be nil")
		}
		if migrator.HasTable(model) {
			continue
		}
		if err := db.AutoMigrate(model); err != nil {
			return err
		}
	}
	return nil
}

func isNilModel(model any) bool {
	if model == nil {
		return true
	}
	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Register 注册一个 ViewSet。
// App 会在注册前把自身注入给支持注入的 ViewSet，用于注入 App 能力。
func (a *App) Register(vs ViewSet) {
	if aware, ok := vs.(appAwareViewSet); ok {
		aware.setApp(a)
	}
	vs.Register(a.Engine)
}

// PermissionRegistry 返回权限注册表，便于业务代码注册自定义权限名称。
func (a *App) PermissionRegistry() *PermissionRegistry {
	return a.permissions
}

// InitDir 初始化推荐业务目录结构；目录已存在时会忽略。
func (a *App) InitDir() error {
	for _, dir := range []string{"dao", "handlers", "middlewares", "models", "routers", "serializers", "services", "ui", "utils"} {
		if err := os.MkdirAll(filepath.Clean(dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// UserBootstrapResult 返回启动时用户系统初始化结果。
func (a *App) UserBootstrapResult() UserBootstrapResult {
	return a.userBootstrap
}

// AdminBootstrapResult 返回启动时管理员账号初始化结果。
func (a *App) AdminBootstrapResult() AdminBootstrapResult {
	return AdminBootstrapResult{
		Enabled:  a.userBootstrap.Enabled,
		Created:  a.userBootstrap.AdminCreated,
		Username: a.userBootstrap.AdminUsername,
		Password: a.userBootstrap.AdminPassword,
	}
}

// setGinMode 安全设置 Gin 运行模式。
func setGinMode(mode string) error {
	if mode == "" {
		return nil
	}
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		gin.SetMode(mode)
		return nil
	default:
		return fmt.Errorf("sgin: unsupported gin mode %q", mode)
	}
}

// appAwareViewSet 是框架内部接口，用来把 App 注入到 ViewSet。
type appAwareViewSet interface {
	setApp(*App)
}

func (a *App) defaultAuthMiddleware() gin.HandlerFunc {
	auth := a.JWTAuth()
	return func(c *gin.Context) {
		if !a.config.Auth.Required || a.isAnonymousRoute(c) {
			c.Next()
			return
		}
		auth(c)
	}
}

func (a *App) usesDefaultAuthMiddleware() bool {
	return a != nil && a.defaultAuth && a.config.Auth.Required
}

func (a *App) markAnonymousRoute(method string, path string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeBasePath(path)
	if method == "" || path == "" {
		return
	}
	a.anonymousMu.Lock()
	defer a.anonymousMu.Unlock()
	if a.anonymousRoutes == nil {
		a.anonymousRoutes = map[string]struct{}{}
	}
	a.anonymousRoutes[routeAuthKey(method, path)] = struct{}{}
}

func (a *App) isAnonymousRoute(c *gin.Context) bool {
	if a == nil || c == nil {
		return false
	}
	method := strings.ToUpper(c.Request.Method)
	paths := []string{c.FullPath(), c.Request.URL.Path}
	a.anonymousMu.RLock()
	defer a.anonymousMu.RUnlock()
	for _, path := range paths {
		path = normalizeBasePath(path)
		if _, ok := a.anonymousRoutes[routeAuthKey(method, path)]; ok {
			return true
		}
	}
	return false
}

func routeAuthKey(method string, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizeBasePath(path)
}
