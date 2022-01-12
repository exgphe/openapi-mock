package openapi

import (
	"github.com/google/uuid"
	"time"
)

type EstablishSubscriptionInput struct {
	Input struct {
		Encoding     string `json:"encoding"`
		Subscription struct {
			Subscription []Subscription `json:"subscription"`
		} `json:"subscriptions"`
	} `json:"ietf-subscribed-notifications:input"`
}

type Subscription struct {
	Topic          string `json:"topic"`
	ObjectTypeInfo string `json:"object-type-info"`
}

type EstablishSubscriptionOutputWrapped struct {
	Output EstablishSubscriptionOutput `json:"ietf-subscribed-notifications:output"`
}

type EstablishSubscriptionOutput struct {
	ID uint32 `json:"id"`
	//ReplayStartTimeRevision string `json:"replay-start-time-revision"`
}

type DeleteSubscriptionInput struct {
	Input struct {
		ID uint32 `json:"id"`
	} `json:"ietf-subscribed-notifications:input"`
}

const (
	TopicResources = "resources"
	TopicServices  = "services"

	ObjectTypeInfoNode           = "NODE"
	ObjectTypeInfoLink           = "LINK"
	ObjectTypeInfoTP             = "TP"
	ObjectTypeInfoTTP            = "TTP"
	ObjectTypeInfoTunnel         = "TUNNEL"
	ObjectTypeInfoClientService  = "client-service"
	ObjectTypeInfoEthTranService = "eth-tran-service"
	ObjectTypeInfoServicePm      = "service-pm"

	OperationCreate = "create"
	OperationDelete = "delete"
	OperationUpdate = "update"
)

func NoSuchSubscriptionError() RestconfError {
	return RestconfError{
		ErrorType:    ErrorTypeApplication,
		ErrorTag:     ErrorTagInvalidValue,
		ErrorMessage: "Referenced subscription doesn't exist. This may be as a result of a nonexistent subscription ID, an ID that belongs to another subscriber, or an ID for a configured subscription.",
		ErrorAppTag:  "ietf-subscribed-notifications:no-such-subscription",
	}
}

func EncodingUnsupportedError() RestconfError {
	return RestconfError{
		ErrorType:    ErrorTypeApplication,
		ErrorTag:     ErrorTagInvalidValue,
		ErrorMessage: "Unable to encode notification messages in the desired format.",
		ErrorAppTag:  "ietf-subscribed-notifications:encoding-unsupported",
	}
}

func (output EstablishSubscriptionOutput) Wrap() EstablishSubscriptionOutputWrapped {
	return EstablishSubscriptionOutputWrapped{Output: output}
}

type RestconfNotification struct {
	Notification RestconfNotificationBody `json:"ietf-restconf:notification"`
}

type RestconfNotificationBody struct {
	EventTime        string           `json:"eventTime"`
	PushChangeUpdate PushChangeUpdate `json:"ietf-yang-push:push-change-update"`
}

type PushChangeUpdate struct {
	SubscriptionID   uint32        `json:"subscription-id"`
	UpdatesNotSent   []interface{} `json:"updates-not-sent,omitempty"`
	DatastoreChanges interface{}   `json:"datastore-changes"`
}

type YangPatch struct {
	YangPatch YangPatchBody `json:"ietf-yang-patch:yang-patch"`
}

type YangPatchBody struct {
	PatchID string `json:"patch-id"`
	//Comment string          `json:"comment"`
	Edit []YangPatchEdit `json:"edit"`
}

type YangPatchEdit struct {
	EditID    string      `json:"edit-id"`
	Operation string      `json:"operation"`
	Target    string      `json:"target"`
	Value     interface{} `json:"value"`
}

func NewRestconfNotification(id uint32, operation string, target string, value interface{}) RestconfNotification {
	currentTime := time.Now()
	return RestconfNotification{
		Notification: RestconfNotificationBody{
			EventTime: currentTime.UTC().Format("2006-01-02T15:04:05.000Z"),
			PushChangeUpdate: PushChangeUpdate{
				SubscriptionID: id,
				DatastoreChanges: YangPatch{
					YangPatch: YangPatchBody{
						PatchID: uuid.New().String(),
						Edit:    []YangPatchEdit{{EditID: "0", Operation: operation, Target: target, Value: value}},
					},
				},
			},
		},
	}
}
