package api

import (
	"encoding/json"
)

type Event struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}
