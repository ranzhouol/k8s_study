package util

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"
)

// GetTargetSecret will get secrets(type=targetType, namespace=targetNamespace) from a list of secret references.
func GetTargetSecret(client kubeclient.Interface, secretReferences []corev1.ObjectReference, targetType corev1.SecretType, targetNamespace string) (*corev1.Secret, error) {
	var errNotFound error
	for _, objectReference := range secretReferences {
		logrus.Infof("checking secret: %s/%s", targetNamespace, objectReference.Name)
		secret, err := client.CoreV1().Secrets(targetNamespace).Get(context.TODO(), objectReference.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				errNotFound = err
				continue
			}
			return nil, err
		}

		if secret.Type == targetType {
			return secret, nil
		}
	}

	if errNotFound == nil {
		return nil, fmt.Errorf("no specific secret found in namespace: %s", targetNamespace)
	}
	return nil, errNotFound
}

// CreateSecret just try to create the secret.
func CreateSecret(client kubeclient.Interface, secret *corev1.Secret) (*corev1.Secret, error) {
	st, err := client.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return client.CoreV1().Secrets(secret.Namespace).Get(context.TODO(), secret.Name, metav1.GetOptions{})
		}
		return nil, err
	}

	return st, nil
}

// PatchSecret just try to patch the secret.
func PatchSecret(client kubeclient.Interface, namespace, name string, pt types.PatchType, patchSecretBody *corev1.Secret) error {
	patchSecretByte, err := json.Marshal(patchSecretBody)
	if err != nil {
		logrus.Errorf("failed to marshal patch body of secret object %v into JSON: %v", patchSecretByte, err)
		return err
	}

	_, err = client.CoreV1().Secrets(namespace).Patch(context.TODO(), name, pt, patchSecretByte, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func IfSecretExists(clusterClient kubeclient.Interface, targetNamespace, targetName string) (bool, error) {
	_, err := clusterClient.CoreV1().Secrets(targetNamespace).Get(context.TODO(), targetName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
