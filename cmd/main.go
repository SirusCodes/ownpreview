package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"ownpreview/image"
	"ownpreview/k8s"
	"path/filepath"

	"k8s.io/client-go/util/homedir"
)

type flags struct {
	kubeconfig string
	namespace  string
	gitURL     string
	gitRef     string
	project    string
}

func parseFlags() *flags {
	kubeconfig := flag.String("kubeconfig", "", "(optional)	absolute path to the kubeconfig file")
	namespace := flag.String("namespace", "default", "Kubernetes namespace to use (default: default)")
	gitURL := flag.String("git-url", "", "Git repository URL to build the container from")
	gitRef := flag.String("git-ref", "main", "Git reference (branch, tag, or commit) to build the container from (default: main)")
	project := flag.String("project", "my-project", "Project name for the container build (default: my-project)")

	flag.Parse()

	if *kubeconfig == "" {
		home := homedir.HomeDir()
		if home == "" {
			fmt.Println("error: unable to determine home directory")
			os.Exit(1)
		}
		*kubeconfig = filepath.Join(home, ".kube", "config")
	}

	if *gitURL == "" {
		fmt.Println("error: git-url is required")
		os.Exit(1)
	}

	return &flags{
		kubeconfig: *kubeconfig,
		namespace:  *namespace,
		gitURL:     *gitURL,
		gitRef:     *gitRef,
		project:    *project,
	}
}

func main() {
	f := parseFlags()

	if _, err := os.Stat(f.kubeconfig); os.IsNotExist(err) {
		fmt.Printf("error: kubeconfig file not found at %s", f.kubeconfig)
		os.Exit(1)
	}

	k8sClient, err := k8s.GetClient(&f.kubeconfig)
	if err != nil {
		fmt.Printf("error: failed to create Kubernetes client: %v", err)
		os.Exit(1)
	}

	rg := image.NewK8sRegistry()
	if err := rg.Setup(context.Background(), k8sClient, f.namespace); err != nil {
		fmt.Printf("error: failed to setup registry: %v", err)
		os.Exit(1)
	}

	builder := image.NewK8sBuilder(k8sClient, f.namespace)
	buildRequest := image.BuildRequest{
		Source: &image.GitSource{
			URL: f.gitURL,
			Ref: f.gitRef,
		},
		ImageName:    f.project,
		Registry: rg,
	}

	if err := builder.Build(context.Background(), buildRequest); err != nil {
		fmt.Printf("error: failed to build container: %v", err)
		os.Exit(1)
	}

	fmt.Println("Container build job created successfully.")
}
