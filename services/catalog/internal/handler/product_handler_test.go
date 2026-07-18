// Test Budget: 1 distinct behavior × 2 = 2 max unit tests
// Actual: 1
//
// Behavior 1: RegisterRoutes wires the media routes without a gin
//
//	static-vs-wildcard conflict (reorder vs :imageId share a segment)
package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProductHandler_RegisterRoutes_MediaRoutesDoNotConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewProductHandler(nil)

	// gin panics at registration on a static/wildcard conflict; a clean return
	// proves reorder, :imageId (PUT/DELETE), and :sign coexist.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("route registration panicked: %v", r)
		}
	}()
	h.RegisterRoutes(gin.New().Group("/v1"))
}
