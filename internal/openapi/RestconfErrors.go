package openapi

import "encoding/json"

type RestconfErrors struct {
	Errors []RestconfError `json:"error"`
}

type RestconfError struct {
	ErrorType    string      `json:"error-type"`
	ErrorTag     string      `json:"error-tag"`
	ErrorPath    string      `json:"error-path,omitempty"`
	ErrorMessage string      `json:"error-message,omitempty"`
	ErrorAppTag  string      `json:"error-app-tag,omitempty"`
	ErrorInfo    interface{} `json:"error-info,omitempty"`
}

/*
   +-------------------------+------------------+
   | error-tag               | status code      |
   +-------------------------+------------------+
   | in-use                  | 409              |
   |                         |                  |
   | invalid-value           | 400, 404, or 406 |
   |                         |                  |
   | (request) too-big       | 413              |
   |                         |                  |
   | (response) too-big      | 400              |
   |                         |                  |
   | missing-attribute       | 400              |
   |                         |                  |
   | bad-attribute           | 400              |
   |                         |                  |
   | unknown-attribute       | 400              |
   |                         |                  |
   | bad-element             | 400              |
   |                         |                  |
   | unknown-element         | 400              |
   |                         |                  |
   | unknown-namespace       | 400              |
   |                         |                  |
   | access-denied           | 401 or 403       |
   |                         |                  |
   | lock-denied             | 409              |
   |                         |                  |
   | resource-denied         | 409              |
   |                         |                  |
   | rollback-failed         | 500              |
   |                         |                  |
   | data-exists             | 409              |
   |                         |                  |
   | data-missing            | 409              |
   |                         |                  |
   | operation-not-supported | 405 or 501       |
   |                         |                  |
   | operation-failed        | 412 or 500       |
   |                         |                  |
   | partial-operation       | 500              |
   |                         |                  |
   | malformed-message       | 400              |
   +-------------------------+------------------+
*/

const (
	ErrorTypeTransport   = "transport"
	ErrorTypeRpc         = "rpc"
	ErrorTypeProtocol    = "protocol"
	ErrorTypeApplication = "application"

	ErrorTagInvalidValue         = "invalid-value"
	ErrorTagOperationFailed      = "operation-failed"
	ErrorTagOperationNotSuported = "operation-not-supported"
	ErrorTagDataExists           = "data-exists"
	ErrorTagBadElement           = "bad-element"
	ErrorTagResourceDenied       = "resource-denied"
)

func NewRestconfErrors(errors ...RestconfError) RestconfErrors {
	return RestconfErrors{
		Errors: errors,
	}
}

func (r RestconfErrors) Error() string {
	marshal, _ := json.Marshal(r)
	return string(marshal)
}

func (r RestconfErrors) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Errors struct {
			Errors []RestconfError `json:"error"`
		} `json:"ietf-restconf:errors"`
	}{
		Errors: r,
	})
}
