package check

import (
	"context"
	"fmt"
	"kube-diff/util"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func getK8sMetadata(k8s_configFile string) *dynamic.DynamicClient {
	config, err := clientcmd.BuildConfigFromFlags("", k8s_configFile)
	util.TryPanic(err)
	config.QPS = 10
	config.Burst = 20
	dynamicClientSet, err := dynamic.NewForConfig(config)
	util.TryPanic(err)
	return dynamicClientSet
}

type Differences struct {
	name       string
	DiffinSRC  []string
	DiffinDest []string
}

func CheckCluster(srcConfigFile string, destConfigFile string, workers map[string]bool) {
	workersMap = workers

	srcDynamicClient := getK8sMetadata(srcConfigFile)
	dstDynamicClient := getK8sMetadata(destConfigFile)

	finalOp := make(map[string]Differences)

	//Missing reources
	for _, gvr := range getGVRs() {
		srcResources, err := srcDynamicClient.Resource(gvr).Namespace("default").List(context.Background(), v1.ListOptions{})
		util.TryPanic(err)

		dstResources, err := dstDynamicClient.Resource(gvr).Namespace("default").List(context.Background(), v1.ListOptions{})
		util.TryPanic(err)

		finalOp[fmt.Sprintf("missing %s", gvr.Resource)] = findMissing(gvr.Resource, ToMap(srcResources.Items), ToMap(dstResources.Items))

		if strings.EqualFold(gvr.Resource, "deployments") {
			//Check Replicas
			finalOp[fmt.Sprintf("UnMatched replicas %s", gvr.Resource)] = checkReplicas(ToMap(srcResources.Items), ToMap(dstResources.Items))
			//check DH_tags, image, replicas
			finalOp[fmt.Sprintf("Unmatched DH specific labels/Annotations/Image %s", gvr.Resource)] = CheckDepoySpec(ToMap(srcResources.Items), ToMap(dstResources.Items))
		}
		if strings.EqualFold(gvr.Resource, "cronjobs") {
			diff1, diff2 := CheckCronSpec(ToMap(srcResources.Items), ToMap(dstResources.Items))
			finalOp[fmt.Sprintf("UnMatched Cronjob Job status %s", gvr.Resource)] = diff1
			finalOp[fmt.Sprintf("Unmatched DH specific labels/Annotations/Image %s", gvr.Resource)] = diff2
		}
	}
	worker, nonworker := checkscaledobjects(dstDynamicClient)
	finalOp["Paused ScaledObject for workers in Dest cluster"] = worker
	finalOp["Paused ScaledObject for NON-WORKERS in Dest cluster"] = nonworker
	printOP(finalOp)
}

func CheckCronSpec(srcResources, destResources map[string]unstructured.Unstructured) (Differences, Differences) {
	diff := Differences{}
	diff.name = "Cron Spec"

	diff2 := Differences{}
	diff2.name = "Cron Spec"

	for srcResName, srcRes := range srcResources {
		if destRes, ok := destResources[srcResName]; ok {
			srcMinfo, err := util.GetMirroSpecForUnstructuredForCron(srcRes)
			if err {
				panic("Cannot Create Mirror Object for src")
			}
			destMinfo, err := util.GetMirroSpecForUnstructuredForCron(destRes)
			if err {
				panic("Cannot Create Mirror Object for dest")
			}

			if !srcMinfo.Equals(destMinfo, false) {
				diff.DiffinDest = append(diff.DiffinDest, srcResName)
			}
		}
	}
	return diff, diff2
}

var workersMap map[string]bool

func checkscaledobjects(dstDynamicClient *dynamic.DynamicClient) (Differences, Differences) {
	gvr := schema.GroupVersionResource{Version: "v1alpha1", Resource: "scaledobjects", Group: "keda.sh"}
	dstResources, err := dstDynamicClient.Resource(gvr).Namespace("default").List(context.Background(), v1.ListOptions{})
	util.TryPanic(err)

	workers := Differences{}
	nonworkers := Differences{}
	workers.DiffinDest = make([]string, 0)
	nonworkers.DiffinDest = make([]string, 0)
	workers.name = "Paused ScaledObject for workers in Dest cluster"
	nonworkers.name = "Paused ScaledObject for NON-WORKERS in Dest cluster"

	for _, res := range ToMap(dstResources.Items) {
		val := util.GetUnstructuredObjectNestedVal(res, false, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
		appName := util.GetUnstructuredObjectNestedVal(res, true, "metadata", "labels", "app").(string)

		if val != nil {
			if _, ok := workersMap[appName]; ok {
				workers.DiffinDest = append(workers.DiffinDest, fmt.Sprintf("%v, repo: %v", res.GetName(), util.GetRepoFromunstructured(res)))

			} else {
				nonworkers.DiffinDest = append(nonworkers.DiffinDest, fmt.Sprintf("%v, repo: %v", res.GetName(), util.GetRepoFromunstructured(res)))
			}
		}
	}
	return workers, nonworkers
}

func CheckDepoySpec(srcResources, destResources map[string]unstructured.Unstructured) Differences {
	diff := Differences{}
	diff.name = "deployment Spec"

	for srcResName, srcRes := range srcResources {
		if destRes, ok := destResources[srcResName]; ok {
			srcMinfo, err := util.GetMirroSpecForUnstructured(srcRes)
			if err {
				panic("Cannot Create Mirror Object for src")
			}
			destMinfo, err := util.GetMirroSpecForUnstructured(destRes)
			if err {
				panic("Cannot Create Mirror Object for dest")
			}

			if !srcMinfo.Equals(destMinfo, false) {
				diff.DiffinDest = append(diff.DiffinDest, srcResName)
			}
		}
	}
	return diff
}

func checkReplicas(srcResources, destResources map[string]unstructured.Unstructured) Differences {
	diff := Differences{}
	diff.name = "deployment replicas"
	for srcResName, srcRes := range srcResources {

		if destRes, ok := destResources[srcResName]; ok {
			srcreplicas := util.GetUnstructuredObjectNestedVal(srcRes, true, "spec", "replicas").(int64)
			destreplicas := util.GetUnstructuredObjectNestedVal(destRes, true, "spec", "replicas").(int64)
			if srcreplicas != destreplicas {
				diff.DiffinDest = append(diff.DiffinDest, fmt.Sprintf("%s, Replicas src = %v, dest = %v, repo = %v", srcResName, srcreplicas, destreplicas, util.GetRepoFromunstructured(destRes)))
			}
		}
	}
	return diff
}

func ToMap(resources []unstructured.Unstructured) map[string]unstructured.Unstructured {
	op := make(map[string]unstructured.Unstructured)
	for _, v := range resources {
		op[v.GetName()] = v
	}

	return op
}

func findMissing(resource string, lhs, rhs map[string]unstructured.Unstructured) Differences {
	missing := Differences{}
	dest, src := getMissingElemnets(lhs, rhs), getMissingElemnets(rhs, lhs)
	missing.name = resource
	missing.DiffinSRC = src
	missing.DiffinDest = dest
	return missing
}

func getMissingElemnets(lhs, rhs map[string]unstructured.Unstructured) []string {

	missing := make([]string, 0)
	for k, resObj := range lhs {
		if _, ok := rhs[k]; !ok {
			missing = append(missing, fmt.Sprintf("%v, repo: %v", k, util.GetRepoFromunstructured(resObj)))
		}
	}
	return missing
}

func getGVRs() []schema.GroupVersionResource {

	gvrs := make([]schema.GroupVersionResource, 0)
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Group: "apps", Resource: "deployments"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Resource: "services"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Group: "autoscaling.k8s.io", Resource: "verticalpodautoscalers"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Group: "policy", Resource: "poddisruptionbudgets"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Resource: "ingresses", Group: "networking.k8s.io"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1", Resource: "horizontalpodautoscalers", Group: "autoscaling"})
	gvrs = append(gvrs, schema.GroupVersionResource{Version: "v1beta1", Resource: "cronjobs", Group: "batch"})

	return gvrs
}

func printOP(finalOp map[string]Differences) {

	for k, op := range finalOp {
		fmt.Printf("********************************* %s ***********************************", k)
		fmt.Println()
		fmt.Println()

		if len(op.DiffinDest) > 0 {
			fmt.Println("--- in Destination ----")
			for _, a := range op.DiffinDest {
				fmt.Println(a)
			}
			fmt.Println("---------------------")
			fmt.Println()

		}
		if len(op.DiffinSRC) > 0 {
			fmt.Println("--- in Source ----")
			for _, a := range op.DiffinSRC {
				fmt.Println(a)
			}
			fmt.Println("---------------------")
			fmt.Println()

		}
		fmt.Println("******************************************************************************************************")
		fmt.Println()
		fmt.Println()
		fmt.Println()
	}

}
