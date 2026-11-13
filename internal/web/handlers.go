package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.templates.login(w, r.URL.Query().Get("message"))
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	user, err := s.store.User(r.Context(), r.FormValue("user_id"))
	if err != nil {
		s.templates.login(w, "请选择有效演示账号")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "shop_session", Value: s.signedUser(user.ID), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "shop_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	data, err := s.store.Dashboard(r.Context(), user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary, err := s.store.Summary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.templates.dashboard(w, pageData{User: user, Dashboard: data, Summary: summary, Message: r.URL.Query().Get("message")})
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	quantity, _ := strconv.Atoi(r.FormValue("quantity"))
	_, err := s.store.CreateOrder(r.Context(), currentUser(r), r.FormValue("product_id"), quantity)
	s.redirectMessage(w, r, messageFor(err, "订单已创建，库存已占用"))
}

func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	err := s.store.CancelOrder(r.Context(), currentUser(r), r.FormValue("order_id"), "买家取消")
	s.redirectMessage(w, r, messageFor(err, "订单已取消，库存已释放"))
}

func (s *Server) shipOrder(w http.ResponseWriter, r *http.Request) {
	err := s.store.Ship(r.Context(), currentUser(r), r.FormValue("order_id"), r.FormValue("carrier"), r.FormValue("tracking"))
	s.redirectMessage(w, r, messageFor(err, "发货登记成功"))
}

func (s *Server) requestRefund(w http.ResponseWriter, r *http.Request) {
	err := s.store.RequestRefund(r.Context(), currentUser(r), r.FormValue("order_id"))
	s.redirectMessage(w, r, messageFor(err, "退款申请已提交，仓库发货已暂停"))
}

func (s *Server) reviewRefund(w http.ResponseWriter, r *http.Request) {
	approved := r.FormValue("decision") == "approve"
	err := s.store.ReviewRefund(r.Context(), currentUser(r), r.FormValue("refund_id"), r.FormValue("reason"), approved)
	message := "退款已拒绝，订单恢复待发货"
	if approved {
		message = "退款已同意，等待渠道退款通知"
	}
	s.redirectMessage(w, r, messageFor(err, message))
}

func (s *Server) retryRefund(w http.ResponseWriter, r *http.Request) {
	err := s.store.RetryRefund(r.Context(), currentUser(r), r.FormValue("refund_id"), r.FormValue("reason"))
	s.redirectMessage(w, r, messageFor(err, "客服已发起退款重试，等待渠道通知"))
}

func (s *Server) startLateRefund(w http.ResponseWriter, r *http.Request) {
	err := s.store.StartLateRefund(r.Context(), currentUser(r), r.FormValue("exception_id"))
	s.redirectMessage(w, r, messageFor(err, "已对迟到支付发起退款，不恢复库存"))
}

func (s *Server) expireOrders(w http.ResponseWriter, r *http.Request) {
	_, err := s.store.ExpirePending(r.Context())
	s.redirectMessage(w, r, messageFor(err, "已检查并关闭超时订单"))
}

func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	price, priceErr := strconv.ParseInt(r.FormValue("price_cents"), 10, 64)
	available, stockErr := strconv.Atoi(r.FormValue("available"))
	if priceErr != nil || stockErr != nil {
		s.redirectMessage(w, r, "商品价格和可售数量必须是有效整数")
		return
	}
	err := s.store.UpdateProduct(r.Context(), currentUser(r), r.FormValue("product_id"), price, available, r.FormValue("on_sale") == "1")
	s.redirectMessage(w, r, messageFor(err, "商品维护已保存，不影响历史订单快照"))
}

func messageFor(err error, success string) string {
	if err == nil {
		return success
	}
	if errors.Is(err, app.ErrDuplicate) {
		return "重复通知已幂等处理"
	}
	return err.Error()
}
