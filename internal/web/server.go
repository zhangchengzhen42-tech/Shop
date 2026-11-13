package web

import (
	"net/http"
	"net/url"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
)

type Server struct {
	store        *app.Store
	cookieSecret string
	templates    *templates
}

func New(store *app.Store, cookieSecret string) *Server {
	return &Server{store: store, cookieSecret: cookieSecret, templates: newTemplates()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.postOnly(s.logout))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", s.withUser(s.dashboard))
	mux.HandleFunc("/orders", s.withUser(s.postOnly(s.createOrder)))
	mux.HandleFunc("/orders/cancel", s.withUser(s.postOnly(s.cancelOrder)))
	mux.HandleFunc("/orders/ship", s.withUser(s.postOnly(s.shipOrder)))
	mux.HandleFunc("/refunds/request", s.withUser(s.postOnly(s.requestRefund)))
	mux.HandleFunc("/refunds/review", s.withUser(s.postOnly(s.reviewRefund)))
	mux.HandleFunc("/refunds/retry", s.withUser(s.postOnly(s.retryRefund)))
	mux.HandleFunc("/exceptions/refund", s.withUser(s.postOnly(s.startLateRefund)))
	mux.HandleFunc("/channel/payment", s.withUser(s.postOnly(s.channelPayment)))
	mux.HandleFunc("/channel/refund", s.withUser(s.postOnly(s.channelRefund)))
	mux.HandleFunc("/orders/expire", s.withUser(s.postOnly(s.expireOrders)))
	mux.HandleFunc("/products/update", s.withUser(s.postOnly(s.updateProduct)))
	return mux
}

func (s *Server) postOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func (s *Server) redirectMessage(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/?message="+urlQueryEscape(message), http.StatusSeeOther)
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}
