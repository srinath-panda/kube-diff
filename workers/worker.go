package wrokers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"kube-diff/util"
	"os"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

var workers_list map[string]bool

func SetWorkers(workerfile string) {
	if workerfile == "" {
		util.TryPanic(errors.New("workerslist is empty"))
	}

	file, err := os.Open(workerfile)
	util.TryPanic(err)
	defer file.Close()
	workers_list = make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		str := scanner.Text()
		appName := strings.Replace(str, " ", "", -1)

		if _, ok := workers_list[appName]; !ok {
			workers_list[appName] = true
		}
	}
}

func GetWorkers() map[string]bool {

	if len(workers_list) == 0 {
		util.TryPanic(errors.New("SetWorkers method was not called"))
	}
	return workers_list
}

func EnableWorkers(srcClusterClient, dstClusterClient *kubernetes.Clientset) {

	for app := range GetWorkers() {
		selector := fmt.Sprintf("app=%s", app)
		deployments, _ := srcClusterClient.AppsV1().Deployments("default").List(context.Background(), v1.ListOptions{LabelSelector: selector})

		for _, srcdeploy := range deployments.Items {
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return UpdateReplicasForDeploy(srcdeploy.Name, *srcdeploy.Spec.Replicas, dstClusterClient)
			})
			util.TryPanic(retryErr)

			retryErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return UpdateReplicasForDeploy(srcdeploy.Name, 0, dstClusterClient)
			})
			util.TryPanic(retryErr)
		}
	}
}

func UpdateReplicasForDeploy(deployName string, replica int32, ClusterClient *kubernetes.Clientset) error {
	deployment, err := ClusterClient.AppsV1().Deployments("default").Get(context.Background(), deployName, v1.GetOptions{})
	util.TryPanic(err)
	deployment.Spec.Replicas = int32Ptr(replica)
	_, err = ClusterClient.AppsV1().Deployments("default").Update(context.Background(), deployment, v1.UpdateOptions{})
	return err
}

func int32Ptr(i int32) *int32 { return &i }
