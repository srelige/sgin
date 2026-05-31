package sgin

import (
	"os"
	"path/filepath"
	"testing"
)

type initBookTable struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (initBookTable) TableName() string {
	return "init_books"
}

type initAuthorTable struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (initAuthorTable) TableName() string {
	return "init_authors"
}

type initExistingTableV1 struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func (initExistingTableV1) TableName() string {
	return "init_existing"
}

type initExistingTableV2 struct {
	ID   uint `gorm:"primaryKey"`
	Name string
	Info string
}

func (initExistingTableV2) TableName() string {
	return "init_existing"
}

func TestInitDirCreatesRecommendedDirectories(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}

	if err := app.InitDir(); err != nil {
		t.Fatalf("InitDir: %v", err)
	}
	if err := app.InitDir(); err != nil {
		t.Fatalf("InitDir should ignore existing dirs: %v", err)
	}

	for _, name := range []string{"dao", "handlers", "middlewares", "models", "routers", "serializers", "services", "ui", "utils"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("expected %s directory: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be directory", name)
		}
	}
}

func TestInitTableCreatesOneOrMoreTables(t *testing.T) {
	app := newInitTableTestApp(t)

	if err := app.InitTable(&initBookTable{}, &initAuthorTable{}); err != nil {
		t.Fatalf("InitTable: %v", err)
	}
	if err := app.InitTable(&initBookTable{}); err != nil {
		t.Fatalf("InitTable should ignore existing table: %v", err)
	}

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	if !db.Migrator().HasTable(&initBookTable{}) {
		t.Fatalf("expected book table")
	}
	if !db.Migrator().HasTable(&initAuthorTable{}) {
		t.Fatalf("expected author table")
	}
}

func TestInitTableSkipsExistingTables(t *testing.T) {
	app := newInitTableTestApp(t)

	if err := app.InitTable(&initExistingTableV1{}); err != nil {
		t.Fatalf("InitTable v1: %v", err)
	}
	if err := app.InitTable(&initExistingTableV2{}); err != nil {
		t.Fatalf("InitTable v2 should skip existing table: %v", err)
	}

	db, err := app.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	if db.Migrator().HasColumn(&initExistingTableV2{}, "info") {
		t.Fatalf("expected existing table to be left unchanged")
	}
}

func TestInitTableRejectsNilModel(t *testing.T) {
	app := newInitTableTestApp(t)

	if err := app.InitTable(nil); err == nil {
		t.Fatalf("expected nil model error")
	}
}

func newInitTableTestApp(t *testing.T) *App {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Server.Mode = "test"
	cfg.User.Enabled = false
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "app.db")
	cfg.Database.AutoMigrate = false

	app, err := NewE(WithConfig(cfg))
	if err != nil {
		t.Fatalf("NewE: %v", err)
	}
	t.Cleanup(app.CloseDB)
	return app
}
