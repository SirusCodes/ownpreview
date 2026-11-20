package k8s

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClient(configPath *string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot find config - %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cannot create clientset - %v", err)
	}

	return clientset, nil
}

func GetDynamicClient(configPath *string) (*dynamic.DynamicClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot find config - %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cannot create dynamic clientset - %v", err)
	}

	return dynClient, nil
}
