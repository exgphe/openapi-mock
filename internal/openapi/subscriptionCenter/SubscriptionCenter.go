package subscriptionCenter

import (
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/set"
)

type SubscriptionCenter struct {
	Subscriptions map[uint32][]string
}

func NewSubscriptionCenter() *SubscriptionCenter {
	return &SubscriptionCenter{Subscriptions: make(map[uint32][]string)}
}

func (subscriptionCenter *SubscriptionCenter) Subscribe(subscriptions []openapi.Subscription) (resultId uint32) {
	if subscriptionCenter.Subscriptions == nil {
		subscriptionCenter.Subscriptions = make(map[uint32][]string)
	}
	resultId = uint32(len(subscriptionCenter.Subscriptions) + 1)
	objectTypeInfoSet := set.NewHashSet()
	for _, subscription := range subscriptions {
		objectTypeInfoSet.Add(subscription.ObjectTypeInfo)
	}
	subscriptionCenter.Subscriptions[resultId] = make([]string, objectTypeInfoSet.Len())
	for _, objectTypeInfo := range objectTypeInfoSet.Elements() {
		subscriptionCenter.Subscriptions[resultId] = append(subscriptionCenter.Subscriptions[resultId], objectTypeInfo.(string))
	}
	return
}

func (subscriptionCenter *SubscriptionCenter) Get(id uint32) []string {
	return subscriptionCenter.Subscriptions[id]
}

func (subscriptionCenter *SubscriptionCenter) Delete(id uint32) bool {
	if subscriptionCenter.Subscriptions[id] == nil {
		return false
	}
	delete(subscriptionCenter.Subscriptions, id)
	return true
}
