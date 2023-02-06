package cron

import (
	"context"
	"fmt"
	"strings"

	v1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

func FindDiff(srcClusterClient, dstClusterClient *kubernetes.Clientset) {
	srcCronJobs := getCronJobsList(srcClusterClient)
	op := make([]string, 0)
	for _, srcCron := range srcCronJobs.Items {
		if isSame, reason := kubeDiff(srcCron, srcClusterClient, dstClusterClient); !isSame {
			op = append(op, fmt.Sprintf("%s, Reason: %s", srcCron.Name, reason))
		}
	}

	for _, o := range op {
		fmt.Println(o)
	}
}

func SyncCron(srcClusterClient, dstClusterClient *kubernetes.Clientset) {

	srcCronJobs := getCronJobsList(srcClusterClient)

	msg := fmt.Sprintf("Syncing %d CronJobs", len(srcCronJobs.Items))
	fmt.Printf("***********************************>> %s <<**************************************************", msg)
	fmt.Println()

	var cnt int = 0
	for _, srcCron := range srcCronJobs.Items {
		cnt++
		mInfo, err := getMirroSpec(srcCron)
		if err {
			continue
		}
		ChkupdateDestCron(dstClusterClient, mInfo, cnt)
	}
	op.Print()
	fmt.Println("*********************************** >> Cronjobs Synced << **************************************************")
}

func ChkupdateDestCron(dstClusterClient *kubernetes.Clientset, srcmInfo MirrorSpec, cnt int) bool {

	dstCron, err := dstClusterClient.BatchV1beta1().CronJobs("default").Get(context.Background(), srcmInfo.CronName, v1.GetOptions{})

	if err != nil {
		op.AddMissingDest(srcmInfo.CronName)
		return false
	}

	dstmInfo, _ := getMirroSpec(*dstCron)

	if !checkEqual(srcmInfo, dstmInfo) {
		return UpdateCron(srcmInfo, dstmInfo, dstClusterClient, dstCron, cnt)
	} else {
		fmt.Printf("%d - Skipping CronJob %s as both are equal", cnt, srcmInfo.CronName)
		fmt.Println()
	}
	return true
}

func UpdateCron(srcmInfo MirrorSpec, dstmInfo MirrorSpec, dstClusterClient *kubernetes.Clientset, dstCron *v1beta1.CronJob, cnt int) bool {
	dstCron.Spec.JobTemplate.Spec.Template.Labels["cfg"] = srcmInfo.AppConfig
	dstCron.Spec.JobTemplate.Spec.Template.Annotations["image"] = srcmInfo.ImageAnnotation
	dstCron.Spec.JobTemplate.Spec.Template.Labels["version"] = srcmInfo.version_label
	dstCron.Spec.JobTemplate.Spec.Template.Labels["tags.datadoghq.com/version"] = srcmInfo.dd_label

	if strings.ToLower(srcmInfo.isSpec) == "false" {
		dstCron.Spec.JobTemplate.Spec.Template.Labels["infra-cfg"] = srcmInfo.infra_cfg
	}

	for idx, cnt := range dstCron.Spec.JobTemplate.Spec.Template.Spec.Containers {
		if cnt.Name == srcmInfo.CronName {
			dstCron.Spec.JobTemplate.Spec.Template.Spec.Containers[idx].Image = getImage(cnt.Image, srcmInfo.Image, srcmInfo.clusterName, dstmInfo.clusterName)
		}
	}

	fmt.Printf("%d - Updating Cron %s", cnt, srcmInfo.CronName)
	fmt.Println()
	_, err := dstClusterClient.BatchV1beta1().CronJobs("default").Update(context.Background(), dstCron, v1.UpdateOptions{})

	if err != nil {
		op.AddUpdateFailed(srcmInfo.CronName)
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

	if srcmInfo.CronName != dstmInfo.CronName {
		return false
	}

	if srcmInfo.infra_cfg != dstmInfo.infra_cfg {
		return false
	}

	if srcmInfo.ImageAnnotation != dstmInfo.ImageAnnotation {
		return false
	}

	return true
}

func checkPanic(err error) {
}

func kubeDiff(srcCron v1beta1.CronJob, srcClusterClient, dstClusterClient *kubernetes.Clientset) (bool, string) {

	currSrcCron, err := srcClusterClient.BatchV1beta1().CronJobs("default").Get(context.Background(), srcCron.Name, v1.GetOptions{})
	checkPanic(err)
	currDstCron, err := dstClusterClient.BatchV1beta1().CronJobs("default").Get(context.Background(), srcCron.Name, v1.GetOptions{})
	checkPanic(err)

	srcMirorInfo, errr := getMirroSpec(*currSrcCron)
	if errr {
		return false, "cannot find in source cluster"
	}
	dstMirorInfo, errr := getMirroSpec(*currDstCron)
	if errr {
		return false, "cannot find in destination cluster"
	}
	if !checkEqual(srcMirorInfo, dstMirorInfo) {
		return false, "diff in spec/image"
	}

	if srcMirorInfo.Suspend != dstMirorInfo.Suspend {
		return false, fmt.Sprintf("suspend in source = %s, destination = %s different.", srcMirorInfo.CronName, dstMirorInfo.CronName)
	}

	return true, ""
}

func getMirroSpec(cron v1beta1.CronJob) (MirrorSpec, bool) {

	appName := cron.Labels["app"]

	isSpec := cron.Spec.JobTemplate.Spec.Template.Labels["specfile"]

	Suspend := cron.Spec.Suspend

	cronName := cron.Name

	AppConfig := cron.Spec.JobTemplate.Spec.Template.Labels["cfg"]

	infra_cf := cron.Spec.JobTemplate.Spec.Template.Labels["infra-cfg"]

	Image := getContainer(cron, cronName)
	if Image == "" {
		return MirrorSpec{}, true
	}

	ImageAnnotation := cron.Spec.JobTemplate.Spec.Template.Annotations["image"]
	version_label := cron.Spec.JobTemplate.Spec.Template.Labels["version"]
	dd_label := cron.Spec.JobTemplate.Spec.Template.Labels["tags.datadoghq.com/version"]

	mSpec := MirrorSpec{}
	mSpec.appName = appName
	mSpec.AppConfig = AppConfig
	mSpec.CronName = cronName
	mSpec.Image = Image
	mSpec.ImageAnnotation = ImageAnnotation
	mSpec.version_label = version_label
	mSpec.dd_label = dd_label
	mSpec.dd_label = dd_label
	mSpec.clusterName = cron.Labels["cluster"]
	mSpec.infra_cfg = infra_cf
	mSpec.isSpec = isSpec
	mSpec.Suspend = *Suspend

	return mSpec, false
}

func getContainer(cron v1beta1.CronJob, cronName string) string {

	for _, container := range cron.Spec.JobTemplate.Spec.Template.Spec.Containers {
		if container.Name == cronName {
			return container.Image
		}
	}
	return ""
}

type MirrorSpec struct {
	appName         string
	AppConfig       string
	Image           string
	ImageAnnotation string
	version_label   string
	dd_label        string
	clusterName     string
	CronName        string
	infra_cfg       string
	isSpec          string
	Suspend         bool
}

func getCronJobsList(clientSet *kubernetes.Clientset) *v1beta1.CronJobList {

	CronJobs, err := clientSet.BatchV1beta1().CronJobs("default").List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	return CronJobs
}
