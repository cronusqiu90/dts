package util

import "encoding/json"

func PrettyShow(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}
