package sgin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// LoadConfig 按优先级加载配置。
// 优先级为：默认配置 < config.example.yaml < config.yaml < 环境变量；代码显式配置由 NewE 在更外层处理，优先级最高。
func LoadConfig(opts LoadOptions) (Config, error) {
	if opts == (LoadOptions{}) {
		opts = DefaultLoadOptions()
	}
	if opts.ConfigFile == "" {
		opts.ConfigFile = DefaultLoadOptions().ConfigFile
	}
	if opts.ExampleConfigFile == "" {
		opts.ExampleConfigFile = DefaultLoadOptions().ExampleConfigFile
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(opts.ConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if opts.Strict {
				return cfg, fmt.Errorf("%w: %s", ErrConfigNotFound, opts.ConfigFile)
			}
			data, err = os.ReadFile(opts.ExampleConfigFile)
			if err != nil {
				envOnly := opts.UseEnv && hasEnvOverrides()
				if errors.Is(err, os.ErrNotExist) && opts.AutoGenerateExample && !envOnly {
					if writeErr := WriteExampleConfig(opts.ExampleConfigFile); writeErr != nil {
						return cfg, writeErr
					}
					data, err = os.ReadFile(opts.ExampleConfigFile)
				}
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return cfg, err
				}
			}
			if len(bytes.TrimSpace(data)) > 0 {
				if err := decodeConfig(data, &cfg, opts.Strict); err != nil {
					return cfg, err
				}
			}
		} else {
			return cfg, err
		}
	} else if len(bytes.TrimSpace(data)) > 0 {
		if err := decodeConfig(data, &cfg, opts.Strict); err != nil {
			return cfg, err
		}
	}

	if opts.UseEnv {
		ApplyEnvOverrides(&cfg)
	}
	if err := ValidateConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func hasEnvOverrides() bool {
	for _, key := range sginEnvKeys {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

func decodeConfig(data []byte, cfg *Config, strict bool) error {
	if strict {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		return decoder.Decode(cfg)
	}
	return yaml.Unmarshal(data, cfg)
}

// WriteExampleConfig 写入示例配置文件。
// LoadConfig 只会在文件不存在时调用它；直接调用该函数会覆盖目标文件。
func WriteExampleConfig(path string) error {
	if path == "" {
		return fmt.Errorf("sgin: example config path cannot be empty")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := exampleConfigYAML()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

// ApplyEnvOverrides 使用 SGIN_* 环境变量覆盖配置。
// 无法解析的布尔值或整数会被忽略，避免环境变量拼写错误直接导致进程启动失败。
func ApplyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	setStringEnv("SGIN_APP_NAME", &cfg.App.Name)
	setStringEnv("SGIN_APP_ENV", &cfg.App.Env)
	setBoolEnv("SGIN_APP_DEBUG", &cfg.App.Debug)
	setStringEnv("SGIN_SERVER_ADDR", &cfg.Server.Addr)
	setStringEnv("SGIN_SERVER_MODE", &cfg.Server.Mode)
	setStringEnv("SGIN_DATABASE_DRIVER", &cfg.Database.Driver)
	setStringEnv("SGIN_DATABASE_DSN", &cfg.Database.DSN)
	setBoolEnv("SGIN_REDIS_ENABLED", &cfg.Redis.Enabled)
	setStringEnv("SGIN_REDIS_ADDR", &cfg.Redis.Addr)
	setBoolEnv("SGIN_ADMIN_ENABLED", &cfg.Admin.Enabled)
	setStringEnv("SGIN_ADMIN_PATH", &cfg.Admin.Path)
	setBoolEnv("SGIN_AUTH_REQUIRED", &cfg.Auth.Required)
	setBoolEnv("SGIN_REST_PAGINATION", &cfg.REST.Pagination)
	setIntEnv("SGIN_REST_DEFAULT_PAGE", &cfg.REST.DefaultPage)
	setIntEnv("SGIN_REST_DEFAULT_PAGE_SIZE", &cfg.REST.DefaultPageSize)
	setIntEnv("SGIN_REST_MAX_PAGE_SIZE", &cfg.REST.MaxPageSize)
	setStringEnv("SGIN_REST_STATIC_DIR", &cfg.REST.StaticDir)
	setBoolEnv("SGIN_USER_ENABLED", &cfg.User.Enabled)
	setStringEnv("SGIN_USER_PATH", &cfg.User.Path)
	setBoolEnv("SGIN_USER_ADMIN_INIT", &cfg.User.Admin.Init)
	setStringEnv("SGIN_USER_ADMIN_USERNAME", &cfg.User.Admin.Username)
	setStringEnv("SGIN_JWT_SECRET", &cfg.JWT.Secret)
	setIntEnv("SGIN_JWT_EXPIRED", &cfg.JWT.Expired)
	setIntEnv("SGIN_JWT_REFRESH_EXPIRED", &cfg.JWT.RefreshExpired)
}

var sginEnvKeys = []string{
	"SGIN_APP_NAME",
	"SGIN_APP_ENV",
	"SGIN_APP_DEBUG",
	"SGIN_SERVER_ADDR",
	"SGIN_SERVER_MODE",
	"SGIN_DATABASE_DRIVER",
	"SGIN_DATABASE_DSN",
	"SGIN_REDIS_ENABLED",
	"SGIN_REDIS_ADDR",
	"SGIN_ADMIN_ENABLED",
	"SGIN_ADMIN_PATH",
	"SGIN_AUTH_REQUIRED",
	"SGIN_REST_PAGINATION",
	"SGIN_REST_DEFAULT_PAGE",
	"SGIN_REST_DEFAULT_PAGE_SIZE",
	"SGIN_REST_MAX_PAGE_SIZE",
	"SGIN_REST_STATIC_DIR",
	"SGIN_USER_ENABLED",
	"SGIN_USER_PATH",
	"SGIN_USER_ADMIN_INIT",
	"SGIN_USER_ADMIN_USERNAME",
	"SGIN_JWT_SECRET",
	"SGIN_JWT_EXPIRED",
	"SGIN_JWT_REFRESH_EXPIRED",
}

// setStringEnv 在环境变量存在时写入字符串配置。
func setStringEnv(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = value
	}
}

// setBoolEnv 在环境变量存在且可解析时写入布尔配置。
func setBoolEnv(key string, target *bool) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(value); err == nil {
			*target = parsed
		}
	}
}

// setIntEnv 在环境变量存在且可解析时写入整数配置。
func setIntEnv(key string, target *int) {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}
