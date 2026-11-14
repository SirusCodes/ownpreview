package image

import (
	"context"
	"fmt"
	"ownpreview/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type BuildRequest struct {
	Source Source
	Image  string
}

type ImageBuilder interface {
	Build(ctx context.Context, req BuildRequest) error
}

type k8sBuilder struct {
	client    *kubernetes.Clientset
	namespace string
}

func NewK8sBuilder(client *kubernetes.Clientset, namespace string) ImageBuilder {
	return &k8sBuilder{
		client:    client,
		namespace: namespace,
	}
}

func (b *k8sBuilder) Build(ctx context.Context, req BuildRequest) error {
	job, err := createJobSpec(req.Image, req.Source)
	if err != nil {
		return err
	}

	_, err = b.client.BatchV1().Jobs(b.namespace).Create(ctx, &job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %v", err)
	}

	return nil
}

func createJobSpec(targetImage string, builderSource Source) (batchv1.Job, error) {
	jobName := func(val string) string {
		hash, _ := utils.SmallAlphaNumericHash(val, 8)
		return fmt.Sprintf("container-build-job-%s", hash)
	}

	if gitSource, ok := builderSource.(*GitSource); ok {
		return createGitJobSpec(jobName(gitSource.URL+gitSource.Ref), targetImage, gitSource), nil
	}

	return batchv1.Job{}, fmt.Errorf("unsupported builder source type")
}

func createGitJobSpec(jobName, targetImage string, gitSource *GitSource) batchv1.Job {
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"container.apparmor.security.beta.kubernetes.io/buildkit": "unconfined",
					},
				},
				Spec: corev1.PodSpec{
					// 1. The Init Container runs first to clone the code
					InitContainers: []corev1.Container{
						{
							Name:  "git-clone",
							Image: "alpine/git:latest",
							// In a real app, you'd check out the specific PR branch here
							Command: []string{"sh", "-c"},
							Args: []string{
								fmt.Sprintf(
									"git init /workspace && cd /workspace && git remote add origin %s && git fetch --depth=1 origin %s && git checkout FETCH_HEAD",
									gitSource.URL,
									gitSource.Ref,
								),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace-volume",
									MountPath: "/workspace",
								},
							},
						},
					},
					// 2. The Main Container runs after the Init Container finishes
					Containers: []corev1.Container{
						{
							Name:  "buildkit",
							Image: "moby/buildkit:master-rootless",
							Env: []corev1.EnvVar{
								{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
							},
							Command: []string{"buildctl-daemonless.sh"},
							Args: []string{
								"build",
								"--frontend", "dockerfile.v0",
								"--local", "context=/workspace", // Pointing to the shared volume
								"--local", "dockerfile=/workspace", // Pointing to the shared volume
								"--output", fmt.Sprintf("type=image,name=%s,push=true", targetImage),
							},
							SecurityContext: &corev1.SecurityContext{
								SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
								RunAsUser:      func(i int64) *int64 { return &i }(1000),
								RunAsGroup:     func(i int64) *int64 { return &i }(1000),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace-volume",
									MountPath: "/workspace",
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					// 3. Define the shared volumes
					Volumes: []corev1.Volume{
						{
							Name: "workspace-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{}, // Ephemeral storage just for this Pod
							},
						},
					},
				},
			},
		},
	}
}
