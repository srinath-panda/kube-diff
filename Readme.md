## Small helper script to mirror deployments and cronjobs from source to destination cluster

- the script works along with mirror script in pd-devops/charts
- the script updates the app,infra and image in labels, annoatations and spec
- can optionally set the replicas from source clusters
- can find missing between the clusters resource objects and difference in DH labels/Images/annotations/replicas


## for usage , run 
```sh
kube-diff -h 
```
examples 


1. to mirror deployment (with replicas) except the workers
```sh
./kube-diff -s "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v121-green" -d "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v123-blue"  --mirror
``` 

2. to sync deployment (with replicas) and crons
```sh
./kube-diff -s "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v121-green" -d "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v123-blue"  --deploy --replicas --cron
``` 

3. to sync deployment only (without replicas)
```sh
./kube-diff -s "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v121-green" -d "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v123-blue"  --deploy
``` 

4. to sync crons
```sh
./kube-diff -s "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v121-green" -d "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v123-blue" --cron
``` 
5. to find diff 

```sh
./kube-diff -s "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v121-green" -d "/pd-box/docs/chapters/infra/k8s-configs/config.staging-eu-1-v123-blue" --compare
``` 

# to build 

```sh
make build_all
```
