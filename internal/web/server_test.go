package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
)

func testServer(t *testing.T) (*Server, *app.Store) {
	t.Helper()
	store, err := app.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store, "test-secret"), store
}

func loginCookie(t *testing.T, server *Server, userID string) *http.Cookie {
	t.Helper()
	return &http.Cookie{Name: "shop_session", Value: server.signedUser(userID), Path: "/"}
}

func TestLoginAndDashboardRoleRendering(t *testing.T) {
	server, _ := testServer(t)
	login := httptest.NewRecorder()
	server.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), "Shop 交易系统") {
		t.Fatalf("login page = %d, body=%q", login.Code, login.Body.String())
	}
	form := url.Values{"user_id": {"ops-1"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, req)
	if result.Code != http.StatusSeeOther || result.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login result = %d, headers=%v", result.Code, result.Header())
	}
	dashboard := httptest.NewRecorder()
	dashboardReq := httptest.NewRequest(http.MethodGet, "/", nil)
	dashboardReq.AddCookie(loginCookie(t, server, "ops-1"))
	server.Handler().ServeHTTP(dashboard, dashboardReq)
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), "商品维护") {
		t.Fatalf("operations dashboard = %d, body has maintenance=%t", dashboard.Code, strings.Contains(dashboard.Body.String(), "商品维护"))
	}
}

func TestAuthenticationAndPostOnlyGuards(t *testing.T) {
	server, _ := testServer(t)
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusFound || unauthorized.Header().Get("Location") != "/login" {
		t.Fatalf("unauthorized = %d, location=%q", unauthorized.Code, unauthorized.Header().Get("Location"))
	}
	tampered := httptest.NewRequest(http.MethodGet, "/", nil)
	tampered.AddCookie(&http.Cookie{Name: "shop_session", Value: "YnV5ZXItMS.invalid"})
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, tampered)
	if result.Code != http.StatusFound {
		t.Fatalf("tampered session = %d", result.Code)
	}
	getPostOnly := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodGet, "/orders", nil)
	postReq.AddCookie(loginCookie(t, server, "buyer-1"))
	server.Handler().ServeHTTP(getPostOnly, postReq)
	if getPostOnly.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET mutation endpoint = %d", getPostOnly.Code)
	}
	logout := httptest.NewRecorder()
	server.Handler().ServeHTTP(logout, httptest.NewRequest(http.MethodGet, "/logout", nil))
	if logout.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET logout = %d", logout.Code)
	}
}

func TestRoleCannotMutateAnotherRole(t *testing.T) {
	server, store := testServer(t)
	if _, err := store.User(context.Background(), "buyer-1"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/products/update", strings.NewReader(url.Values{
		"product_id":  {"p1"},
		"price_cents": {"12000"},
		"available":   {"4"},
		"on_sale":     {"1"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(loginCookie(t, server, "buyer-1"))
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, req)
	message := result.Header().Get("Location")
	if parsed, err := url.Parse(message); err == nil {
		message = parsed.Query().Get("message")
	}
	if result.Code != http.StatusSeeOther || message != "无权执行此操作" {
		t.Fatalf("buyer product mutation = %d, location=%q", result.Code, result.Header().Get("Location"))
	}
}

func postForm(t *testing.T, server *Server, userID, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(loginCookie(t, server, userID))
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, req)
	if result.Code != http.StatusSeeOther {
		t.Fatalf("POST %s status = %d, body=%s", path, result.Code, result.Body.String())
	}
	return result
}

func buyerOrders(t *testing.T, store *app.Store) []app.Order {
	t.Helper()
	buyer, err := store.User(context.Background(), "buyer-1")
	if err != nil {
		t.Fatal(err)
	}
	orders, err := store.Orders(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	return orders
}

func locationMessage(t *testing.T, result *httptest.ResponseRecorder) string {
	t.Helper()
	location, err := url.Parse(result.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location.Query().Get("message")
}

func TestTransactionHandlersEndToEnd(t *testing.T) {
	server, store := testServer(t)
	create := postForm(t, server, "buyer-1", "/orders", url.Values{"product_id": {"p1"}, "quantity": {"1"}})
	if locationMessage(t, create) != "订单已创建，库存已占用" {
		t.Fatalf("create message=%q", create.Header().Get("Location"))
	}
	order := buyerOrders(t, store)[0]
	postForm(t, server, "channel-1", "/channel/payment", url.Values{
		"order_id": {order.ID}, "channel_tx": {"WEB-PAY-1"}, "amount_cents": {"10000"}, "signature": {"valid"},
	})
	postForm(t, server, "warehouse-1", "/orders/ship", url.Values{
		"order_id": {order.ID}, "carrier": {"顺丰"}, "tracking": {"SF-WEB-1"},
	})
	buyerDashboard := httptest.NewRecorder()
	buyerRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	buyerRequest.AddCookie(loginCookie(t, server, "buyer-1"))
	server.Handler().ServeHTTP(buyerDashboard, buyerRequest)
	if buyerDashboard.Code != http.StatusOK || !strings.Contains(buyerDashboard.Body.String(), "SF-WEB-1") {
		t.Fatalf("buyer tracking dashboard=%d, contains tracking=%t", buyerDashboard.Code, strings.Contains(buyerDashboard.Body.String(), "SF-WEB-1"))
	}

	postForm(t, server, "buyer-1", "/orders", url.Values{"product_id": {"p1"}, "quantity": {"1"}})
	refundable := buyerOrders(t, store)[0]
	postForm(t, server, "channel-1", "/channel/payment", url.Values{
		"order_id": {refundable.ID}, "channel_tx": {"WEB-PAY-2"}, "amount_cents": {"10000"}, "signature": {"valid"},
	})
	postForm(t, server, "buyer-1", "/refunds/request", url.Values{"order_id": {refundable.ID}})
	refunds, err := store.Refunds(context.Background())
	if err != nil || len(refunds) != 1 {
		t.Fatalf("refunds=%#v err=%v", refunds, err)
	}
	postForm(t, server, "service-1", "/refunds/review", url.Values{"refund_id": {refunds[0].ID}, "reason": {"售前退款"}, "decision": {"approve"}})
	postForm(t, server, "channel-1", "/channel/refund", url.Values{"refund_id": {refunds[0].ID}, "result": {"failed"}})
	blocked := postForm(t, server, "channel-1", "/channel/refund", url.Values{"refund_id": {refunds[0].ID}, "result": {"success"}})
	if locationMessage(t, blocked) != app.ErrInvalidState.Error() {
		t.Fatalf("channel retried failed refund without service action: %q", locationMessage(t, blocked))
	}
	postForm(t, server, "service-1", "/refunds/retry", url.Values{"refund_id": {refunds[0].ID}, "reason": {"渠道恢复"}})
	postForm(t, server, "channel-1", "/channel/refund", url.Values{"refund_id": {refunds[0].ID}, "result": {"success"}})

	postForm(t, server, "buyer-1", "/orders", url.Values{"product_id": {"p1"}, "quantity": {"1"}})
	late := buyerOrders(t, store)[0]
	postForm(t, server, "buyer-1", "/orders/cancel", url.Values{"order_id": {late.ID}})
	postForm(t, server, "channel-1", "/channel/payment", url.Values{
		"order_id": {late.ID}, "channel_tx": {"WEB-LATE"}, "amount_cents": {"10000"}, "signature": {"valid"},
	})
	exceptions, err := store.Exceptions(context.Background())
	if err != nil || len(exceptions) != 1 {
		t.Fatalf("exceptions=%#v err=%v", exceptions, err)
	}
	postForm(t, server, "service-1", "/exceptions/refund", url.Values{"exception_id": {exceptions[0].ID}})

	postForm(t, server, "ops-1", "/products/update", url.Values{
		"product_id": {"p2"}, "price_cents": {"9000"}, "available": {"18"}, "on_sale": {"0"},
	})
	postForm(t, server, "ops-1", "/orders/expire", url.Values{})
	opsDashboard := httptest.NewRecorder()
	opsRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	opsRequest.AddCookie(loginCookie(t, server, "ops-1"))
	server.Handler().ServeHTTP(opsDashboard, opsRequest)
	for _, text := range []string{"交易与履约记录", "收款记录", "退款记录", "发货记录", "WEB-PAY-1"} {
		if !strings.Contains(opsDashboard.Body.String(), text) {
			t.Fatalf("operations dashboard missing %q", text)
		}
	}
}

func TestHandlerValidationAndHelpers(t *testing.T) {
	server, _ := testServer(t)
	invalidLogin := httptest.NewRecorder()
	invalidLoginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("user_id=missing"))
	invalidLoginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	server.Handler().ServeHTTP(invalidLogin, invalidLoginReq)
	if invalidLogin.Code != http.StatusOK || !strings.Contains(invalidLogin.Body.String(), "请选择有效演示账号") {
		t.Fatalf("invalid login=%d", invalidLogin.Code)
	}
	invalidPayment := postForm(t, server, "channel-1", "/channel/payment", url.Values{"amount_cents": {"not-a-number"}})
	if !strings.Contains(locationMessage(t, invalidPayment), "支付金额") {
		t.Fatalf("invalid payment message=%q", invalidPayment.Header().Get("Location"))
	}
	invalidProduct := postForm(t, server, "ops-1", "/products/update", url.Values{"price_cents": {"bad"}, "available": {"bad"}})
	if !strings.Contains(locationMessage(t, invalidProduct), "价格") {
		t.Fatalf("invalid product message=%q", invalidProduct.Header().Get("Location"))
	}
	if !requiresRole(app.User{Role: app.RoleBuyer}, app.RoleBuyer) || requiresRole(app.User{Role: app.RoleBuyer}, app.RoleOperations) {
		t.Fatal("requiresRole returned an unexpected result")
	}
	if statusText("PENDING") != "待付款" || statusText("UNKNOWN") != "UNKNOWN" {
		t.Fatal("status text mapping failed")
	}
	if messageFor(app.ErrDuplicate, "ok") != "重复通知已幂等处理" || messageFor(app.ErrForbidden, "ok") != app.ErrForbidden.Error() {
		t.Fatal("message mapping failed")
	}
	logout := postForm(t, server, "buyer-1", "/logout", url.Values{})
	if logout.Header().Get("Set-Cookie") == "" {
		t.Fatal("logout should expire the session cookie")
	}
}
