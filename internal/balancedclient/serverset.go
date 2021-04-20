package balancedclient

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ServerSetStatus uint8

const (
	StatusDead ServerSetStatus = iota
	StatusStarting
	StatusAlive
	StatusStopping
	StatusStopped
	StatusWarning
)

var serversetStatusNames = []string{
	"DEAD",
	"STARTING",
	"ALIVE",
	"STOPPING",
	"STOPPED",
	"WARNING",
}

func (st ServerSetStatus) String() string {
	if uint(st) >= uint(len(serversetStatusNames)) {
		return fmt.Sprintf("#%d", uint(st))
	}
	return serversetStatusNames[st]
}

func (st ServerSetStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

func (ptr *ServerSetStatus) UnmarshalJSON(raw []byte) error {
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	for index, name := range serversetStatusNames {
		if strings.EqualFold(str, name) {
			*ptr = ServerSetStatus(index)
			return nil
		}
	}
	return fmt.Errorf("unknown ServerSetStatus %q", str)
}

var _ fmt.Stringer = ServerSetStatus(0)
var _ json.Marshaler = ServerSetStatus(0)
var _ json.Unmarshaler = (*ServerSetStatus)(nil)

type ServerSetEndpoint struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
}

type ServerSetMember struct {
	ServiceEndpoint     ServerSetEndpoint            `json:"serviceEndpoint"`
	AdditionalEndpoints map[string]ServerSetEndpoint `json:"additionalEndpoints"`
	Status              ServerSetStatus              `json:"status"`
	ShardID             int32                        `json:"shardId,omitempty"`
	Metadata            map[string]string            `json:"metadata,omitempty"`
}
