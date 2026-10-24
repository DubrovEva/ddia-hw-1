package golden

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvPod        = "POD_NAME"
	EnvDatacenter = "DATACENTER"
	EnvDeployment = "DEPLOYMENT_NAME"
)

type Config struct {
	Pod        string
	Datacenter string
	Deployment string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Pod:        os.Getenv(EnvPod),
		Datacenter: os.Getenv(EnvDatacenter),
		Deployment: os.Getenv(EnvDeployment),
	}

	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var missing []string
	if c.Pod == "" {
		missing = append(missing, EnvPod)
	}
	if c.Datacenter == "" {
		missing = append(missing, EnvDatacenter)
	}
	if c.Deployment == "" {
		missing = append(missing, EnvDeployment)
	}
	if len(missing) > 0 {
		return fmt.Errorf("golden: missing required settings: %s", strings.Join(missing, ", "))
	}

	return nil
}
