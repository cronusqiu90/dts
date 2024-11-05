package util

import (
	"fmt"
	"strings"
)

func GetSource(label string) (int, error) {
	items := strings.Split(label, "_")
	if len(items) != 3 {
		return 0, fmt.Errorf("failed to parse cmdline(-name)")
	}
	return getProjectCode(items[0])*1000 + getEnvironCode(items[1])*100 + getModuleCode(items[2]), nil
}

func getModuleCode(name string) int {
	switch name {
	case "debug":
		return 0
	case "tc":
		return 1
	case "mq":
		return 2
	case "lbs":
		return 3
	case "cj":
		return 4
	case "tmpl":
		return 5
	case "trace":
		return 6
	case "gz":
		return 7
	default:
		return -1
	}
}

func getEnvironCode(name string) int {
	switch name {
	case "dev":
		return 1
	case "test":
		return 2
	case "trail":
		return 3
	case "prod":
		return 4
	default:
		return -1
	}
}

func getProjectCode(name string) int {
	switch name {
	case "ad3200":
		return 1
	case "newplt":
		return 2
	case "103":
		return 3
	case "qzy":
		return 4
	case "hk":
		return 5
	default:
		return -1
	}
}
