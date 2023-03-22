package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"kube-diff/check"
	"kube-diff/cron"
	so "kube-diff/scaledobject"
	"kube-diff/util"
	wrokers "kube-diff/workers"
	"os"
	"strings"

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

	fmt.Println("-----------------------------deploy Failed due to spec file-----------------------------")
	abc(op.InvalidSpecArr)
	fmt.Println("---------------------------------------------------------------------------------------")

	fmt.Println("-----------------------------deploy Missing in Destination cluster-----------------------------")
	abc(op.MissingDest)
	fmt.Println("---------------------------------------------------------------------------------------")

	fmt.Println("-----------------------------deploy Failed to update due to update error-----------------------------")
	abc(op.UpdateFailed)
	fmt.Println("---------------------------------------------------------------------------------------")
}

var setreplica bool = false

func syncDeployment(srcClusterClient, dstClusterClient *kubernetes.Clientset, runType util.RunType) {

	skipMirrorSpecDeploy := map[string]bool{"oauth2-google": true, "synthetics-private-location": true}
	srcDeployments := getDeploymentsList(srcClusterClient)

	skippedDeployments := wrokers.GetWorkers()
	msg := fmt.Sprintf("Syncing %d Deployments", len(srcDeployments.Items))
	if setreplica {
		msg += " with replicas"
	}
	fmt.Printf("***********************************>> %s <<**************************************************", msg)
	fmt.Println()

	var cnt int = 0
	for _, srcdeploy := range srcDeployments.Items {
		cnt++
		mInfo, err := util.GetMirroSpec(srcdeploy)
		if err {
			fmt.Printf("cannot create Mirror spec for %s", srcdeploy.Name)
			fmt.Println()
			if _, ok := skipMirrorSpecDeploy[srcdeploy.Name]; !ok {
				panic(err)
			}
			continue
		}

		//if worker managed in pd-app-charts , skip for --deploy and --mirror options
		_, isWorker := skippedDeployments[strings.ToLower(mInfo.AppName)]
		if runType != util.EnableWorkers && strings.EqualFold(mInfo.DH_repo, "pd-app-charts") && isWorker {
			fmt.Printf("%d - Skipping Deploy %s as its worker & managed by pd-app-charts", cnt, mInfo.DeployName)
			continue
		}
		//if mirror then scale replica for only workers not managed by pd-app-charts
		if runType == util.Mirror {
			if isWorker {
				setreplica = false
			} else {
				setreplica = true
			}
		}

		ChkupdateDestDeploy(dstClusterClient, mInfo, cnt)
	}
	op.Print()
	fmt.Println("*********************************** >> Deployments Synced << **************************************************")
}

func ChkupdateDestDeploy(dstClusterClient *kubernetes.Clientset, srcmInfo util.MirrorSpec, cnt int) bool {

	dstDeploy, err := dstClusterClient.AppsV1().Deployments("default").Get(context.Background(), srcmInfo.DeployName, v1.GetOptions{})

	if err != nil {
		op.AddMissingDest(srcmInfo.DeployName)
		return false
	}

	dstmInfo, _ := util.GetMirroSpec(*dstDeploy)

	if !srcmInfo.Equals(dstmInfo, setreplica) {
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

	if setreplica {
		dstDeploy.Spec.Replicas = &srcmInfo.Replicas
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

func checkPanic(err error) {
	if err != nil {
		panic(err)
	}
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

	config_built.QPS = 100
	config_built.Burst = 500
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

	src_config := parser.String("s", "src", &argparse.Options{Required: false, Help: "Path to the Source cluster's kubeconfig (Normally old cluster)"})
	dest_config := parser.String("d", "dst", &argparse.Options{Required: false, Help: "Path to the Destination/target cluster's kubeconfig (Normally new cluster)"})
	worker_list := parser.String("w", "worker", &argparse.Options{Required: false, Help: "Path to the worker list (pd-devops/scripts/cluster_upgrade/000_worker_list)"})

	deploy := parser.Flag("", "deploy", &argparse.Options{Help: "Optionally attempt to Sync all deployments from first cluster to second(without replicas)"})
	enableCron := parser.Flag("", "cron", &argparse.Options{Help: "Optionally attempt to Sync all cronJobs from first cluster to second(without Suspend)"})

	set_replicas := parser.Flag("r", "replicas", &argparse.Options{Help: "Optionally attempt to set replicas for all workloads from first cluster to second (use with --deploy)"})
	mirror := parser.Flag("m", "mirror", &argparse.Options{Help: "Optionally attempt to mirror replicas for deploy and set the cronjob status"})
	compare := parser.Flag("", "compare", &argparse.Options{Help: "Optionally attempt to compare clusters"})
	version := parser.Flag("v", "version", &argparse.Options{Help: "get version with release info"})
	enableWorkers := parser.Flag("", "enableworkers", &argparse.Options{Help: "Enable workers, scaledobject in dest cluster and disable it in old cluster"})
	saveState := parser.Flag("", "savestate", &argparse.Options{Help: "save state of deployments"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	if *version {
		fmt.Printf("version: %v,", 0.05)
		fmt.Println("Mirror now scales updates the spec to src cluster and sets replica, updates the scaled object")
		return
	}

	validateInputs(*src_config, *dest_config)
	wrokers.SetWorkers(*worker_list)

	if *compare {
		check.CheckCluster(*src_config, *dest_config, wrokers.GetWorkers())
		return
	}

	srcClusterKConfig := createConfig(*src_config)
	dstClusterKConfig := createConfig(*dest_config)

	srcClusterClient := newK8sConnectionConfig(srcClusterKConfig)
	dstClusterClient := newK8sConnectionConfig(dstClusterKConfig)

	if *saveState {
		SavereplicasForAllDeployment(srcClusterClient)
		return
	}

	var runType util.RunType

	if *mirror {
		runType = util.Mirror
	} else if *deploy {
		runType = util.Deploy
	} else if *enableWorkers {
		runType = util.EnableWorkers
	}

	if *set_replicas {
		setreplica = true
	}

	if *enableWorkers {
		//Sync replicas from the old cluster for workers , scale down replicas in old cluster
		wrokers.EnableWorkers(srcClusterClient, dstClusterClient, wrokers.GetWorkers(), util.GetClusterNamefromConfig(*src_config))
		//unpause the scaledobje
		nMap, _ := wrokers.GetMirrorInfoFromState(util.GetClusterNamefromConfig(*src_config))
		so.UpdateScaledObject2(*src_config, *dest_config, wrokers.GetWorkers(), util.EnableWorkers, nMap)
	}

	if *deploy || *mirror {
		syncDeployment(srcClusterClient, dstClusterClient, runType)
	}

	if *mirror {
		so.UpdateScaledObject(*src_config, *dest_config, wrokers.GetWorkers(), util.Mirror)
	}

	if *enableCron || *mirror {
		cron.SyncCron(srcClusterClient, dstClusterClient)
	}
}

func validateInputs(src_config, dest_config string) {
	var confirm string

	if src_config == "" || dest_config == "" {
		fmt.Println("src config and dest config is empty.")
		os.Exit(1)
	}
	if strings.Contains(src_config, "v123") {
		fmt.Println("The source cluster is v123. type 'yes' to confirm")
		fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "yes") {
			fmt.Println(("User exited"))
			os.Exit(1)
		}
	}

}

func SavereplicasForAllDeployment(srcClusterClient *kubernetes.Clientset) {
	deployments, err := srcClusterClient.AppsV1().Deployments("default").List(context.Background(), v1.ListOptions{})
	util.TryPanic(err)
	skipMirrorSpecDeploy := map[string]bool{"oauth2-google": true, "synthetics-private-location": true}

	mMap := make(map[string]util.MirrorSpec)
	var mInfo util.MirrorSpec
	for _, deploy := range deployments.Items {
		if _, ok := skipMirrorSpecDeploy[deploy.Name]; !ok {
			mInfo, _ = util.GetMirroSpec(deploy)
			if _, o := mMap[deploy.Name]; !o {
				mMap[deploy.Name] = mInfo
			}
		}
	}

	b, _ := json.Marshal(mMap)
	ioutil.WriteFile(fmt.Sprintf("%v.json", mInfo.ClusterName), b, 0644)
}
