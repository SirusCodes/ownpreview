package k8s

import (
	"context"
	"fmt"
	"ownpreview/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

func WatchJobProgress(ctx context.Context, client *kubernetes.Clientset, namespace, jobName string) error {
	watcher, err := client.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", jobName),
	})
	if err != nil {
		return fmt.Errorf("failed to start job watch: %w", err)
	}
	defer watcher.Stop()

	log := utils.NewLogger("watcher").With("job", jobName, "namespace", namespace)
	log.DebugContext(ctx, "started watching job progress")

	lastSucceeded, lastTarget := int32(-1), int32(-1)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed unexpectedly")
			}

			if event.Type == watch.Deleted {
				return fmt.Errorf("job %s was deleted", jobName)
			}

			job, ok := event.Object.(*batchv1.Job)
			if !ok {
				continue
			}

			target := int32(1)
			if job.Spec.Completions != nil {
				target = *job.Spec.Completions
			}

			if job.Status.Succeeded != lastSucceeded || target != lastTarget {
				log.InfoContext(ctx, "build progress", "succeeded", job.Status.Succeeded, "target", target)
				lastSucceeded = job.Status.Succeeded
				lastTarget = target
			}

			// Check for terminal conditions
			for _, cond := range job.Status.Conditions {
				if cond.Status != corev1.ConditionTrue {
					continue
				}
				if cond.Type == batchv1.JobComplete {
					log.DebugContext(ctx, "build job completed successfully")
					return nil
				}
				if cond.Type == batchv1.JobFailed {
					return fmt.Errorf("job failed: %s (%s)", cond.Reason, cond.Message)
				}
			}
		}
	}
}
