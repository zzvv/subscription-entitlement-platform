package domain

// Receipt records the notification work created by a confirmed subscription command.
type Receipt struct {
	ID             string
	Tenant         string
	SubscriptionID string
	Action         string
	Status         string
}

func NewReceipt(id, tenant, subscriptionID, action string) Receipt {
	return Receipt{
		ID:             id,
		Tenant:         tenant,
		SubscriptionID: subscriptionID,
		Action:         action,
		Status:         "pending",
	}
}
