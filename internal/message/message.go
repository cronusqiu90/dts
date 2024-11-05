package message

import (
	"crawlab.org/internal/db"
)

type Kwargs struct {
	Category             int    `json:"category"`
	Domain               []int  `json:"domain"`
	DataType             int    `json:"data_type"`
	Source               int    `json:"source"`
	Hostname             string `json:"hostname"`
	RemainTasks          int    `json:"remain_tasks"`
	AliveAccounts        int    `json:"alive_accounts"`
	DisabledAliveAccount bool   `json:"disabled_alive_account"`
}
type Request struct {
	ID     string `json:"id"`
	Kwargs Kwargs `json:"kwargs"`
}

type Response struct {
	ID        string        `json:"id"`
	Status    string        `json:"status"`
	Traceback interface{}   `json:"traceback"`
	Result    interface{}   `json:"result"`
	Children  []interface{} `json:"children"`
}

type Result struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Tasks   []db.ImTask `json:"tasks"`
}
