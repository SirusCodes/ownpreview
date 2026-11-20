package image

import (
	"context"
	"log/slog"
	"ownpreview/utils"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Registry interface {
	Setup(context.Context) error
	GetRegistryURL() string
	RemoveImage(context.Context, string) error
	IsInsecureConnection() bool
}

type k8sRegistry struct {
	registryURL string
	client      *kubernetes.Clientset
	namespace   string
	logger      *slog.Logger
}

const k8sRegistryURL = "registry.default.svc.cluster.local:5000"

func NewK8sRegistry(client *kubernetes.Clientset, namespace string) Registry {
	return &k8sRegistry{
		registryURL: k8sRegistryURL,
		client:      client,
		namespace:   namespace,
		logger:      utils.NewLogger("registry"),
	}
}

func (r *k8sRegistry) GetRegistryURL() string {
	return r.registryURL
}

func (r *k8sRegistry) IsInsecureConnection() bool {
	return true
}

func (r *k8sRegistry) Setup(ctx context.Context) error {
	log := r.logger.With("namespace", r.namespace)
	log.DebugContext(ctx, "setting up in-cluster registry")

	labels := map[string]string{"app": "registry"}

	secret := &corev1.Secret{
		Name:      "registry-creds",
		Namespace: r.namespace,
		Type:      corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
		},
	}

	if _, err := r.client.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.DebugContext(ctx, "registry credentials secret already exists")
	} else {
		log.DebugContext(ctx, "registry credentials secret created")
	}

	// 1. Equivalent to: kubectl create deployment registry --image=registry:2
	dirOrCreate := corev1.HostPathDirectoryOrCreate
	deployment := &appsv1.Deployment{
		Name:      "registry",
		Namespace: r.namespace,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
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
								{
									ContainerPort: 5000,
									HostPort:      5000,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "registry-storage",
									MountPath: "/var/lib/registry",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "registry-storage",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/ownpreview/registry",
									Type: &dirOrCreate,
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := r.client.AppsV1().Deployments(r.namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.DebugContext(ctx, "registry deployment already exists")
	} else {
		log.DebugContext(ctx, "registry deployment created")
	}

	// 2. Equivalent to: kubectl expose deployment registry --port=5000 --target-port=5000
	service := &corev1.Service{
		Name:      "registry",
		Namespace: r.namespace,
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

	if _, err := r.client.CoreV1().Services(r.namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.DebugContext(ctx, "registry service already exists")
	} else {
		log.DebugContext(ctx, "registry service created")
	}

	log.DebugContext(ctx, "container registry is ready", "registry_url", r.registryURL)
	return nil
}

func (r *k8sRegistry) RemoveImage(ctx context.Context, name string) error {
	panic("Not implemented")
}
