package internal

import (
	"os"
	"testing"
	"time"
)

func TestExtractConfigDumpContent(t *testing.T) {
	configContent, err := os.ReadFile("../test/data/list_config_spec.dump/config.dump")
	if err != nil {
		t.Fatal(err)
	}

	specContent, err := os.ReadFile("../test/data/list_config_spec.dump/spec.dump")
	if err != nil {
		t.Fatal(err)
	}

	chkptConfig, err := extractConfigDumpContent(configContent, specContent)
	if err != nil {
		t.Fatalf("ExtractConfigDumpContent failed: %v", err)
	}

	expectedNamespace := "default"
	expectedPod := "pod-name"
	expectedContainer := "container-name"
	expectedContainerManager := "cri-o"
	expectedTimestamp := time.Date(2024, 1, 28, 0, 10, 45, 673538606, time.FixedZone("", 19800))
	if chkptConfig.Namespace != expectedNamespace || chkptConfig.Pod != expectedPod || chkptConfig.Container != expectedContainer || !chkptConfig.Timestamp.Equal(expectedTimestamp) || chkptConfig.ContainerManager != expectedContainerManager {
		t.Errorf("ExtractConfigDumpContent returned unexpected values: namespace=%s, pod=%s, container=%s, timestamp=%v", chkptConfig.Namespace, chkptConfig.Pod, chkptConfig.Container, chkptConfig.Timestamp)
	}
}
