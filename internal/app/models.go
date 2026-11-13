package app

import "time"

const (
	RoleBuyer      = "buyer"
	RoleOperations = "operations"
	RoleWarehouse  = "warehouse"
	RoleService    = "service"
	RoleChannel    = "channel"

	OrderPending       = "PENDING"
	OrderAwaitShipment = "AWAIT_SHIPMENT"
	OrderRefundReview  = "REFUND_REVIEW"
	OrderRefunding     = "REFUNDING"
	OrderRefunded      = "REFUNDED"
	OrderShipped       = "SHIPPED"
	OrderClosed        = "CLOSED"

	RefundReview         = "REVIEW"
	RefundRejected       = "REJECTED"
	RefundPendingChannel = "PENDING_CHANNEL"
	RefundFailed         = "FAILED"
	RefundSuccess        = "SUCCESS"
)

type User struct {
	ID   string
	Name string
	Role string
}

type Product struct {
	ID        string
	Name      string
	Price     int64
	Available int
	OnSale    bool
}

type Order struct {
	ID          string
	BuyerID     string
	BuyerName   string
	ProductID   string
	ProductName string
	UnitPrice   int64
	Quantity    int
	Total       int64
	Status      string
	CreatedAt   time.Time
	CloseAt     time.Time
	ClosedAt    *time.Time
	CloseReason string
	PaymentTx   string
	PaidAt      *time.Time
	Carrier     string
	Tracking    string
}

type Refund struct {
	ID          string
	OrderID     string
	ExceptionID string
	Kind        string
	Amount      int64
	Status      string
	Reason      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Shipment struct {
	OrderID   string
	Carrier   string
	Tracking  string
	CreatedAt time.Time
}

type Payment struct {
	OrderID            string
	ChannelTx          string
	Amount             int64
	VerificationStatus string
	Result             string
	Reason             string
	ReceivedAt         time.Time
}

type Exception struct {
	ID        string
	OrderID   string
	ChannelTx string
	Amount    int64
	Kind      string
	Status    string
	Reason    string
	CreatedAt time.Time
}

type Audit struct {
	CreatedAt time.Time
	Actor     string
	Action    string
	Detail    string
}

type Dashboard struct {
	Products   []Product
	Orders     []Order
	Payments   []Payment
	Refunds    []Refund
	Shipments  []Shipment
	Exceptions []Exception
	Audits     []Audit
}

type Summary struct {
	Available      int
	ReceivedCents  int64
	RefundedCents  int64
	AwaitShipment  int
	PendingPayment int
}
