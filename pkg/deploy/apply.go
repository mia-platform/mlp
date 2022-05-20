package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/mia-platform/mlp/pkg/resourceutil"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

type applyFunction func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error

const awaitCompletionAnnotation = "mia-platform.eu/await-completion"

var decoratedApply = withAwaitableResource(apply)

func withAwaitableResource(apply applyFunction) applyFunction {
	return func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
		gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, res.Object.GroupVersionKind())
		if err != nil {
			return err
		}

		// register a watcher and starts to listen for events for the gvr
		// if res is annotated with awaitCompletionAnnotation
		var watchEvents <-chan watch.Event
		startTime := time.Now()
		awaitCompletionValue, awaitCompletionFound := res.Object.GetAnnotations()[awaitCompletionAnnotation]
		if awaitCompletionFound {
			watcher, err := clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Watch(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			watchEvents = watcher.ResultChan()
			fmt.Printf("Registered a watcher for resource: %s.%s.%s having name %s\n", gvr.Group, gvr.Version, gvr.Resource, res.Object.GetName())
		}

		// actually apply the resource
		if err := apply(clients, res, deployConfig); err != nil {
			return err
		}

		// return if no event channel has been set
		if watchEvents == nil {
			return nil
		}

		// parse timeout from annotation value
		timeout, err := time.ParseDuration(awaitCompletionValue)
		if err != nil {
			msg := fmt.Sprintf("Error in %s annotation value: must be a valid duration", awaitCompletionAnnotation)
			return errors.Wrap(err, msg)
		}

		// check if res can be handled
		if _, err := handleResourceCompletionEvent(&res, nil, startTime); err != nil {
			return err
		}

		// consume watcher events and wait for the resource to complete or exit because of timeout
		for {
			select {
			case event := <-watchEvents:
				isCompleted, err := handleResourceCompletionEvent(&res, &event, startTime)
				if err != nil {
					msg := "Error while watching resource events"
					return errors.Wrap(err, msg)
				}

				if isCompleted {
					return nil
				}
			case <-time.NewTimer(timeout).C:
				msg := fmt.Sprintf("Timeout received while waiting for job %s completion", res.Object.GetName())
				return errors.New(msg)
			}
		}
	}
}

// handleResourceCompletionEvent takes the target resource, the watch event and
// the initial watch time as arguments. It returns (true, nil) when the given
// resource has completed in the given event. If the event is nil returns (false, nil)
// when the resource supports events watching otherwise returns (false, error).
func handleResourceCompletionEvent(res *resourceutil.Resource, event *watch.Event, startTime time.Time) (bool, error) {
	switch res.GroupVersionKind.Kind {
	case "Job":
		if event == nil {
			return false, nil
		}

		u := event.Object.(*unstructured.Unstructured)
		var job batchv1.Job
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &job); err != nil {
			return false, err
		}

		if job.Name != res.Object.GetName() {
			return false, nil
		}

		if completedAt := job.Status.CompletionTime; event.Type == watch.Modified && completedAt != nil && completedAt.Time.After(startTime) {
			fmt.Println("Job completed:", res.Object.GetName())
			return true, nil
		}

		return false, nil
	default:
		msg := fmt.Sprintf("No watch handler for resource %s.%s.%s having name %s", res.GroupVersionKind.Group, res.GroupVersionKind.Version, res.GroupVersionKind.Kind, res.Object.GetName())
		return false, errors.New(msg)
	}
}

func apply(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {

	gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, res.Object.GroupVersionKind())
	if err != nil {
		return err
	}

	var onClusterObj *unstructured.Unstructured
	if onClusterObj, err = clients.dynamic.Resource(gvr).
		Namespace(res.Object.GetNamespace()).
		Get(context.Background(), res.Object.GetName(), metav1.GetOptions{}); err != nil {
		// create the resource only if it is not present in the cluster
		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			// creates kubectl.kubernetes.io/last-applied-configuration annotation
			// inside the resource except for Secrets and ConfigMaps
			if res.Object.GetKind() != "Secret" && res.Object.GetKind() != "ConfigMap" {
				orignAnn := res.Object.GetAnnotations()
				if orignAnn == nil {
					orignAnn = make(map[string]string)
				}
				objJson, err := res.Object.MarshalJSON()
				if err != nil {
					return err
				}
				orignAnn[corev1.LastAppliedConfigAnnotation] = string(objJson)
				res.Object.SetAnnotations(orignAnn)
			}

			if err = cronJobAutoCreate(clients.dynamic, &res.Object); err != nil {
				return err
			}

			_, err = clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Create(context.Background(),
					&res.Object,
					metav1.CreateOptions{})
		}
		return err
	}

	// Do not modify the resource if is already present on cluster and the annotation is set to "once"
	if res.Object.GetAnnotations()[resourceutil.GetMiaAnnotation("deploy")] != "once" {

		// Replace only if it is a Secret or configmap otherwise patch the resource
		if res.Object.GetKind() == "Secret" || res.Object.GetKind() == "ConfigMap" {
			fmt.Printf("Replacing %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			_, err = clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Update(context.Background(),
					&res.Object,
					metav1.UpdateOptions{})

		} else {

			fmt.Printf("Updating %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			if deployConfig.DeployType == smartDeploy && (res.Object.GetKind() == "CronJob" || res.Object.GetKind() == "Deployment") {
				isNotUsingSemver, err := resourceutil.IsNotUsingSemver(&res)
				if err != nil {
					return errors.Wrap(err, "failed semver check")
				}

				if deployConfig.ForceDeployWhenNoSemver && isNotUsingSemver {
					if err := ensureDeployAll(&res, time.Now()); err != nil {
						return errors.Wrap(err, "failed ensure deploy all on resource not using semver")
					}
				} else {
					if err = ensureSmartDeploy(onClusterObj, &res); err != nil {
						return errors.Wrap(err, "failed smart deploy ensure")
					}
				}
			}

			if res.Object.GetKind() == "CronJob" {
				if err := checkIfCreateJob(clients.dynamic, onClusterObj, res); err != nil {
					return errors.Wrap(err, "failed check if create job")
				}
			}

			patch, patchType, err := createPatch(*onClusterObj, res)

			// create the patch
			if err != nil {
				return errors.Wrap(err, "failed to create patch")
			}

			if _, err := clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Patch(context.Background(),
					res.Object.GetName(), patchType, patch, metav1.PatchOptions{}); err != nil {
				return errors.Wrap(err, "failed to patch")
			}
		}
		return err
	}
	return nil
}
