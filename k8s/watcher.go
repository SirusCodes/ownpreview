package k8s

import (
	"context"
	"fmt"

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

			// Print live pod progress
			target := int32(1)
			if job.Spec.Completions != nil {
				target = *job.Spec.Completions
			}

			if job.Status.Succeeded != lastSucceeded || target != lastTarget {
				fmt.Printf("Building image....: %d/%d\n", job.Status.Succeeded, target)
				lastSucceeded = job.Status.Succeeded
				lastTarget = target
			}

			// Check for terminal conditions
			for _, cond := range job.Status.Conditions {
				if cond.Status != corev1.ConditionTrue {
					continue
				}
				if cond.Type == batchv1.JobComplete {
					fmt.Println("Job completed successfully.")
					return nil
				}
				if cond.Type == batchv1.JobFailed {
					return fmt.Errorf("job failed: %s (%s)", cond.Reason, cond.Message)
				}
			}
		}
	}
}
