package platform

import "context"

type Check struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

type Report struct {
	Platform string  `json:"platform"`
	Ready    bool    `json:"ready"`
	Checks   []Check `json:"checks"`
}

type Adapter interface {
	ID() string
	Doctor(context.Context) (Report, error)
}
