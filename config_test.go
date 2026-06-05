package sgin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigGeneratesExampleAndUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	examplePath := filepath.Join(dir, "config.example.yaml")

	cfg, err := LoadConfig(LoadOptions{
		ConfigFile:          configPath,
		ExampleConfigFile:   examplePath,
		AutoGenerateExample: true,
		UseEnv:              false,
	})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.App.Name != "sgin-app" {
		t.Fatalf("expected default app name, got %q", cfg.App.Name)
	}
	if cfg.JWT.Secret == "" {
		t.Fatal("expected generated jwt secret")
	}
	if !cfg.Auth.Required {
		t.Fatal("expected auth.required to default to true")
	}
	if _, err := os.Stat(examplePath); err != nil {
		t.Fatalf("expected generated example config: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config.yaml should not be generated, stat err=%v", err)
	}
}

func TestLoadConfigUsesExistingExampleWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	examplePath := filepath.Join(dir, "config.example.yaml")
	if err := os.WriteFile(examplePath, []byte(`
app:
  name: from-example
server:
  mode: test
jwt:
  secret: stable-secret
  expired: 2
  refresh_expired: 24
user:
  enabled: false
`), 0o644); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	cfg, err := LoadConfig(LoadOptions{
		ConfigFile:        configPath,
		ExampleConfigFile: examplePath,
		UseEnv:            false,
	})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.App.Name != "from-example" {
		t.Fatalf("expected example app name, got %q", cfg.App.Name)
	}
	if cfg.JWT.Secret != "stable-secret" {
		t.Fatalf("expected stable jwt secret, got %q", cfg.JWT.Secret)
	}
}

func TestLoadConfigUsesEnvWithoutGeneratingExample(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	examplePath := filepath.Join(dir, "config.example.yaml")
	t.Setenv("SGIN_APP_NAME", "from-env-only")
	t.Setenv("SGIN_AUTH_REQUIRED", "false")

	cfg, err := LoadConfig(LoadOptions{
		ConfigFile:          configPath,
		ExampleConfigFile:   examplePath,
		AutoGenerateExample: true,
		UseEnv:              true,
	})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.App.Name != "from-env-only" {
		t.Fatalf("expected env app name, got %q", cfg.App.Name)
	}
	if cfg.Auth.Required {
		t.Fatal("expected env auth override")
	}
	if _, err := os.Stat(examplePath); !os.IsNotExist(err) {
		t.Fatalf("config.example.yaml should not be generated when env is present, stat err=%v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config.yaml should not be generated, stat err=%v", err)
	}
}

func TestLoadConfigAppliesEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
app:
  name: from-file
server:
  addr: ":9000"
  mode: test
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("SGIN_APP_NAME", "from-env")
	t.Setenv("SGIN_SERVER_ADDR", ":9999")
	t.Setenv("SGIN_REST_PAGINATION", "true")
	t.Setenv("SGIN_REST_DEFAULT_PAGE", "2")
	t.Setenv("SGIN_REST_DEFAULT_PAGE_SIZE", "30")
	t.Setenv("SGIN_REST_MAX_PAGE_SIZE", "60")
	t.Setenv("SGIN_REST_STATIC_DIR", "/tmp/sgin-uploads")
	t.Setenv("SGIN_AUTH_REQUIRED", "false")

	cfg, err := LoadConfig(LoadOptions{
		ConfigFile: configPath,
		UseEnv:     true,
	})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.App.Name != "from-env" {
		t.Fatalf("expected env app name, got %q", cfg.App.Name)
	}
	if cfg.Server.Addr != ":9999" {
		t.Fatalf("expected env server addr, got %q", cfg.Server.Addr)
	}
	if !cfg.REST.Pagination || cfg.REST.DefaultPage != 2 || cfg.REST.DefaultPageSize != 30 || cfg.REST.MaxPageSize != 60 || cfg.REST.StaticDir != "/tmp/sgin-uploads" {
		t.Fatalf("expected rest env overrides, got %+v", cfg.REST)
	}
	if cfg.Auth.Required {
		t.Fatal("expected auth.required env override")
	}
}

func TestWithConfigHasHighestPriority(t *testing.T) {
	t.Setenv("SGIN_APP_NAME", "from-env")
	cfg := DefaultConfig()
	cfg.App.Name = "from-code"
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE returned error: %v", err)
	}
	if app.Config().App.Name != "from-code" {
		t.Fatalf("expected code config to win, got %q", app.Config().App.Name)
	}
}

func TestLoadConfigAppliesAdminEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
server:
  mode: test
jwt:
  secret: admin-env-secret
user:
  enabled: true
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("SGIN_ADMIN_ENABLED", "true")
	t.Setenv("SGIN_ADMIN_PATH", "/panel")

	cfg, err := LoadConfig(LoadOptions{ConfigFile: configPath, UseEnv: true})
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.Admin.Enabled {
		t.Fatal("expected admin UI to be enabled from env")
	}
	if cfg.Admin.Path != "/panel" {
		t.Fatalf("expected admin path from env, got %q", cfg.Admin.Path)
	}
}

func TestLoadConfigStrictMode(t *testing.T) {
	dir := t.TempDir()
	missingConfig := filepath.Join(dir, "missing.yaml")

	_, err := LoadConfig(LoadOptions{
		ConfigFile: missingConfig,
		Strict:     true,
		UseEnv:     false,
	})
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected strict missing config error, got %v", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
server:
  mode: test
unknown_section:
  enabled: true
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadConfig(LoadOptions{ConfigFile: configPath, Strict: true, UseEnv: false}); err == nil {
		t.Fatal("expected strict unknown field error")
	}
}

func TestValidateConfigRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "server mode",
			mutate: func(cfg *Config) {
				cfg.Server.Mode = "invalid"
			},
		},
		{
			name: "database driver",
			mutate: func(cfg *Config) {
				cfg.Database.Driver = "oracle"
			},
		},
		{
			name: "default page",
			mutate: func(cfg *Config) {
				cfg.REST.DefaultPage = 0
			},
		},
		{
			name: "page size",
			mutate: func(cfg *Config) {
				cfg.REST.DefaultPageSize = 0
			},
		},
		{
			name: "max page size",
			mutate: func(cfg *Config) {
				cfg.REST.MaxPageSize = 0
			},
		},
		{
			name: "page size over max",
			mutate: func(cfg *Config) {
				cfg.REST.DefaultPageSize = 101
				cfg.REST.MaxPageSize = 100
			},
		},
		{
			name: "user path",
			mutate: func(cfg *Config) {
				cfg.User.Enabled = true
				cfg.User.Path = ""
			},
		},
		{
			name: "admin without user",
			mutate: func(cfg *Config) {
				cfg.Admin.Enabled = true
				cfg.User.Enabled = false
			},
		},
		{
			name: "admin path",
			mutate: func(cfg *Config) {
				cfg.Admin.Enabled = true
				cfg.Admin.Path = ""
			},
		},
		{
			name: "admin username",
			mutate: func(cfg *Config) {
				cfg.User.Enabled = true
				cfg.User.Admin.Init = true
				cfg.User.Admin.Username = ""
			},
		},
		{
			name: "jwt secret",
			mutate: func(cfg *Config) {
				cfg.User.Enabled = true
				cfg.JWT.Secret = ""
			},
		},
		{
			name: "jwt expired",
			mutate: func(cfg *Config) {
				cfg.JWT.Expired = 0
			},
		},
		{
			name: "jwt refresh expired",
			mutate: func(cfg *Config) {
				cfg.JWT.RefreshExpired = 0
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mutate(&cfg)
			if err := ValidateConfig(cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
