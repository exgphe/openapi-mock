package subscriptionCenter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/set"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	net "github.com/subchord/go-sse"
	"log"
	"net/http"
	"time"
)

type SubscriptionCenter struct {
	subscriptions map[uint32][]string // subscription id -> objectType[]
	broker        *net.Broker
	connMap       map[uint32]map[string]*net.ClientConnection // subscription id -> connection id -> connection
}

func NewSubscriptionCenter() *SubscriptionCenter {
	broker := net.NewBroker(map[string]string{"Access-Control-Allow-Origin": "*"})
	broker.SetDisconnectCallback(func(clientId string, sessionId string) {
		log.Printf("session %v of client %v was disconnected.", sessionId, clientId)
	})
	return &SubscriptionCenter{subscriptions: make(map[uint32][]string), broker: broker, connMap: make(map[uint32]map[string]*net.ClientConnection)}
}

func (subscriptionCenter *SubscriptionCenter) Subscribe(subscriptions []openapi.Subscription) (resultId uint32) {
	if subscriptionCenter.subscriptions == nil {
		subscriptionCenter.subscriptions = make(map[uint32][]string)
	}
	resultId = uint32(len(subscriptionCenter.subscriptions) + 1)
	objectTypeInfoSet := set.NewHashSet()
	for _, subscription := range subscriptions {
		objectTypeInfoSet.Add(subscription.ObjectTypeInfo)
	}
	subscriptionCenter.subscriptions[resultId] = make([]string, objectTypeInfoSet.Len())
	for _, objectTypeInfo := range objectTypeInfoSet.Elements() {
		subscriptionCenter.subscriptions[resultId] = append(subscriptionCenter.subscriptions[resultId], objectTypeInfo.(string))
	}
	return
}

func (subscriptionCenter *SubscriptionCenter) Get(id uint32) []string {
	return subscriptionCenter.subscriptions[id]
}

func (subscriptionCenter *SubscriptionCenter) Delete(id uint32) bool {
	if subscriptionCenter.subscriptions[id] == nil {
		return false
	}
	delete(subscriptionCenter.subscriptions, id)
	return true
}

func (subscriptionCenter *SubscriptionCenter) Connect(id uint32, w http.ResponseWriter, r *http.Request) (err error) {
	clientId := uuid.New().String()
	conn, err := subscriptionCenter.broker.ConnectWithHeartBeatInterval(clientId, w, r, 1*time.Second)
	if err != nil {
		return
	}
	if subscriptionCenter.connMap[id] == nil {
		subscriptionCenter.connMap[id] = map[string]*net.ClientConnection{}
	}
	subscriptionCenter.connMap[id][clientId] = conn
	<-conn.Done()
	delete(subscriptionCenter.connMap[id], clientId)
	return nil
}

func (subscriptionCenter *SubscriptionCenter) SendAll(objectType string, operation string, value interface{}, ids ...string) error {
	target, err := objectTypeToTarget(objectType, ids...)
	if err != nil {
		return err
	}
	for subscriptionID, objectTypes := range subscriptionCenter.subscriptions {
		for _, t := range objectTypes {
			if t == objectType {
				notification := openapi.NewRestconfNotification(subscriptionID, operation, target, value)
				subscriptionCenter.Send(notification)
				break
			}
		}
	}
	return nil
}

func objectTypeToTarget(objectType string, ids ...string) (string, error) {
	switch objectType {
	case openapi.ObjectTypeInfoNode:
		url := "/restconf/data/ietf-network:networks/network/network-id=" + ids[0] + "/node"
		if len(ids) > 1 {
			url += "/node-id=" + ids[1]
		}
		return url, nil
	default:
		return "", errors.New("Object type " + objectType + " not supported")
	}
}

func (subscriptionCenter *SubscriptionCenter) Send(notification openapi.RestconfNotification) {
	for _, conn := range subscriptionCenter.connMap[notification.Notification.PushChangeUpdate.SubscriptionID] {
		conn.Send(&SSEEvent{
			Data: notification,
		})
	}
}

type SSEEvent struct {
	Data interface{}
}

func (e SSEEvent) Prepare() []byte {
	var data bytes.Buffer

	marshal, err := json.Marshal(e.Data)
	if err != nil {
		logrus.Errorf("error marshaling JSONEvent: %v", err)
		return []byte{}
	}

	data.WriteString(fmt.Sprintf("data: %s\n", string(marshal)))
	data.WriteString("\n")

	return data.Bytes()
}

func (e SSEEvent) GetId() string {
	return ""
}

func (e SSEEvent) GetEvent() string {
	return ""
}

func (e SSEEvent) GetData() string {
	marshal, err := json.Marshal(e.Data)
	if err != nil {
		logrus.Errorf("error marshaling JSONEvent: %v", err)
		return ""
	}
	return string(marshal)
}
