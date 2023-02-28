package wrokers

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func GetWorkers() map[string]bool {

	arr := []string{
		"address-service-worker", "availability-indexer", "auditlog-worker", "backend-cron",
		"backend-worker", "billing-engine-worker", "billing-engine-aggregator",
		"billing-receipt", "catalog-dbsync-worker", "campaigns-management-api-clean-campaigns",
		"campaigns-management-worker", "ccs-consumer", "corporate-api-order-payment-worker",
		"corporate-api-order-placed-worker", "corporate-api-order-updates-worker",
		"corporate-api-report-worker", "corporate-api-subscription-worker",
		"customer-intelligence-worker", "customer-worker", "geolocator-worker",
		"incentives-odr-worker", "incentives-pro-worker", "job-scheduler-worker",
		"login-worker", "loyalty-worker", "membership-mgmt-api", "menu-importer-worker",
		"menu-worker", "mmt-mgmt-worker", "offers-mgmt-worker", "offers-worker",
		"order-service-worker", "order-state-machine-worker", "order-tracking-notification-engine",
		"otg-worker", "otp-worker", "pablo-methods-worker", "pablo-refund-worker",
		"partnerships-cvf-worker", "raf-worker", "raf-voucher-worker", "refund-worker",
		"rewards-worker", "subscription-core-worker", "survey-answers-consumer",
	}

	skippedDeployments := make(map[string]bool)
	for _, a := range arr {
		skippedDeployments[strings.ToLower(a)] = true
	}
	return skippedDeployments
}

func EnableWorkers(srcClusterClient, dstClusterClient *kubernetes.Clientset) {

	for app := range GetWorkers() {
		selector := fmt.Sprintf("app=%s", app)
		deployments, _ := srcClusterClient.AppsV1().Deployments("default").List(context.Background(), v1.ListOptions{LabelSelector: selector})

		for _, srcdeploy := range deployments.Items {
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return UpdateReplicasForDeploy(srcdeploy.Name, *srcdeploy.Spec.Replicas, dstClusterClient)
			})
			tryPanic(retryErr)

			retryErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return UpdateReplicasForDeploy(srcdeploy.Name, 0, dstClusterClient)
			})
			tryPanic(retryErr)
		}
	}
}

func UpdateReplicasForDeploy(deployName string, replica int32, ClusterClient *kubernetes.Clientset) error {
	deployment, err := ClusterClient.AppsV1().Deployments("default").Get(context.Background(), deployName, v1.GetOptions{})
	tryPanic(err)
	deployment.Spec.Replicas = int32Ptr(replica)
	_, err = ClusterClient.AppsV1().Deployments("default").Update(context.Background(), deployment, v1.UpdateOptions{})
	return err
}

func tryPanic(err error) {
}

func int32Ptr(i int32) *int32 { return &i }
