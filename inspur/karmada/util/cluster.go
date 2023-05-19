package util

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clusterv1alpha1 "ranzhouol/k8s_study/inspur/karmada/cluster/v1alpha1"
)

const (
	// KubeCredentials is the secret that contains mandatory credentials whether reported when registering cluster
	KubeCredentials = "KubeCredentials"
	// KubeImpersonator is the secret that contains the token of impersonator whether reported when registering cluster
	KubeImpersonator = "KubeImpersonator"
)

// ObtainClusterID returns the cluster ID property with clusterKubeClient
func ObtainClusterID(clusterKubeClient kubernetes.Interface) (string, error) {
	ns, err := clusterKubeClient.CoreV1().Namespaces().Get(context.TODO(), metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(ns.UID), nil
}

// ClusterRegisterOption represents the option for RegistryCluster.
type ClusterRegisterOption struct {
	ClusterNamespace   string
	ClusterName        string
	ReportSecrets      []string
	ClusterAPIEndpoint string
	ProxyServerAddress string
	ClusterProvider    string
	ClusterRegion      string
	ClusterZone        string
	DryRun             bool

	ControlPlaneConfig *rest.Config
	ClusterConfig      *rest.Config
	Secret             corev1.Secret
	ImpersonatorSecret corev1.Secret
	ClusterID          string
}

// CreateClusterObject create cluster object in karmada control plane
func CreateClusterObject(controlPlaneClient *dynamic.DynamicClient, clusterObj *clusterv1alpha1.Cluster) (*clusterv1alpha1.Cluster, error) {
	// 检查集群名字是否存在
	cluster, exist, err := GetClusterWithKarmadaClient(controlPlaneClient, clusterObj.Name)
	if err != nil {
		return nil, err
	}

	if exist {
		return cluster, fmt.Errorf("cluster(%s) already exist", clusterObj.Name)
	}

	if cluster, err = createCluster(controlPlaneClient, clusterObj); err != nil {
		logrus.Errorf("Failed to create cluster(%s). error: %v", clusterObj.Name, err)
		return nil, err
	}

	return cluster, nil
}

// GetClusterWithKarmadaClient tells if a cluster already joined to control plane.
func GetClusterWithKarmadaClient(client *dynamic.DynamicClient, name string) (*clusterv1alpha1.Cluster, bool, error) {
	gvr := schema.GroupVersionResource{"cluster.karmada.io", "v1alpha1", "clusters"}

	unstructObj, err := client.Resource(gvr).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		logrus.Errorf("Failed to retrieve cluster(%s). error: %v", name, err)
		return nil, false, err
	}

	cluster := &clusterv1alpha1.Cluster{}

	// 转换
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructObj.UnstructuredContent(), cluster)
	if err != nil {
		return nil, false, err
	}

	return cluster, true, nil
}

func createCluster(controlPlaneClient *dynamic.DynamicClient, cluster *clusterv1alpha1.Cluster) (*clusterv1alpha1.Cluster, error) {
	gvr := schema.GroupVersionResource{"cluster.karmada.io", "v1alpha1", "clusters"}

	clusterMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&cluster)
	if err != nil {
		return nil, err
	}

	newClusterUnstruct, err := controlPlaneClient.Resource(gvr).Create(context.TODO(), &unstructured.Unstructured{clusterMap}, metav1.CreateOptions{})
	if err != nil {
		logrus.Errorf("Failed to create cluster(%s). error: %v", cluster.Name, err)
		return nil, err
	}
	newCluster := &clusterv1alpha1.Cluster{}
	// 转换

	if err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(newClusterUnstruct.UnstructuredContent(), newCluster); err != nil {
		return nil, err
	}
	return newCluster, nil
}

// IsClusterIdentifyUnique checks whether the ClusterID exists in the karmada control plane.
func IsClusterIdentifyUnique(dynamiccontrolPlaneClient *dynamic.DynamicClient, id string) (bool, string, error) {
	gvr := schema.GroupVersionResource{"cluster.karmada.io", "v1alpha1", "clusters"}

	unstructObj, err := dynamiccontrolPlaneClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, "", err
	}

	clusterList := &clusterv1alpha1.ClusterList{}

	// 转换
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructObj.UnstructuredContent(), clusterList)
	if err != nil {
		return false, "", err
	}

	for _, cluster := range clusterList.Items {
		if cluster.Spec.ID == id {
			return false, cluster.Name, nil
		}
	}
	return true, "", nil
}
