package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseCallbackFormNormalizesNonStandardQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := map[string]string{
		"semicolon separator":    "/api/v1/payments/callback?pid=2026;out_trade_no=ORDER-1;trade_status=TRADE_SUCCESS;sign=abc",
		"html escaped ampersand": "/api/v1/payments/callback?pid=2026&amp;out_trade_no=ORDER-1&amp;trade_status=TRADE_SUCCESS&amp;sign=abc",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, target, nil)

			form, err := parseCallbackForm(c)
			if err != nil {
				t.Fatalf("parse callback form failed: %v", err)
			}
			if got := getFirstValue(form, "out_trade_no"); got != "ORDER-1" {
				t.Fatalf("unexpected out_trade_no: %q", got)
			}
			if got := getFirstValue(form, "trade_status"); got != "TRADE_SUCCESS" {
				t.Fatalf("unexpected trade_status: %q", got)
			}
			if got := getFirstValue(form, "sign"); got != "abc" {
				t.Fatalf("unexpected sign: %q", got)
			}
		})
	}
}

func TestParseCallbackFormPreferPostForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := strings.NewReader("out_trade_no=ORDER-POST&trade_status=TRADE_SUCCESS&sign=abc&notify_id=n1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback?channel_id=999", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req

	form, err := parseCallbackForm(c)
	if err != nil {
		t.Fatalf("parse callback form failed: %v", err)
	}
	if got := getFirstValue(form, "out_trade_no"); got != "ORDER-POST" {
		t.Fatalf("unexpected out_trade_no: %s", got)
	}
	if got := getFirstValue(form, "channel_id"); got != "" {
		t.Fatalf("expected query param excluded from signed form, got %s", got)
	}
}
