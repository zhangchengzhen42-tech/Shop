package app

import "errors"

var (
	ErrForbidden        = errors.New("无权执行此操作")
	ErrNotFound         = errors.New("记录不存在")
	ErrInvalidQuantity  = errors.New("每单数量必须在 1 至 2 件之间")
	ErrOutOfStock       = errors.New("商品未上架或可售库存不足")
	ErrInvalidState     = errors.New("当前状态不允许此操作")
	ErrDuplicate        = errors.New("重复通知已幂等处理")
	ErrAmountMismatch   = errors.New("支付金额与订单金额不一致")
	ErrInvalidSignature = errors.New("支付通知验签失败")
	ErrTxConflict       = errors.New("渠道交易号已关联其他订单")
	ErrReasonRequired   = errors.New("处理原因不能为空")
)
