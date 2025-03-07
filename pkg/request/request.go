package request

import "time"

type Request struct {
	Method       string            `json:"method"`
	URI          string            `json:"uri"`
	Body         []byte            `json:"body,omitempty"`
	Header       map[string]string `json:"header"`
	Status       int               `json:"status"` // status as reported by original server
	Timestamp    time.Time         `json:"ts"`     // time the request was created
	RemoteAddr   string            `json:"remote_addr"`
	UserAgent    string            `json:"agent"`
	Referer      string            `json:"referer"`
	RespBodySize int               `json:"resp_body_size"`
	RespTime     float32           `json:"resp_time"`
}
