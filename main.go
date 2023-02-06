package main

import (
	"context"
	"fmt"
	"kube-diff/cron"
	"os"
	"strings"
	"sync"

	"github.com/akamensky/argparse"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type output struct {
	InvalidSpecArr []string
	MissingDest    []string
	UpdateFailed   []string
}

var op output = output{InvalidSpecArr: make([]string, 0), MissingDest: make([]string, 0), UpdateFailed: make([]string, 0)}

func (op *output) AddInvalidSpecArr(str string) {
	op.InvalidSpecArr = append(op.InvalidSpecArr, str)
}

func (op *output) AddMissingDest(str string) {
	op.MissingDest = append(op.MissingDest, str)
}

func (op *output) AddUpdateFailed(str string) {
	op.UpdateFailed = append(op.UpdateFailed, str)
}

func (op *output) Print() {

	abc := func(strarr []string) {
		for _, s := range strarr {
			fmt.Println(s)
		}
	}

	fmt.Println("-----------------------------Failed due to spec file-----------------------------")
	abc(op.InvalidSpecArr)
	fmt.Println("---------------------------------------------------------------------------------------")

	fmt.Println("-----------------------------Missing in Destination cluster-----------------------------")
	abc(op.MissingDest)
	fmt.Println("---------------------------------------------------------------------------------------")

	fmt.Println("-----------------------------Failed top update due to update error-----------------------------")
	abc(op.UpdateFailed)
	fmt.Println("---------------------------------------------------------------------------------------")
}

var setreplica bool = false

func findDiff(srcClusterClient, dstClusterClient *kubernetes.Clientset) {
	var wg sync.WaitGroup
	guard := make(chan struct{}, 5)
	srcDeployments := getDeploymentsList(srcClusterClient)

	for _, srcdeploy := range srcDeployments.Items {

		wg.Add(1)
		guard <- struct{}{}
		go func(srcdeploy appsv1.Deployment, srcClusterClient, dstClusterClient *kubernetes.Clientset) {
			defer wg.Done()
			if !kubeDiff(srcdeploy, srcClusterClient, dstClusterClient) {
				fmt.Println(srcdeploy.Name)
			}
			<-guard
		}(srcdeploy, srcClusterClient, dstClusterClient)
	}
	wg.Wait()
	return
}

func syncDeployment(srcClusterClient, dstClusterClient *kubernetes.Clientset) {
	srcDeployments := getDeploymentsList(srcClusterClient)

	msg := fmt.Sprintf("Syncing %d Deployments", len(srcDeployments.Items))
	if setreplica {
		msg += " with replicas"
	}
	fmt.Printf("***********************************>> %s <<**************************************************", msg)
	fmt.Println()

	var cnt int = 0
	for _, srcdeploy := range srcDeployments.Items {
		cnt++
		mInfo, err := getMirroSpec(srcdeploy)
		if err {
			continue
		}
		ChkupdateDestDeploy(dstClusterClient, mInfo, cnt)
	}
	op.Print()
	fmt.Println("*********************************** >> Deployments Synced << **************************************************")
}

func ChkupdateDestDeploy(dstClusterClient *kubernetes.Clientset, srcmInfo MirrorSpec, cnt int) bool {

	dstDeploy, err := dstClusterClient.AppsV1().Deployments("default").Get(context.Background(), srcmInfo.DeployName, v1.GetOptions{})

	if err != nil {
		op.AddMissingDest(srcmInfo.DeployName)
		return false
	}

	dstmInfo, _ := getMirroSpec(*dstDeploy)

	if !checkEqual(srcmInfo, dstmInfo) {
		return UpdateDeploy(srcmInfo, dstmInfo, dstClusterClient, dstDeploy, cnt)
	} else {
		fmt.Printf("%d - Skipping Deploy %s as both are equal", cnt, srcmInfo.DeployName)
		fmt.Println()
	}
	return true
}

func UpdateDeploy(srcmInfo MirrorSpec, dstmInfo MirrorSpec, dstClusterClient *kubernetes.Clientset, dstDeploy *appsv1.Deployment, cnt int) bool {
	dstDeploy.Spec.Template.Labels["cfg"] = srcmInfo.AppConfig
	dstDeploy.Spec.Template.Annotations["image"] = srcmInfo.ImageAnnotation
	dstDeploy.Spec.Template.Labels["version"] = srcmInfo.version_label
	dstDeploy.Spec.Template.Labels["tags.datadoghq.com/version"] = srcmInfo.dd_label

	if strings.ToLower(srcmInfo.isSpec) == "false" {
		dstDeploy.Spec.Template.Labels["infra-cfg"] = srcmInfo.infra_cfg
	}

	for idx, cnt := range dstDeploy.Spec.Template.Spec.Containers {
		if cnt.Name == srcmInfo.DeployName {
			dstDeploy.Spec.Template.Spec.Containers[idx].Image = getImage(cnt.Image, srcmInfo.Image, srcmInfo.clusterName, dstmInfo.clusterName)
		}
	}

	if setreplica {
		dstDeploy.Spec.Replicas = &srcmInfo.replicas
	}

	fmt.Printf("%d - Updating Deploy %s", cnt, srcmInfo.DeployName)
	fmt.Println()
	_, err := dstClusterClient.AppsV1().Deployments("default").Update(context.Background(), dstDeploy, v1.UpdateOptions{})

	if err != nil {
		op.AddUpdateFailed(srcmInfo.DeployName)
	}
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

func checkEqual(srcmInfo, dstmInfo MirrorSpec) bool {

	if srcmInfo.appName != dstmInfo.appName {
		return false
	}
	if srcmInfo.dd_label != dstmInfo.dd_label {
		return false
	}
	if srcmInfo.version_label != dstmInfo.version_label {
		return false
	}

	if srcmInfo.AppConfig != dstmInfo.AppConfig {
		return false
	}

	if srcmInfo.DeployName != dstmInfo.DeployName {
		return false
	}

	if srcmInfo.infra_cfg != dstmInfo.infra_cfg {
		return false
	}

	if srcmInfo.ImageAnnotation != dstmInfo.ImageAnnotation {
		return false
	}

	if setreplica && srcmInfo.replicas != dstmInfo.replicas {
		return false
	}
	return true
}

func checkPanic(err error) {
}

func kubeDiff(srcdeploy appsv1.Deployment, srcClusterClient, dstClusterClient *kubernetes.Clientset) bool {

	currSrcDeploy, err := srcClusterClient.AppsV1().Deployments("default").Get(context.Background(), srcdeploy.Name, v1.GetOptions{})
	checkPanic(err)
	currDstDeploy, err := dstClusterClient.AppsV1().Deployments("default").Get(context.Background(), srcdeploy.Name, v1.GetOptions{})
	checkPanic(err)

	srcMirorInfo, errr := getMirroSpec(*currSrcDeploy)
	if errr {
		return false
	}
	dstMirorInfo, errr := getMirroSpec(*currDstDeploy)
	if errr {
		return false
	}
	if !checkEqual(srcMirorInfo, dstMirorInfo) {
		return false
	}

	return true
}

func getMirroSpec(deploy appsv1.Deployment) (MirrorSpec, bool) {

	appName := deploy.Labels["app"]

	isSpec := deploy.Spec.Template.Labels["specfile"]

	replicas := deploy.Spec.Replicas

	DeployName := deploy.Name
	AppConfig := deploy.Spec.Template.Labels["cfg"]

	infra_cf := deploy.Spec.Template.Labels["infra-cfg"]

	Image := getContainer(deploy, DeployName)
	if Image == "" {
		return MirrorSpec{}, true
	}

	ImageAnnotation := deploy.Spec.Template.Annotations["image"]
	version_label := deploy.Spec.Template.Labels["version"]
	dd_label := deploy.Spec.Template.Labels["tags.datadoghq.com/version"]

	mSpec := MirrorSpec{}
	mSpec.appName = appName
	mSpec.DeployName = DeployName
	mSpec.AppConfig = AppConfig
	mSpec.Image = Image
	mSpec.ImageAnnotation = ImageAnnotation
	mSpec.version_label = version_label
	mSpec.dd_label = dd_label
	mSpec.dd_label = dd_label
	mSpec.clusterName = deploy.Labels["cluster"]
	mSpec.infra_cfg = infra_cf
	mSpec.isSpec = isSpec
	mSpec.replicas = *replicas

	return mSpec, false
}

func getContainer(deploy appsv1.Deployment, DeployName string) string {

	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == DeployName {
			return container.Image
		}
	}
	return ""
}

type MirrorSpec struct {
	appName         string
	DeployName      string
	AppConfig       string
	Image           string
	ImageAnnotation string
	version_label   string
	dd_label        string
	clusterName     string
	cronName        string
	infra_cfg       string
	isSpec          string
	replicas        int32
}

func getDeploymentsList(clientSet *kubernetes.Clientset) *appsv1.DeploymentList {

	deployments, err := clientSet.AppsV1().Deployments("default").List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	return deployments
}

func createConfig(config string) *rest.Config {
	config_built, err := clientcmd.BuildConfigFromFlags("", config)
	if err != nil {
		panic(err.Error())
	}
	return config_built
}

func newK8sConnectionConfig(a_config_built *rest.Config) *kubernetes.Clientset {
	the_clientset, err := kubernetes.NewForConfig(a_config_built)
	if err != nil {
		panic(err.Error())
	}
	return the_clientset
}

func main() {

	parser := argparse.NewParser("print", "Prints provided string to stdout")

	src_config := parser.String("s", "src", &argparse.Options{Required: true, Help: "Path to the Source cluster's kubeconfig (Normally old cluster)"})
	dest_config := parser.String("d", "dst", &argparse.Options{Required: true, Help: "Path to the Destination/target cluster's kubeconfig (Normally new cluster)"})

	diff := parser.Flag("", "diff", &argparse.Options{Help: "Optionally attempt to diff all workloads from first cluster to second(without replicas)"})
	deploy := parser.Flag("", "deploy", &argparse.Options{Help: "Optionally attempt to Sync all deployments from first cluster to second(without replicas)"})
	enableCron := parser.Flag("", "cron", &argparse.Options{Help: "Optionally attempt to Sync all cronJobs from first cluster to second(without Suspend)"})

	set_replicas := parser.Flag("r", "replicas", &argparse.Options{Help: "Optionally attempt to set replicas for all workloads from first cluster to second"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	srcClusterKConfig := createConfig(*src_config)
	dstClusterKConfig := createConfig(*dest_config)

	srcClusterClient := newK8sConnectionConfig(srcClusterKConfig)
	dstClusterClient := newK8sConnectionConfig(dstClusterKConfig)

	if *diff {
		findDiff(srcClusterClient, dstClusterClient)
		return
	}

	if *set_replicas {
		setreplica = true
	}

	if *deploy {
		syncDeployment(srcClusterClient, dstClusterClient)
	}

	if *enableCron {
		cron.SyncCron(srcClusterClient, dstClusterClient)
	}
}
