package wrokers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"kube-diff/util"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
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

func GetMirrorInfoFromState(ClusterName string) (map[string]util.MirrorSpec, error) {
	mMap := make(map[string]util.MirrorSpec)
	op, err := ioutil.ReadFile(fmt.Sprintf("%v.json", ClusterName))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(op, &mMap)
	return mMap, err
}

func EnableWorkers(srcClusterClient, dstClusterClient *kubernetes.Clientset, workers_list map[string]bool, clusterName string) {
	fmt.Printf("****************************************Enabling workers total count (%v) ****************************************", len(workers_list))
	fmt.Println()

	mMap, err := GetMirrorInfoFromState(clusterName)
	util.TryPanic(err)

	for app := range workers_list {
		selector := fmt.Sprintf("app=%s", app)
		deployments, _ := srcClusterClient.AppsV1().Deployments("default").List(context.Background(), v1.ListOptions{LabelSelector: selector})
		cnt := 0
		for _, srcdeploy := range deployments.Items {
			// minfo, _ := util.GetMirroSpec(srcdeploy)
			minfo, _ := mMap[srcdeploy.Name]

			fmt.Printf("Scale down replica to 0 for %v", minfo.DeployName)
			fmt.Println()
			// scale down the replica in the src cluster
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return UpdateReplicasForDeploy(srcdeploy.Name, 0, srcClusterClient)
			})
			util.TryPanic(retryErr)
			if minfo.DH_repo == "pd-app-charts" {
				// //update the image in src to a dummy one
				fmt.Printf("Setting the image to `change-me` for %v", minfo.DeployName)
				fmt.Println()
				retryErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					return UpdateImageForDeploy(srcdeploy.Name, "change-me", srcdeploy.Name, srcClusterClient)
				})
			}

			util.TryPanic(retryErr)
			cnt++
			ChkupdateDestDeploy(dstClusterClient, minfo, cnt)
		}
	}

	fmt.Println("****************************************Enabling workers****************************************")
	fmt.Println()
}

func UpdateReplicasForDeploy(deployName string, replica int32, ClusterClient *kubernetes.Clientset) error {
	deployment, err := ClusterClient.AppsV1().Deployments("default").Get(context.Background(), deployName, v1.GetOptions{})
	//util.TryPanic(err)
	deployment.Spec.Replicas = int32Ptr(replica)
	_, err = ClusterClient.AppsV1().Deployments("default").Update(context.Background(), deployment, v1.UpdateOptions{})
	return err
}

func UpdateImageForDeploy(deployName string, image string, containerName string, ClusterClient *kubernetes.Clientset) error {
	deployment, err := ClusterClient.AppsV1().Deployments("default").Get(context.Background(), deployName, v1.GetOptions{})
	util.TryPanic(err)

	cntCount := 0
	fnd := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == deployName {
			fnd = true
			break
		}
		cntCount++
	}

	if !fnd {
		util.TryPanic(errors.New(fmt.Sprintf("cannot find the container %v", containerName)))
	}

	deployment.Spec.Template.Spec.Containers[cntCount].Image = image
	_, err = ClusterClient.AppsV1().Deployments("default").Update(context.Background(), deployment, v1.UpdateOptions{})
	return err
}

func int32Ptr(i int32) *int32 { return &i }

func ChkupdateDestDeploy(dstClusterClient *kubernetes.Clientset, srcmInfo util.MirrorSpec, cnt int) bool {
	dstDeploy, err := dstClusterClient.AppsV1().Deployments("default").Get(context.Background(), srcmInfo.DeployName, v1.GetOptions{})

	util.TryPanic(err)
	dstmInfo, _ := util.GetMirroSpec(*dstDeploy)

	if !srcmInfo.Equals(dstmInfo, true) {
		return UpdateDeploy(srcmInfo, dstmInfo, dstClusterClient, dstDeploy, cnt)
	} else {
		fmt.Printf("%d - Skipping Deploy %s as both are equal", cnt, srcmInfo.DeployName)
		fmt.Println()
	}
	return true
}

func UpdateDeploy(srcmInfo util.MirrorSpec, dstmInfo util.MirrorSpec, dstClusterClient *kubernetes.Clientset, dstDeploy *appsv1.Deployment, cnt int) bool {
	dstDeploy.Spec.Template.Labels["cfg"] = srcmInfo.AppConfig
	dstDeploy.Spec.Template.Annotations["image"] = srcmInfo.ImageAnnotation
	dstDeploy.Spec.Template.Labels["version"] = srcmInfo.Version_label
	dstDeploy.Spec.Template.Labels["tags.datadoghq.com/version"] = srcmInfo.Dd_label

	if strings.ToLower(srcmInfo.IsSpec) == "false" {
		dstDeploy.Spec.Template.Labels["infra-cfg"] = srcmInfo.Infra_cfg
	}

	for idx, cnt := range dstDeploy.Spec.Template.Spec.Containers {
		if cnt.Name == srcmInfo.DeployName {
			dstDeploy.Spec.Template.Spec.Containers[idx].Image = getImage(cnt.Image, srcmInfo.Image, srcmInfo.ClusterName, dstmInfo.ClusterName)
		}
	}

	dstDeploy.Spec.Replicas = &srcmInfo.Replicas

	fmt.Printf("%d - Updating Deploy %s", cnt, srcmInfo.DeployName)
	fmt.Println()
	_, err := dstClusterClient.AppsV1().Deployments("default").Update(context.Background(), dstDeploy, v1.UpdateOptions{})

	return err == nil
}

func getImage(destImage string, srcImage string, srcclusterName string, dstclusterName string) string {

	if strings.HasPrefix(srcImage, "gcr-registry") {
		srcArr := strings.Split(srcImage, "/")
		dataa := getClusterColorVersion(srcArr[0], srcclusterName, dstclusterName) + "/" + strings.Join(srcArr[1:], "/")
		return dataa
	} else {
		return srcImage
	}
}

func getClusterColorVersion(srcGCRstr string, srcclusterName string, dstclusterName string) string {

	srcclusterarr := strings.Split(srcclusterName, "-")
	dstclusterarr := strings.Split(dstclusterName, "-")

	srccolor := srcclusterarr[len(srcclusterarr)-1]
	srcversion := srcclusterarr[len(srcclusterarr)-2]

	dstcolor := dstclusterarr[len(dstclusterarr)-1]
	dstversion := dstclusterarr[len(dstclusterarr)-2]

	return strings.ReplaceAll(strings.ReplaceAll(srcGCRstr, srccolor, dstcolor), srcversion, dstversion)
}
