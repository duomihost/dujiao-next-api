package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildAuthzPolicyAuditRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("admin_id", uint(7))
	c.Set("username", " admin ")
	c.Set("request_id", " req-1 ")

	req := authzPolicyPayload{
		Role:   "operator",
		Object: "orders",
		Action: " get ",
	}

	got := buildAuthzPolicyAuditRecord(c, req, "policy_grant")

	if got.OperatorAdminID != 7 {
		t.Fatalf("OperatorAdminID = %d, want 7", got.OperatorAdminID)
	}
	if got.OperatorUsername != "admin" {
		t.Fatalf("OperatorUsername = %q, want %q", got.OperatorUsername, "admin")
	}
	if got.Action != "policy_grant" {
		t.Fatalf("Action = %q, want %q", got.Action, "policy_grant")
	}
	if got.Role != req.Role {
		t.Fatalf("Role = %q, want %q", got.Role, req.Role)
	}
	if got.Object != req.Object {
		t.Fatalf("Object = %q, want %q", got.Object, req.Object)
	}
	if got.Method != req.Action {
		t.Fatalf("Method = %q, want %q", got.Method, req.Action)
	}
	if got.RequestID != "req-1" {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, "req-1")
	}
	if got.Detail["role"] != req.Role {
		t.Fatalf("Detail role = %v, want %q", got.Detail["role"], req.Role)
	}
	if got.Detail["object"] != req.Object {
		t.Fatalf("Detail object = %v, want %q", got.Detail["object"], req.Object)
	}
	if got.Detail["method"] != "GET" {
		t.Fatalf("Detail method = %v, want %q", got.Detail["method"], "GET")
	}
}
