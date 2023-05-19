package names

import (
	"fmt"
)

// GenerateServiceAccountName generates the name of a ServiceAccount.
func GenerateServiceAccountName(clusterName string) string {
	return fmt.Sprintf("%s-%s", "karmada", clusterName)
}

// GenerateRoleName generates the name of a Role or ClusterRole.
func GenerateRoleName(serviceAccountName string) string {
	return fmt.Sprintf("karmada-controller-manager:%s", serviceAccountName)
}

// GenerateImpersonationSecretName generates the secret name of impersonation secret.
func GenerateImpersonationSecretName(clusterName string) string {
	return fmt.Sprintf("%s-impersonator", clusterName)
}
