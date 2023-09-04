package main

import (
	"fmt"
	"github.com/gorilla/mux"
	tokenutil "github.com/karmada-io/karmada/pkg/karmadactl/util/bootstraptoken"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"strings"
	"time"
)

type CommandTokenOptions struct {
	TTL                  *metav1.Duration // 令牌失效时间
	Description          string
	Groups               []string
	Usages               []string
	PrintRegisterCommand bool
	parentCommand        string // kubectl karmada 或 karmadactl
}

func (o *CommandTokenOptions) runCreateToken(kubeconfig string, client kubeclient.Interface) (string, error) {
	fmt.Println("creating token")
	bootstrapToken, err := tokenutil.GenerateRandomBootstrapToken(o.TTL, o.Description, o.Groups, o.Usages)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	if err := tokenutil.CreateNewToken(client, bootstrapToken); err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	tokenStr := bootstrapToken.Token.ID + "." + bootstrapToken.Token.Secret

	// if --print-register-command was specified, print a machine-readable full `karmadactl register` command
	// otherwise, just print the token
	if o.PrintRegisterCommand {
		joinCommand, err := tokenutil.GenerateRegisterCommand(kubeconfig,
			o.parentCommand, tokenStr, "")
		if err != nil {
			fmt.Println(err.Error())
			return "", fmt.Errorf("failed to get register command, err: %w", err)
		}
		fmt.Println(joinCommand)
		return joinCommand, nil
	} else {
		fmt.Println(tokenStr)
		return tokenStr, nil
	}
}

// GenerateRegisterCommand generate register command that will be printed
func GenerateRegisterCommand() {

}

func NewCmdTokenCreate(kubeconfig string, tokenOpts *CommandTokenOptions) (string, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	return tokenOpts.runCreateToken(kubeconfig, client)

}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/multicluster/cluster.karmada.io/v1alpha1/clusters/pull", HomeHandler).Methods("GET")
	fmt.Println("http server starting at 3000 ...")
	http.ListenAndServe(":3000", r)
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method)
	if r.Method == "GET" && strings.Contains(r.RequestURI, "/multicluster/cluster.karmada.io/v1alpha1/clusters/pull") {
		opts := &CommandTokenOptions{
			TTL: &metav1.Duration{
				Duration: 24 * time.Hour,
			},
			// 令牌要认证为的额外组，必须以 "system:bootstrappers:" 开头
			Groups: []string{"system:bootstrappers:karmada:default-cluster-token"},
			// 启动 usage-bootstrap-authentication和 usage-bootstrap-signing
			Usages:               []string{"signing", "authentication"},
			PrintRegisterCommand: true,
			parentCommand:        "kubectl karmada", // 或karmadactl
		}
		//kubeconfig := "D:\\Go\\Go_WorkSpace\\src\\inspur.com\\linux\\5174\\karmada-apiserver.config" //用不了
		kubeconfig := "D:\\Go\\Go_WorkSpace\\src\\inspur.com\\linux\\4970\\karmada-apiserver.config"
		command, err := NewCmdTokenCreate(kubeconfig, opts)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		_, err = w.Write([]byte(command))
		if err != nil {
			fmt.Println(err.Error())
		}

	}
}
