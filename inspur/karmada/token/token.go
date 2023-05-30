package main

import (
	"context"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"ranzhouol/k8s_study/inspur/karmada/util"
)

const (
	// karmadaSecretName is the name of the secret on karmada host platform
	karmadaSecretName = "karmada-dashboard-token"

	// karmadaSecretNamespace is the name of the Namespace in which secret resides
	karmadaSecretNamespace = "karmada-system"

	// karmadaServiceAccount is the name of the karmada-dashboard's ServiceAccount
	karmadaServiceAccount = "karmada-dashboard"
)

func main() {
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

	err = createKarmadaToken(karmadaConfig, config)
	if err != nil {
		logrus.Error(err.Error())
	}
}

func createKarmadaToken(karmadaConfig, config *rest.Config) error {
	controlPlaneKubeClient := kubeclient.NewForConfigOrDie(karmadaConfig)
	clusterKubeClient := kubeclient.NewForConfigOrDie(config)

	// 判断secret是否存在
	ok, err := util.IfSecretExists(clusterKubeClient, karmadaSecretNamespace, karmadaSecretName)
	if err != nil {
		return err
	}
	if ok {
		logrus.Info("karmadaHost secret 已经存在")
		return nil
	}

	// 获取karmada控制平台上的secret
	serviceAccount, err := controlPlaneKubeClient.CoreV1().ServiceAccounts(karmadaSecretNamespace).Get(context.TODO(), karmadaServiceAccount, metav1.GetOptions{})
	if err != nil {
		return err
	}
	karmadaControlPlaneSecret, err := util.GetTargetSecret(controlPlaneKubeClient, serviceAccount.Secrets, corev1.SecretTypeServiceAccountToken, karmadaSecretNamespace)
	if err != nil {
		return err
	}

	karmadaHostPlaneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      karmadaSecretName,
			Namespace: karmadaSecretNamespace,
		},
		Data: map[string][]byte{
			"token": karmadaControlPlaneSecret.Data["token"],
		},
	}
	logrus.Infof("在 karmada Host 平面创建secret")
	_, err = util.CreateSecret(clusterKubeClient, karmadaHostPlaneSecret)
	if err != nil {
		return err
	}
	logrus.Infof("secret: %v 创建成功", karmadaSecretName)
	return nil
}
