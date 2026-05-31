package sgin

import "github.com/gin-gonic/gin"

// And 返回一个组合权限：所有子权限都允许时才允许。
func And(perms ...Permission) Permission {
	return andPermission{perms: perms}
}

// Or 返回一个组合权限：任意子权限允许时即允许。
func Or(perms ...Permission) Permission {
	return orPermission{perms: perms}
}

// Not 返回一个取反权限。
func Not(perm Permission) Permission {
	return notPermission{perm: perm}
}

type andPermission struct {
	perms []Permission
}

func (p andPermission) HasPermission(c *gin.Context, action string) Decision {
	for _, perm := range p.perms {
		if perm == nil {
			continue
		}
		decision := perm.HasPermission(c, action)
		if !decision.Allowed {
			return decision
		}
	}
	return Allow()
}

type orPermission struct {
	perms []Permission
}

func (p orPermission) HasPermission(c *gin.Context, action string) Decision {
	if len(p.perms) == 0 {
		return Deny(ErrCodePermissionDenied, "permission denied")
	}
	var last Decision
	for _, perm := range p.perms {
		if perm == nil {
			continue
		}
		decision := perm.HasPermission(c, action)
		if decision.Allowed {
			return Allow()
		}
		last = decision
	}
	if last.Message != "" || last.Code != "" {
		return last
	}
	return Deny(ErrCodePermissionDenied, "permission denied")
}

type notPermission struct {
	perm Permission
}

func (p notPermission) HasPermission(c *gin.Context, action string) Decision {
	if p.perm == nil {
		return Allow()
	}
	decision := p.perm.HasPermission(c, action)
	if decision.Allowed {
		return Deny(ErrCodePermissionDenied, "permission negated")
	}
	return Allow()
}
