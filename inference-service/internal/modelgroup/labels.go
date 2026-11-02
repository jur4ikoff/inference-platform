// Package modelgroup groups model containers in Docker Desktop independently of the platform services.
package modelgroup

import (
	"crypto/sha256"
	"encoding/hex"
)

const Project = "inference-models"

// Labels returns Compose metadata for a model container with its own service name.
func Labels(service string) map[string]string {
	// Compose's project listing requires a config-hash label. These containers
	// are managed by inference-service, so use their stable service identity.
	hash := sha256.Sum256([]byte(service))
	return map[string]string{
		"com.docker.compose.project":              Project,
		"com.docker.compose.service":              service,
		"com.docker.compose.container-number":     "1",
		"com.docker.compose.oneoff":               "False",
		"com.docker.compose.config-hash":          hex.EncodeToString(hash[:]),
		"com.docker.compose.project.config_files": "",
	}
}
