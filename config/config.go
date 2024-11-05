package Config

import (
	"encoding/base64"
	"os"

	"gopkg.in/yaml.v2"

	"crawlab.org/internal/crypto"
)

type ESConfig struct {
	GetTask    string `yaml:"get_task"`
	UpdateTask string `yaml:"update_task"`
}

type config struct {
	ES    ESConfig `yaml:"es"`
	AMQP  string   `yaml:"amqp"`
	Cache string   `yaml:"cache"`
}

var CONF config

func init() {
	crypto.InitKey("")
}

func Setup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, &CONF); err != nil {
		return err
	}

	CONF.AMQP, err = decryptItem(CONF.AMQP)
	if err != nil {
		return err
	}

	CONF.Cache, err = decryptItem(CONF.Cache)
	if err != nil {
		return err
	}

	return nil
}

func decryptItem(s string) (string, error) {
	if len(s) == 0 {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	decStr, err := crypto.Decrypt(raw)
	if err != nil {
		return "", err
	}
	return string(decStr), nil
}
