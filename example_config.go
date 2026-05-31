package sgin

import "fmt"

// exampleConfigYAML 返回自动生成的 config.example.yaml 内容。
func exampleConfigYAML() (string, error) {
	secret, err := generateRandomSecret(32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`app:
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
  secret: "%s"
  expired: 1
  refresh_expired: 168
`, secret), nil
}
