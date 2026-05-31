package sgin

// PermissionFactory 根据名称创建权限实例。
type PermissionFactory func() Permission

// PermissionRegistry 保存权限名称到工厂函数的映射。
type PermissionRegistry struct {
	items map[string]PermissionFactory
}

// NewPermissionRegistry 创建权限注册表并注册内置权限。
func NewPermissionRegistry() *PermissionRegistry {
	r := &PermissionRegistry{items: map[string]PermissionFactory{}}
	r.Register("allow_any", func() Permission { return AllowAny{} })
	r.Register("authenticated", func() Permission { return IsAuthenticated{} })
	r.Register("admin", func() Permission { return IsAdmin{} })
	r.Register("read_only", func() Permission { return ReadOnly{} })
	return r
}

// Register 注册权限工厂；同名注册会覆盖旧值。
func (r *PermissionRegistry) Register(name string, factory PermissionFactory) {
	if r == nil || name == "" || factory == nil {
		return
	}
	r.items[name] = factory
}

// Make 根据名称创建权限实例。
func (r *PermissionRegistry) Make(name string) (Permission, bool) {
	if r == nil {
		return nil, false
	}
	factory, ok := r.items[name]
	if !ok {
		return nil, false
	}
	return factory(), true
}
