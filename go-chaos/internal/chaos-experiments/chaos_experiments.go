package chaos_experiments

import (
	"bytes"
	"embed"
	"encoding/json"
	"strings"

	"github.com/camunda/zeebe-chaos/go-chaos/internal"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// chaosContent holds our static camunda cloud chaos experiments, which are copied with the go:embed directive
//
//go:embed camunda-cloud/*
var chaosContent embed.FS

const experimentFileName = "experiment.json"
const contentFolder = "camunda-cloud"
const manifestFileName = contentFolder + "/manifest.yml"

type Experiments struct {
	Experiments []map[string]interface{} `json:"experiments"`
}

func ReadExperimentsForClusterPlan(clusterPlan string, targetClusterVersion string) (Experiments, error) {
	// semver package requires versions to start with "v"
	normalizedClusterPlan := strings.ToLower(strings.Replace(clusterPlan, " ", "", -1))
	manifest, err := readManifest()
	if err != nil {
		return Experiments{}, err
	}
	paths := manifest.filterExperiments(normalizedClusterPlan, targetClusterVersion)
	internal.LogVerbose("Given cluster plan '%s' and target cluster version '%s', selected the following experiments: [%#v]",
		normalizedClusterPlan, targetClusterVersion, paths)

	experiments := Experiments{}
	for _, path := range paths {
		experimentBytes, err := chaosContent.ReadFile(contentFolder + "/" + path)
		if err != nil {
			return experiments, err
		}
		var jsonObj map[string]interface{}
		err = json.Unmarshal(experimentBytes, &jsonObj)
		if err != nil {
			return experiments, err
		}
		experiments.Experiments = append(experiments.Experiments, jsonObj)
	}

	return experiments, err
}

type manifest struct {
	Experiments []experiment `yaml:"experiments"`
}

type experiment struct {
	Path         string   `yaml:"path"`
	ClusterPlans []string `yaml:"clusterPlans"`
	MinVersion   string   `yaml:"minVersion"`
	MaxVersion   string   `yaml:"maxVersion"`
}

func (m manifest) filterExperiments(clusterPlan string, targetVersion string) (experiments []string) {
	for _, entry := range m.Experiments {
		isValidClusterPlan := slices.Contains(entry.ClusterPlans, clusterPlan)
		matchesMinVersion := entry.MinVersion == "" || (targetVersion != "" && semver.Compare(semver.MajorMinor("v"+targetVersion), semver.MajorMinor("v"+entry.MinVersion)) >= 0)
		matchesMaxVersion := entry.MaxVersion == "" || (targetVersion != "" && semver.Compare(semver.MajorMinor("v"+targetVersion), semver.MajorMinor("v"+entry.MaxVersion)) <= 0)
		matchesVersion := matchesMinVersion && matchesMaxVersion

		if isValidClusterPlan && matchesVersion {
			experiments = append(experiments, entry.Path)
		} else {
			internal.LogInfo(
				"Skipping experiment at '%#v' for cluster plan '%s' and target version '%s' because: [isValidClusterPlan=%t, matchesMinVersion=%t, matchesMaxVersion=%t]",
				entry, clusterPlan, targetVersion, isValidClusterPlan, matchesMinVersion, matchesMaxVersion)
		}
	}

	return
}

func readManifest() (m manifest, err error) {
	manifestBytes, err := chaosContent.ReadFile(manifestFileName)
	if err != nil {
		return
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifestBytes), 0)
	err = decoder.Decode(&m)
	return
}
