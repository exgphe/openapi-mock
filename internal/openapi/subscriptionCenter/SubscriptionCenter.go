package subscriptionCenter

import (
	"bytes"
	"encoding/json"
	"fmt"
	net "github.com/exgphe/go-sse"
	"github.com/google/uuid"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/set"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

type SubscriptionCenter struct {
	counter       uint32
	subscriptions map[uint32][]string // subscription id -> objectType[]
	brokerMap     map[uint32]*net.Broker
	connMap       map[uint32]map[string]*net.ClientConnection // subscription id -> connection id -> connection
}

type SubscriptionCenterDTO struct {
	Counter       uint32
	Subscriptions map[uint32][]string // subscription id -> objectType[]
}

const path = "subscriptions.json"

func newBroker() *net.Broker {
	broker := net.NewBroker(map[string]string{"Access-Control-Allow-Origin": "*"})
	broker.SetDisconnectCallback(func(clientId string, sessionId string) {
		log.Printf("session %v of client %v was disconnected.", sessionId, clientId)
	})
	return broker
}

func NewSubscriptionCenter() *SubscriptionCenter {
	sc := &SubscriptionCenter{counter: 0, subscriptions: make(map[uint32][]string), brokerMap: make(map[uint32]*net.Broker), connMap: make(map[uint32]map[string]*net.ClientConnection)}
	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		// file exists
		var fileContent []byte
		fileContent, _ = ioutil.ReadFile(path)
		var dto SubscriptionCenterDTO
		err = json.Unmarshal(fileContent, &dto)
		if err == nil {
			sc.counter = dto.Counter
			sc.subscriptions = dto.Subscriptions
		}
	}
	return sc
}

func (subscriptionCenter *SubscriptionCenter) Save() (err error) {
	dto := SubscriptionCenterDTO{
		Counter:       subscriptionCenter.counter,
		Subscriptions: subscriptionCenter.subscriptions,
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(path, data, fs.ModePerm)
	return
}

func (subscriptionCenter *SubscriptionCenter) Subscribe(subscriptions []openapi.Subscription) (resultId uint32) {
	if subscriptionCenter.subscriptions == nil {
		subscriptionCenter.subscriptions = make(map[uint32][]string)
	}
	resultId = atomic.AddUint32(&subscriptionCenter.counter, 1)
	objectTypeInfoSet := set.NewHashSet()
	for _, subscription := range subscriptions {
		objectTypeInfoSet.Add(subscription.ObjectTypeInfo)
	}
	subscriptionCenter.subscriptions[resultId] = make([]string, objectTypeInfoSet.Len())
	for i, objectTypeInfo := range objectTypeInfoSet.Elements() {
		subscriptionCenter.subscriptions[resultId][i] = objectTypeInfo.(string)
	}
	_ = subscriptionCenter.Save()
	subscriptionCenter.brokerMap[resultId] = newBroker()
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
	if subscriptionCenter.connMap[id] != nil {
		delete(subscriptionCenter.connMap, id)
	}
	if subscriptionCenter.brokerMap[id] != nil {
		err := subscriptionCenter.brokerMap[id].Close()
		if err != nil {
			println(err)
		}
	}
	delete(subscriptionCenter.brokerMap, id)
	err := subscriptionCenter.Save()
	if err != nil {
		println(err)
	}
	return true
}

func (subscriptionCenter *SubscriptionCenter) Connect(id uint32, interval uint64, w http.ResponseWriter, r *http.Request) (err error) {
	clientId := uuid.New().String()
	if subscriptionCenter.brokerMap[id] == nil {
		subscriptionCenter.brokerMap[id] = newBroker()
	}
	println("Connecting with new client to subscription ", id)
	conn, err := subscriptionCenter.brokerMap[id].ConnectWithHeartBeatInterval(clientId, w, r, time.Duration(interval)*time.Second)
	if err != nil {
		return
	}
	if subscriptionCenter.connMap[id] == nil {
		subscriptionCenter.connMap[id] = map[string]*net.ClientConnection{}
	}
	subscriptionCenter.connMap[id][clientId] = conn
	println("Connected with new client to subscription ", id, "with session id ", conn.SessionId())
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

func objectTypeToTarget(objectType string, unescapedIds ...string) (string, error) {
	ids := make([]string, len(unescapedIds))
	for i := range unescapedIds {
		ids[i] = url.QueryEscape(unescapedIds[i])
	}
	switch objectType {
	case openapi.ObjectTypeInfoNode:
		return "/restconf/data/ietf-network:networks/network=" + ids[0] + "/node=" + ids[1], nil
	case openapi.ObjectTypeInfoTP:
		return "/restconf/data/ietf-network:networks/network=" + ids[0] + "/node=" + ids[1] + "/ietf-network-topology:termination-point=" + ids[2], nil
	case openapi.ObjectTypeInfoTTP:
		return "/restconf/data/ietf-network:networks/network=" + ids[0] + "/node=" + ids[1] + "/ietf-te-topology:te/tunnel-termination-point=" + ids[2], nil
	case openapi.ObjectTypeInfoLink:
		return "/restconf/data/ietf-network:networks/network=" + ids[0] + "/ietf-network-topology:link=" + ids[1], nil
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
