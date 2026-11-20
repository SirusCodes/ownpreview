package deploy

import (
	"context"
	"fmt"
	"ownpreview/image"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type DeployRequest struct {
	Image    string
	Registry image.Registry
	AppName  string
	Version  string
	AppPort  int32
	Host     string
}

type Deployer interface {
	Deploy(ctx context.Context, req DeployRequest) error
}

type k8sDeployer struct {
	client    *kubernetes.Clientset
	dynClient *dynamic.DynamicClient
	namespace string
}

func NewK8sDeployer(client *kubernetes.Clientset, namespace string, dynClient *dynamic.DynamicClient) Deployer {
	return &k8sDeployer{client: client, namespace: namespace, dynClient: dynClient}
}

func (kd *k8sDeployer) Deploy(ctx context.Context, req DeployRequest) error {
	appName := fmt.Sprintf("%s-%s", req.AppName, req.Version)
	labels := map[string]string{"app": appName, "ownpreview.io/version": req.Version}

	deployment := &appsv1.Deployment{
		Name:      appName,
		Namespace: kd.namespace,
		Labels:    labels,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(0)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				Labels: labels,
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-creds"}},
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: req.Image,
							Ports: []corev1.ContainerPort{{ContainerPort: req.AppPort}},
							ReadinessProbe: &corev1.Probe{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromInt32(req.AppPort),
								},
								InitialDelaySeconds: 1,
								PeriodSeconds:       2,
							},
						},
					},
				},
			},
		},
	}

	if _, err := kd.client.AppsV1().Deployments(kd.namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	service := &corev1.Service{
		Name:      appName,
		Namespace: kd.namespace,
		Labels:    labels,
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(req.AppPort),
				},
			},
		},
	}

	if _, err := kd.client.CoreV1().Services(kd.namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	kedaMetadata := map[string]any{
		"name":      appName,
		"namespace": kd.namespace,
		"labels":    labels,
	}

	interceptorRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "http.keda.sh/v1beta1",
			"kind":       "InterceptorRoute",
			"metadata":   kedaMetadata,
			"spec": map[string]any{
				"target": map[string]any{
					"service": appName,
					"port":    int64(80),
				},
				"rules": []any{
					map[string]any{
						"hosts": []any{req.Host},
					},
				},
				"scalingMetric": map[string]any{
					"requestRate": map[string]any{
						"targetValue": int64(100),
						"window":      "1m",
						"granularity": "1s",
					},
				},
			},
		},
	}

	irGVR := schema.GroupVersionResource{
		Group:    "http.keda.sh",
		Version:  "v1beta1",
		Resource: "interceptorroutes",
	}

	if _, err := kd.dynClient.Resource(irGVR).Namespace(kd.namespace).Create(ctx, interceptorRoute, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create InterceptorRoute (v1beta1): %w", err)
	}

	scaledObject := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata":   kedaMetadata,
			"spec": map[string]any{
				"scaleTargetRef": map[string]any{
					"name": appName,
				},
				"minReplicaCount": int64(0),
				"maxReplicaCount": int64(2),
				"cooldownPeriod":  int64(300), // Scales back to 0 after 5 minutes of no traffic
				"triggers": []any{
					map[string]any{
						"type": "external",
						"metadata": map[string]any{
							"scalerAddress":    "keda-add-ons-http-external-scaler.keda.svc.cluster.local:9090",
							"interceptorRoute": appName,
						},
					},
				},
			},
		},
	}

	soGVR := schema.GroupVersionResource{
		Group:    "keda.sh",
		Version:  "v1alpha1",
		Resource: "scaledobjects",
	}

	if _, err := kd.dynClient.Resource(soGVR).Namespace(kd.namespace).Create(ctx, scaledObject, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create ScaledObject: %w", err)
	}

	return nil
}
