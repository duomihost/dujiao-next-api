package router

import (
	"context"
	"net/http"

	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
)

type ResellerTenantResolver interface {
	ResolveRequest(ctx context.Context, req *http.Request) (service.TenantContext, error)
}

func ResellerTenantMiddleware(resolver ResellerTenantResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if resolver == nil {
			c.Next()
			return
		}
		tenant, err := resolver.ResolveRequest(c.Request.Context(), c.Request)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "internal_error", "message": "failed to resolve tenant"})
			return
		}
		if tenant.Unavailable {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "site unavailable"})
			return
		}
		ctx := service.WithTenantContext(c.Request.Context(), tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant", tenant)
		c.Next()
	}
}
