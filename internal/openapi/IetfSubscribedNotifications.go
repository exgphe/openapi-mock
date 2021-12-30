package openapi

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
