package web

import (
	"net/http"
	"strconv"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
)

func (s *Server) channelPayment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	amount, err := strconv.ParseInt(r.FormValue("amount_cents"), 10, 64)
	if err != nil {
		s.redirectMessage(w, r, "支付金额必须是分的整数")
		return
	}
	valid := r.FormValue("signature") == "valid"
	err = s.store.RecordPayment(r.Context(), user, r.FormValue("order_id"), r.FormValue("channel_tx"), amount, valid)
	s.redirectMessage(w, r, messageFor(err, "支付通知已处理"))
}

func (s *Server) channelRefund(w http.ResponseWriter, r *http.Request) {
	success := r.FormValue("result") == "success"
	err := s.store.RecordRefund(r.Context(), currentUser(r), r.FormValue("refund_id"), success)
	s.redirectMessage(w, r, messageFor(err, "退款通知已处理"))
}

func requiresRole(user app.User, role string) bool { return user.Role == role }
