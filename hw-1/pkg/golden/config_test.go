package golden

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvPod, "pod-1")
	t.Setenv(EnvDatacenter, "dc1")
	t.Setenv(EnvDeployment, "items-service")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, Config{Pod: "pod-1", Datacenter: "dc1", Deployment: "items-service"}, cfg)
}

func TestConfigFromEnvMissing(t *testing.T) {
	t.Setenv(EnvPod, "")
	t.Setenv(EnvDatacenter, "dc1")
	t.Setenv(EnvDeployment, "")

	_, err := ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), EnvPod)
	assert.Contains(t, err.Error(), EnvDeployment)
	assert.NotContains(t, err.Error(), EnvDatacenter)
}
