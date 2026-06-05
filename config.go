package sgin

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Config 是 sgin 的完整配置结构。
// YAML/JSON 标签保持一致，方便配置文件、接口输出和测试场景复用同一结构。
type Config struct {
	App      AppConfig      `yaml:"app" json:"app"`
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Redis    RedisConfig    `yaml:"redis" json:"redis"`
	Auth     AuthConfig     `yaml:"auth" json:"auth"`
	REST     RESTConfig     `yaml:"rest" json:"rest"`
	User     UserConfig     `yaml:"user" json:"user"`
	Admin    AdminConfig    `yaml:"admin" json:"admin"`
	JWT      JWTConfig      `yaml:"jwt" json:"jwt"`
}

// AppConfig 描述应用级元信息。
type AppConfig struct {
	Name  string `yaml:"name" json:"name"`
	Env   string `yaml:"env" json:"env"`
	Debug bool   `yaml:"debug" json:"debug"`
}

// ServerConfig 描述 HTTP 服务监听地址和 Gin 模式。
type ServerConfig struct {
	Addr string `yaml:"addr" json:"addr"`
	Mode string `yaml:"mode" json:"mode"`
}

// DatabaseConfig 描述数据库连接信息。
// 默认 ModelViewSet 和内置用户模块都会用 GORM 读取该配置；自定义 Repository 可自行决定存储实现。
type DatabaseConfig struct {
	Driver      string `yaml:"driver" json:"driver"`
	DSN         string `yaml:"dsn" json:"dsn"`
	AutoMigrate bool   `yaml:"auto_migrate" json:"auto_migrate"`
}

// RedisConfig 描述可选 Redis 连接信息。
// 框架当前不主动连接 Redis，只提供统一配置入口。
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Addr     string `yaml:"addr" json:"addr"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`
}

type AuthConfig struct {
	Required bool `yaml:"required" json:"required"`
}

// RESTConfig 描述 ViewSet/REST 层默认行为。
type RESTConfig struct {
	Pagination      bool   `yaml:"pagination" json:"pagination"`
	DefaultPage     int    `yaml:"default_page" json:"default_page"`
	DefaultPageSize int    `yaml:"default_page_size" json:"default_page_size"`
	MaxPageSize     int    `yaml:"max_page_size" json:"max_page_size"`
	StaticDir       string `yaml:"static_dir" json:"static_dir"`
}

// UserConfig 描述内置用户登录能力。
type UserConfig struct {
	Enabled bool            `yaml:"enabled" json:"enabled"`
	Path    string          `yaml:"path" json:"path"`
	Admin   UserAdminConfig `yaml:"admin" json:"admin"`
}

// UserAdminConfig 描述内置管理员账号初始化行为。
type UserAdminConfig struct {
	Init     bool   `yaml:"init" json:"init"`
	Username string `yaml:"username" json:"username"`
}

// AdminConfig 描述内置极简管理界面。
type AdminConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Path    string `yaml:"path" json:"path"`
}

// JWTConfig 描述登录 token 行为，时间单位为小时。
type JWTConfig struct {
	Secret         string `yaml:"secret" json:"secret"`
	Expired        int    `yaml:"expired" json:"expired"`
	RefreshExpired int    `yaml:"refresh_expired" json:"refresh_expired"`
}

// DefaultConfig 返回框架默认配置。
// 默认配置必须保证 sgin.New() 在没有 config.yaml 时也可以直接启动。
func DefaultConfig() Config {
	return Config{
		App: AppConfig{
			Name:  "sgin-app",
			Env:   "development",
			Debug: true,
		},
		Server: ServerConfig{
			Addr: ":8080",
			Mode: "debug",
		},
		Database: DatabaseConfig{
			Driver:      "sqlite",
			DSN:         "./app.db",
			AutoMigrate: false,
		},
		Redis: RedisConfig{
			Enabled: false,
			Addr:    "127.0.0.1:6379",
			DB:      0,
		},
		Auth: AuthConfig{
			Required: true,
		},
		REST: RESTConfig{
			Pagination:      false,
			DefaultPage:     1,
			DefaultPageSize: 20,
			MaxPageSize:     100,
			StaticDir:       "./uploads",
		},
		Admin: AdminConfig{
			Enabled: false,
			Path:    "/sgin-admin",
		},
		User: UserConfig{
			Enabled: true,
			Path:    "/login",
			Admin: UserAdminConfig{
				Init:     true,
				Username: "admin",
			},
		},
		JWT: JWTConfig{
			Secret:         randomConfigSecret(),
			Expired:        1,
			RefreshExpired: 168,
		},
	}
}

// LoadOptions 控制配置加载流程。
type LoadOptions struct {
	ConfigFile          string
	ExampleConfigFile   string
	AutoGenerateExample bool
	Strict              bool
	UseEnv              bool
}

// DefaultLoadOptions 返回配置加载默认行为。
func DefaultLoadOptions() LoadOptions {
	return LoadOptions{
		ConfigFile:          "config.yaml",
		ExampleConfigFile:   "config.example.yaml",
		AutoGenerateExample: true,
		Strict:              false,
		UseEnv:              true,
	}
}

// ValidateConfig 对配置进行轻量校验。
// 它不连接外部服务，只检查会导致框架无法正常初始化的基础字段。
func ValidateConfig(cfg Config) error {
	switch cfg.Server.Mode {
	case "", "debug", "release", "test":
	default:
		return fmt.Errorf("sgin: unsupported server.mode %q", cfg.Server.Mode)
	}

	switch cfg.Database.Driver {
	case "", "sqlite", "sqlite3", "mysql", "pg", "postgres", "postgresql":
	default:
		return fmt.Errorf("sgin: unsupported database.driver %q", cfg.Database.Driver)
	}

	if cfg.REST.DefaultPage <= 0 {
		return fmt.Errorf("sgin: rest.default_page must be greater than 0")
	}
	if cfg.REST.DefaultPageSize <= 0 {
		return fmt.Errorf("sgin: rest.default_page_size must be greater than 0")
	}
	if cfg.REST.MaxPageSize <= 0 {
		return fmt.Errorf("sgin: rest.max_page_size must be greater than 0")
	}
	if cfg.REST.DefaultPageSize > cfg.REST.MaxPageSize {
		return fmt.Errorf("sgin: rest.default_page_size cannot be greater than rest.max_page_size")
	}
	if cfg.User.Enabled && cfg.User.Path == "" {
		return fmt.Errorf("sgin: user.path cannot be empty when user is enabled")
	}
	if cfg.Admin.Enabled && !cfg.User.Enabled {
		return fmt.Errorf("sgin: admin UI requires user.enabled=true")
	}
	if cfg.Admin.Enabled && cfg.Admin.Path == "" {
		return fmt.Errorf("sgin: admin.path cannot be empty when admin UI is enabled")
	}
	if cfg.User.Enabled && cfg.User.Admin.Init && cfg.User.Admin.Username == "" {
		return fmt.Errorf("sgin: user.admin.username cannot be empty when admin init is enabled")
	}
	if cfg.User.Enabled && cfg.JWT.Secret == "" {
		return fmt.Errorf("sgin: jwt.secret cannot be empty when user is enabled")
	}
	if cfg.JWT.Expired <= 0 {
		return fmt.Errorf("sgin: jwt.expired must be greater than 0")
	}
	if cfg.JWT.RefreshExpired <= 0 {
		return fmt.Errorf("sgin: jwt.refresh_expired must be greater than 0")
	}
	return nil
}

func randomConfigSecret() string {
	secret, err := generateRandomSecret(32)
	if err != nil {
		return "sgin-development-secret"
	}
	return secret
}

func generateRandomSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
