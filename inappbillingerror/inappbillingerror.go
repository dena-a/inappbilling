package inappbillingerror

var (
	ErrTransactionNotFound = new(`Transaction Not Found`)
)

type InAppBillingError struct {
	s string
}

func new(text string) error {
	return &InAppBillingError{text}
}

func (e InAppBillingError) Error() string {
	return e.s
}
