package image

import (
	"context"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Registry interface {
	Setup(ctx context.Context, client *kubernetes.Clientset, namespace string) error
	GetRegistryURL() string
	InsecureConnection() bool
}

type K8sRegistry struct {
	registryURL string
}

const k8sRegistryURL = "registry.default.svc.cluster.local:5000"

func NewK8sRegistry() Registry {
	return &K8sRegistry{
		registryURL: k8sRegistryURL,
	}
}

func (r *K8sRegistry) GetRegistryURL() string {
	return r.registryURL
}

func (r *K8sRegistry) InsecureConnection() bool {
	return true
}

func (r *K8sRegistry) Setup(ctx context.Context, client *kubernetes.Clientset, namespace string) error {
	labels := map[string]string{"app": "registry"}
	replicas := int32(1)

	secret := &corev1.Secret{
		Name:      "registry-creds",
		Namespace: namespace,
		Type:      corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
		},
	}

	if _, err := client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); apierrors.IsAlreadyExists(err) {
		return nil
	}

	// 1. Equivalent to: kubectl create deployment registry --image=registry:2
	deployment := &appsv1.Deployment{
		Name:      "registry",
		Namespace: namespace,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "registry",
							Image: "registry:2",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 5000},
							},
						},
					},
				},
			},
		},
	}

	if _, err := client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// 2. Equivalent to: kubectl expose deployment registry --port=5000 --target-port=5000
	service := &corev1.Service{
		Name:      "registry",
		Namespace: namespace,
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       5000,
					TargetPort: intstr.FromInt32(5000),
				},
			},
		},
	}

	if _, err := client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
