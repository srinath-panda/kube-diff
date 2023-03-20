package scaledobject

import (
	"context"
	"fmt"
	"kube-diff/util"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func UpdateScaledObject(src_config, destConfigFile string, workersMap map[string]bool) {
	gvr := schema.GroupVersionResource{Version: "v1alpha1", Resource: "scaledobjects", Group: "keda.sh"}

	dstDynamicClient := getK8sMetadata(destConfigFile)
	srcDynamicClient := getK8sMetadata(src_config)
	dstResources, err := dstDynamicClient.Resource(gvr).Namespace("default").List(context.Background(), v1.ListOptions{})
	util.TryPanic(err)

	for _, scaledObject := range dstResources.Items {

		soObject, _ := dstDynamicClient.Resource(gvr).Namespace("default").Get(context.Background(), scaledObject.GetName(), v1.GetOptions{})
		so := soObject.DeepCopy()

		appName := util.GetUnstructuredObjectNestedVal(*so, true, "metadata", "labels", "app").(string)
		if _, ok := workersMap[appName]; ok {
			fmt.Printf("Skipping ScaledObject %s, since its a worker", so.GetName())
			fmt.Println()
			continue
		}

		replica, err := getDeployReplica(srcDynamicClient, so.GetName())
		util.TryPanic(err)

		val := util.GetUnstructuredObjectNestedVal(*so, false, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")

		if val != nil {
			if replica > 0 {
				fmt.Printf("Unpause ScaledObject %s", so.GetName())
				fmt.Println()

				err = unstructured.SetNestedField(so.Object, nil, "metadata", "annotations")
				util.TryPanic(err)
				_, err = dstDynamicClient.Resource(gvr).Namespace("default").Update(context.Background(), so, v1.UpdateOptions{})
				util.TryPanic(err)
			} else {
				fmt.Printf("Skipping ScaledObject %s since replica is %v ", so.GetName(), replica)
				fmt.Println()
			}
		} else {
			fmt.Printf("Skipping ScaledObject %s, since its was already unpaused", so.GetName())
			fmt.Println()
		}

	}

}

func getDeployReplica(dstDynamicClient *dynamic.DynamicClient, name string) (int64, error) {
	gvr := schema.GroupVersionResource{Version: "v1", Group: "apps", Resource: "deployments"}
	deploy, err := dstDynamicClient.Resource(gvr).Namespace("default").Get(context.Background(), name, v1.GetOptions{})
	util.TryPanic(err)
	return util.GetUnstructuredObjectNestedVal(*deploy, true, "spec", "replicas").(int64), nil
}

func getK8sMetadata(k8s_configFile string) *dynamic.DynamicClient {
	config, err := clientcmd.BuildConfigFromFlags("", k8s_configFile)
	util.TryPanic(err)
	config.QPS = 10
	config.Burst = 20
	dynamicClientSet, err := dynamic.NewForConfig(config)
	util.TryPanic(err)
	return dynamicClientSet
}
