package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1alpha1 "ranzhouol/k8s_study/inspur/karmada/cluster/v1alpha1"
	util2 "ranzhouol/k8s_study/inspur/karmada/util"
	names2 "ranzhouol/k8s_study/inspur/karmada/util/names"
)

const (
	// SecretTokenKey is the name of secret token key.
	SecretTokenKey = "token"

	// SecretCADataKey is the name of secret caBundle key.
	SecretCADataKey = "caBundle"

	// KarmadaConfigPath is the path to karmada-apiserver.config
	KarmadaConfigPath = "/etc/karmada/karmada-apiserver.config"

	GroupName = "cluster.karmada.io"
)

var (
	// Policy rules allowing full access to resources in the cluster or namespace.
	namespacedPolicyRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{rbacv1.VerbAll},
			APIGroups: []string{rbacv1.APIGroupAll},
			Resources: []string{rbacv1.ResourceAll},
		},
	}
	clusterPolicyRules = []rbacv1.PolicyRule{
		namespacedPolicyRules[0],
		{
			NonResourceURLs: []string{rbacv1.NonResourceAll},
			Verbs:           []string{"get"},
		},
	}
	SchemeGroupVersion  = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}
	clusterResourceKind = SchemeGroupVersion.WithKind("Cluster")
)

func main() {
	clusterName := "test1"
	karmadaConfigPath := "D:\\Go\\Go_WorkSpace\\src\\inspur.com\\linux\\5174\\karmada-apiserver.config"
	kubeconfigPath := "D:\\Go\\Go_WorkSpace\\src\\inspur.com\\linux\\5174\\config"

	karmadaConfig, err := clientcmd.BuildConfigFromFlags("", karmadaConfigPath)
	if err != nil {
		panic(err.Error())
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		panic(err.Error())
	}
	//namespace?
	err = joinCluster(karmadaConfig, config, clusterName)
	if err != nil {
		logrus.Errorf("JoinCluster  ====>   err: %s", err.Error())
	}
}

func joinCluster(controlPlaneRestConfig, clusterConfig *rest.Config, clusterName string) error {
	controlPlaneKubeClient := kubeclient.NewForConfigOrDie(controlPlaneRestConfig)
	karmadaClient, err := dynamic.NewForConfig(controlPlaneRestConfig)
	if err != nil {
		logrus.Error("karmadaClient error")
		return err
	}

	clusterKubeClient := kubeclient.NewForConfigOrDie(clusterConfig)

	registerOption := util2.ClusterRegisterOption{
		ClusterNamespace:   "karmada-cluster",
		ClusterName:        clusterName,
		ReportSecrets:      []string{util2.KubeCredentials, util2.KubeImpersonator},
		ClusterProvider:    "",
		ClusterRegion:      "",
		ClusterZone:        "",
		DryRun:             false,
		ControlPlaneConfig: controlPlaneRestConfig,
		ClusterConfig:      clusterConfig,
	}

	// 得到 kube-system 的UID
	id, err := util2.ObtainClusterID(clusterKubeClient)
	if err != nil {
		return err
	}

	// 判断集群是否已经加入
	ok, name, err := util2.IsClusterIdentifyUnique(karmadaClient, id) //karmadaClient
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("the same cluster has been registered with name %s", name)
	}
	//
	registerOption.ClusterID = id

	logrus.Infof("joining cluster config. endpoint: %s", clusterConfig.Host)
	clusterSecret, impersonatorSecret, err := obtainCredentialsFromMemberCluster(
		clusterKubeClient, registerOption)
	if err != nil {
		return err
	}

	registerOption.Secret = *clusterSecret
	registerOption.ImpersonatorSecret = *impersonatorSecret
	// 注册集群到ControllerPlane
	err = registerClusterInControllerPlane(registerOption, controlPlaneKubeClient)
	if err != nil {
		return err
	}

	fmt.Printf("cluster(%s) is joined successfully\n", clusterName)
	return nil
}

// 从成员集群获取凭证
func obtainCredentialsFromMemberCluster(clusterKubeClient kubeclient.Interface, opts util2.ClusterRegisterOption) (*corev1.Secret, *corev1.Secret, error) {
	var err error

	// ensure namespace where the karmada control plane credential be stored exists in cluster.
	if _, err = util2.EnsureNamespaceExist(clusterKubeClient, opts.ClusterNamespace, opts.DryRun); err != nil {
		return nil, nil, err
	}

	// create a ServiceAccount in cluster.
	serviceAccountObj := &corev1.ServiceAccount{}
	serviceAccountObj.Namespace = opts.ClusterNamespace
	serviceAccountObj.Name = names2.GenerateServiceAccountName(opts.ClusterName)
	if serviceAccountObj, err = util2.EnsureServiceAccountExist(clusterKubeClient, serviceAccountObj, opts.DryRun); err != nil {
		return nil, nil, err
	}

	// create a ServiceAccount for impersonation in cluster.
	impersonationSA := &corev1.ServiceAccount{}
	impersonationSA.Namespace = opts.ClusterNamespace
	impersonationSA.Name = names2.GenerateServiceAccountName("impersonator")
	if impersonationSA, err = util2.EnsureServiceAccountExist(clusterKubeClient, impersonationSA, opts.DryRun); err != nil {
		return nil, nil, err
	}

	// create a ClusterRole in cluster.
	clusterRole := &rbacv1.ClusterRole{}
	clusterRole.Name = names2.GenerateRoleName(serviceAccountObj.Name)
	clusterRole.Rules = clusterPolicyRules
	if _, err = ensureClusterRoleExist(clusterKubeClient, clusterRole, opts.DryRun); err != nil {
		return nil, nil, err
	}

	// create a ClusterRoleBinding in cluster.
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	clusterRoleBinding.Name = clusterRole.Name
	clusterRoleBinding.Subjects = buildRoleBindingSubjects(serviceAccountObj.Name, serviceAccountObj.Namespace)
	clusterRoleBinding.RoleRef = buildClusterRoleReference(clusterRole.Name)
	if _, err = ensureClusterRoleBindingExist(clusterKubeClient, clusterRoleBinding, opts.DryRun); err != nil {
		return nil, nil, err
	}

	if opts.DryRun {
		return nil, nil, nil
	}
	// 使用k8s封装的重试机制进行尝试获取
	clusterSecret, err := util2.WaitForServiceAccountSecretCreation(clusterKubeClient, serviceAccountObj)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get serviceAccount secret from cluster(%s), error: %v", opts.ClusterName, err)
	}

	impersonatorSecret, err := util2.WaitForServiceAccountSecretCreation(clusterKubeClient, impersonationSA)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get serviceAccount secret for impersonation from cluster(%s), error: %v", opts.ClusterName, err)
	}

	return clusterSecret, impersonatorSecret, nil
}

func registerClusterInControllerPlane(opts util2.ClusterRegisterOption, controlPlaneKubeClient kubeclient.Interface) error {
	// ensure namespace where the cluster object be stored exists in control plane.
	// 查看namespace
	if _, err := util2.EnsureNamespaceExist(controlPlaneKubeClient, opts.ClusterNamespace, opts.DryRun); err != nil {
		return err
	}

	// create secret in control plane
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: opts.ClusterNamespace,
			Name:      opts.ClusterName,
		},
		Data: map[string][]byte{
			SecretCADataKey: opts.Secret.Data["ca.crt"],
			SecretTokenKey:  opts.Secret.Data[SecretTokenKey],
		},
	}
	// 1、创建secret，在host集群中创建对应的secret
	secret, err := util2.CreateSecret(controlPlaneKubeClient, secret)
	if err != nil {
		return fmt.Errorf("failed to create secret in control plane. error: %v", err)
	}
	opts.Secret = *secret

	// create secret to store impersonation info in control plane
	impersonatorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: opts.ClusterNamespace,
			Name:      names2.GenerateImpersonationSecretName(opts.ClusterName),
		},
		Data: map[string][]byte{
			SecretTokenKey: opts.ImpersonatorSecret.Data[SecretTokenKey],
		},
	}
	//2、创建impersonatorSecret在 host集群中
	impersonatorSecret, err = util2.CreateSecret(controlPlaneKubeClient, impersonatorSecret)
	if err != nil {
		return fmt.Errorf("failed to create impersonator secret in control plane. error: %v", err)
	}
	opts.ImpersonatorSecret = *impersonatorSecret

	// 创建集群
	cluster, err := generateClusterInControllerPlane(opts)
	if err != nil {
		return err
	}
	patchSecretBody := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, clusterResourceKind),
			},
		},
	}
	err = util2.PatchSecret(controlPlaneKubeClient, impersonatorSecret.Namespace, impersonatorSecret.Name, types.MergePatchType, patchSecretBody)
	if err != nil {
		return fmt.Errorf("failed to patch impersonator secret %s/%s, error: %v", impersonatorSecret.Namespace, impersonatorSecret.Name, err)
	}
	err = util2.PatchSecret(controlPlaneKubeClient, secret.Namespace, secret.Name, types.MergePatchType, patchSecretBody)
	if err != nil {
		return fmt.Errorf("failed to patch secret %s/%s, error: %v", secret.Namespace, secret.Name, err)
	}
	return nil
}

func generateClusterInControllerPlane(opts util2.ClusterRegisterOption) (*clusterv1alpha1.Cluster, error) {
	clusterObj := &clusterv1alpha1.Cluster{}
	clusterObj.Name = opts.ClusterName
	clusterObj.Spec.SyncMode = clusterv1alpha1.Push
	clusterObj.Spec.APIEndpoint = opts.ClusterConfig.Host
	clusterObj.Spec.ID = opts.ClusterID
	clusterObj.Spec.SecretRef = &clusterv1alpha1.LocalSecretReference{
		Namespace: opts.Secret.Namespace,
		Name:      opts.Secret.Name,
	}
	clusterObj.Spec.ImpersonatorSecretRef = &clusterv1alpha1.LocalSecretReference{
		Namespace: opts.ImpersonatorSecret.Namespace,
		Name:      opts.ImpersonatorSecret.Name,
	}

	if opts.ClusterProvider != "" {
		clusterObj.Spec.Provider = opts.ClusterProvider
	}

	if opts.ClusterZone != "" {
		clusterObj.Spec.Zone = opts.ClusterZone
	}

	if opts.ClusterRegion != "" {
		clusterObj.Spec.Region = opts.ClusterRegion
	}

	if opts.ClusterConfig.TLSClientConfig.Insecure {
		clusterObj.Spec.InsecureSkipTLSVerification = true
	}

	if opts.ClusterConfig.Proxy != nil {
		url, err := opts.ClusterConfig.Proxy(nil)
		if err != nil {
			return nil, fmt.Errorf("clusterConfig.Proxy error, %v", err)
		}
		clusterObj.Spec.ProxyURL = url.String()
	}

	//controlPlaneKarmadaClient := karmadaclientset.NewForConfigOrDie(opts.ControlPlaneConfig)
	controlPlaneKarmadaClient, err := dynamic.NewForConfig(opts.ControlPlaneConfig)
	if err != nil {
		panic(err.Error())
	}
	cluster, err := util2.CreateClusterObject(controlPlaneKarmadaClient, clusterObj)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster(%s) object. error: %v", opts.ClusterName, err)
	}

	return cluster, nil
}

// ensureClusterRoleBindingExist makes sure that the specific ClusterRoleBinding exist in cluster.
// If ClusterRoleBinding not exit, just create it.
func ensureClusterRoleBindingExist(client kubeclient.Interface, clusterRoleBinding *rbacv1.ClusterRoleBinding, dryRun bool) (*rbacv1.ClusterRoleBinding, error) {
	if dryRun {
		return clusterRoleBinding, nil
	}

	exist, err := util2.IsClusterRoleBindingExist(client, clusterRoleBinding.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check if ClusterRole exist. ClusterRole: %s, error: %v", clusterRoleBinding.Name, err)
	}
	if exist {
		logrus.Infof("ensure ClusterRole succeed as already exist. ClusterRole: %s", clusterRoleBinding.Name)
		return clusterRoleBinding, nil
	}

	createdObj, err := util2.CreateClusterRoleBinding(client, clusterRoleBinding)
	if err != nil {
		return nil, fmt.Errorf("ensure ClusterRole failed due to create failed. ClusterRole: %s, error: %v", clusterRoleBinding.Name, err)
	}

	return createdObj, nil
}

// ensureClusterRoleExist makes sure that the specific cluster role exist in cluster.
// If cluster role not exit, just create it.
func ensureClusterRoleExist(client kubeclient.Interface, clusterRole *rbacv1.ClusterRole, dryRun bool) (*rbacv1.ClusterRole, error) {
	if dryRun {
		return clusterRole, nil
	}

	exist, err := util2.IsClusterRoleExist(client, clusterRole.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check if ClusterRole exist. ClusterRole: %s, error: %v", clusterRole.Name, err)
	}
	if exist {
		logrus.Infof("ensure ClusterRole succeed as already exist. ClusterRole: %s", clusterRole.Name)
		return clusterRole, nil
	}

	createdObj, err := util2.CreateClusterRole(client, clusterRole)
	if err != nil {
		return nil, fmt.Errorf("ensure ClusterRole failed due to create failed. ClusterRole: %s, error: %v", clusterRole.Name, err)
	}

	return createdObj, nil
}

// buildRoleBindingSubjects will generate a subject as per service account.
// The subject used by RoleBinding or ClusterRoleBinding.
func buildRoleBindingSubjects(serviceAccountName, serviceAccountNamespace string) []rbacv1.Subject {
	return []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      serviceAccountName,
			Namespace: serviceAccountNamespace,
		},
	}
}

// buildClusterRoleReference will generate a ClusterRole reference.
func buildClusterRoleReference(roleName string) rbacv1.RoleRef {
	return rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     roleName,
	}
}
