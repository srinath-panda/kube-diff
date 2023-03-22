package util

import (
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type MirrorSpec struct {
	AppName         string
	DeployName      string
	AppConfig       string
	Image           string
	ImageAnnotation string
	Version_label   string
	Dd_label        string
	ClusterName     string
	Infra_cfg       string
	IsSpec          string
	Replicas        int32
	Suspend         bool
	DH_repo         string
}

func GetMirroSpec(deploy appsv1.Deployment) (MirrorSpec, bool) {

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
	mSpec.AppName = appName
	mSpec.DeployName = DeployName
	mSpec.AppConfig = AppConfig
	mSpec.Image = Image
	mSpec.ImageAnnotation = ImageAnnotation
	mSpec.Version_label = version_label
	mSpec.Dd_label = dd_label
	mSpec.Dd_label = dd_label
	mSpec.ClusterName = deploy.Labels["cluster"]
	mSpec.Infra_cfg = infra_cf
	mSpec.IsSpec = isSpec
	mSpec.Replicas = *replicas
	mSpec.DH_repo = deploy.Spec.Template.Labels["dh/repo"]

	return mSpec, false
}

func (srcmInfo *MirrorSpec) Equals(dstmInfo MirrorSpec, checkReplica bool) bool {

	if srcmInfo.AppName != dstmInfo.AppName {
		return false
	}
	if srcmInfo.Dd_label != dstmInfo.Dd_label {
		return false
	}
	if srcmInfo.Version_label != dstmInfo.Version_label {
		return false
	}

	if srcmInfo.AppConfig != dstmInfo.AppConfig {
		return false
	}

	if srcmInfo.DeployName != dstmInfo.DeployName {
		return false
	}

	if srcmInfo.Infra_cfg != dstmInfo.Infra_cfg {
		return false
	}

	if srcmInfo.ImageAnnotation != dstmInfo.ImageAnnotation {
		return false
	}

	if checkReplica && srcmInfo.Replicas != dstmInfo.Replicas {
		return false
	}

	// if strings.HasPrefix(srcmInfo.Image, "gcr-registry") {
	// 	srcImage := strings.Join(strings.Split(srcmInfo.Image, "/")[1:], "/")
	// 	dstImage := strings.Join(strings.Split(dstmInfo.Image, "/")[1:], "/")
	// 	return srcImage == dstImage
	// } else {
	// 	return srcmInfo.Image == dstmInfo.Image
	// }
	return true
}

func getContainer(deploy appsv1.Deployment, DeployName string) string {

	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == DeployName {
			return container.Image
		}
	}
	return ""
}

func GetUnstructuredObjectNestedVal(res unstructured.Unstructured, throwerror bool, fields ...string) interface{} {

	val, fnd, err := unstructured.NestedFieldNoCopy(res.Object, fields...)
	if throwerror && (err != nil || !fnd) {
		panic("not found")
	}

	return val
}

func checkReturnEmptyString(val interface{}) string {

	if val == nil {
		return ""
	}
	return val.(string)
}

func GetMirroSpecForUnstructured(deploy unstructured.Unstructured) (MirrorSpec, bool) {

	DeployName := deploy.GetName()

	appName := GetUnstructuredObjectNestedVal(deploy, true, "metadata", "labels", "app").(string)
	replicas := GetUnstructuredObjectNestedVal(deploy, true, "spec", "replicas").(int64)

	ImageAnnotation := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "annotations", "image"))
	version_label := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "labels", "version"))
	dd_label := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "labels", "tags.datadoghq.com/version"))
	isSpec := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "labels", "specfile"))
	infra_cf := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "labels", "infra-cfg"))
	AppConfig := checkReturnEmptyString(GetUnstructuredObjectNestedVal(deploy, false, "spec", "template", "metadata", "labels", "cfg"))

	Image := ""
	containers := GetUnstructuredObjectNestedVal(deploy, true, "spec", "template", "spec", "containers").([]interface{})
	for _, container := range containers {
		containerMap := container.(map[string]interface{})
		if strings.EqualFold(containerMap["name"].(string), DeployName) {
			Image = containerMap["image"].(string)
			break
		}
	}
	if Image == "" {
		return MirrorSpec{}, true
	}

	mSpec := MirrorSpec{}
	mSpec.AppName = appName
	mSpec.DeployName = DeployName
	mSpec.AppConfig = AppConfig
	mSpec.Image = Image
	mSpec.ImageAnnotation = ImageAnnotation
	mSpec.Version_label = version_label
	mSpec.Dd_label = dd_label
	mSpec.Dd_label = dd_label
	mSpec.ClusterName = GetUnstructuredObjectNestedVal(deploy, true, "metadata", "labels", "cluster").(string)
	mSpec.Infra_cfg = infra_cf
	mSpec.IsSpec = isSpec
	mSpec.Replicas = int32(replicas)

	return mSpec, false

}

func GetMirroSpecForUnstructuredForCron(cron unstructured.Unstructured) (MirrorSpec, bool) {

	DeployName := cron.GetName()

	appName := GetUnstructuredObjectNestedVal(cron, true, "metadata", "labels", "app").(string)
	suspend := GetUnstructuredObjectNestedVal(cron, true, "spec", "suspend").(bool)

	ImageAnnotation := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "annotations", "image"))
	version_label := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "labels", "version"))
	dd_label := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "labels", "tags.datadoghq.com/version"))
	isSpec := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "labels", "specfile"))
	infra_cf := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "labels", "infra-cfg"))
	AppConfig := checkReturnEmptyString(GetUnstructuredObjectNestedVal(cron, false, "spec", "jobTemplate", "spec", "template", "metadata", "labels", "cfg"))

	Image := ""
	containers := GetUnstructuredObjectNestedVal(cron, true, "spec", "jobTemplate", "spec", "template", "spec", "containers").([]interface{})
	for _, container := range containers {
		containerMap := container.(map[string]interface{})
		if strings.EqualFold(containerMap["name"].(string), DeployName) {
			Image = containerMap["image"].(string)
			break
		}
	}
	if Image == "" {
		return MirrorSpec{}, true
	}

	mSpec := MirrorSpec{}
	mSpec.AppName = appName
	mSpec.DeployName = DeployName
	mSpec.AppConfig = AppConfig
	mSpec.Image = Image
	mSpec.ImageAnnotation = ImageAnnotation
	mSpec.Version_label = version_label
	mSpec.Dd_label = dd_label
	mSpec.Dd_label = dd_label
	mSpec.ClusterName = GetUnstructuredObjectNestedVal(cron, true, "metadata", "labels", "cluster").(string)
	mSpec.Infra_cfg = infra_cf
	mSpec.IsSpec = isSpec
	mSpec.Replicas = int32(0)
	mSpec.Suspend = suspend
	return mSpec, false

}

func TryPanic(err error) {
	if err != nil {
		panic(err)
	}
}

type RunType int

const (
	Deploy RunType = iota
	Mirror
	EnableWorkers
)

func GetClusterNamefromConfig(src_config string) string {
	paths := strings.Split(src_config, "/")
	cluster := paths[len(paths)-1]

	test := strings.Split(cluster, ".")
	print(test[len(test)-1])
	clusterName := test[len(test)-1]
	return clusterName
}
