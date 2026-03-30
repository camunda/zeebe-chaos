package chaos_experiments

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ShouldReadExperiments(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("G3-S", "8.4.0-SNAPSHOT")

	// then
	require.NoError(t, err)
	assert.NotNil(t, experimentsForClusterPlan)

	require.NoError(t, err)
	assert.Greater(t, len(experimentsForClusterPlan.Experiments), 2)
	assert.NotEqual(t, experimentsForClusterPlan.Experiments[0], experimentsForClusterPlan.Experiments[1])
	assert.NotEqual(t, "Zeebe deployment distribution", experimentsForClusterPlan.Experiments[0])
}

func Test_ShouldBeAbleToMarshalExperiments(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("G3-S", "8.4.0-SNAPSHOT")

	// then
	marshal, err := json.Marshal(experimentsForClusterPlan)
	require.NoError(t, err)

	// experiments key should be small
	assert.Contains(t, string(marshal), "experiments")
}

func Test_ShouldExcludeExperimentsWithLowerMinVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.2.0-SNAPSHOT")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 1, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
}

func Test_ShouldExcludeExperimentsWithHigherMaxVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.5.0-SNAPSHOT")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 1, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
}

func Test_ShouldIncludeExperimentsForClusterPlan(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.3.0-SNAPSHOT")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
	assert.Equal(t, "Versioned Test Experiment", experimentsForClusterPlan.Experiments[1]["title"])
}

func Test_ShouldHandleNoExperimentsFound(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("i do not exist", "8.3.0-SNAPSHOT")

	// then
	assert.NoError(t, err)
	assert.Empty(t, experimentsForClusterPlan.Experiments)
}

func Test_ShouldExcludeExperimentsWithVersionBoundsOnEmptyTargetVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 1, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
}

func Test_ShouldIncludeExperimentsWithMatchingMaxVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.4")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
	assert.Equal(t, "Versioned Test Experiment", experimentsForClusterPlan.Experiments[1]["title"])
}

func Test_ShouldIncludeExperimentsWithMatchingMinVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.3")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
	assert.Equal(t, "Versioned Test Experiment", experimentsForClusterPlan.Experiments[1]["title"])
}

func Test_ShouldIncludeExperimentsWithMatchingVersion(t *testing.T) {
	// given

	// when
	experimentsForClusterPlan, err := ReadExperimentsForClusterPlan("test", "8.3.9")

	// then
	assert.NoError(t, err)
	assert.Equal(t, 2, len(experimentsForClusterPlan.Experiments))
	assert.Equal(t, "This is a fake experiment", experimentsForClusterPlan.Experiments[0]["title"])
	assert.Equal(t, "Versioned Test Experiment", experimentsForClusterPlan.Experiments[1]["title"])
}
