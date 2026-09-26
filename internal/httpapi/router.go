package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isa-service/internal/isa"
)

// NewRouter wires the ISA HTTP surface. The service has exactly two
// calculation endpoints plus a built-in demo; there is no UI and no storage.
//
//	GET /api/isa/point?altitude=<metres>[&delta_t=<kelvin>]
//	GET /api/isa/profile?start=<m>&end=<m>&step=<m>[&delta_t=<K>]
//	GET /api/isa/demo
//	GET /healthz
func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "model_ceiling_m": isa.ModelCeiling})
	})

	api := r.Group("/api/isa")
	{
		api.GET("/point", handlePoint)
		api.GET("/profile", handleProfile)
		api.GET("/demo", handleDemo)
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, errorBody{Error: "not_found", Reason: "unknown route; see /api/isa/point and /api/isa/profile"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, errorBody{Error: "method_not_allowed", Reason: "only GET is supported"})
	})
	r.HandleMethodNotAllowed = true

	return r
}
