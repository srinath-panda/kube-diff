
1. pd-devops/charts
```sh 
Make mirror -src -dst 
```

2. take the latest kube-diff tool and downlaod the latest workers list in pd-devops/script


3. Sync the all the apps between old and new cluster
```sh
./kube-diff -s <src-cluster> -d <dst-cluster> -w <worker_list> --deploy --cron 
```

4. 
  -- update image/tag for all except workers managed in pd-app-charts
  -- scale up replica for non workers
  -- unpause scaledobject for non workers 

```sh
./kube-diff -s <src-cluster> -d <dst-cluster> -w <worker_list> --mirror
```

## steps during traffic switch 
5. take the latest kube-diff tool and downlaod the latest workers list in pd-devops/script

6. switch traffic to 5%, check if ther are errors

7. switch traffic to 20%, check if there are errors 

8. save the state of the old cluster before the traffic switch -- mandatory for next step 
```sh
./kube-diff -s <src-cluster> -d <dst-cluster> -w <worker_list> --savestate
```

9. Ask Fausto to deploy the pr for disabling the workers in new cluster, make sure all the workers are scaled up and is failing

10. enable all the workers in the new cluster and downscale it in old cluster and change the image for workers managed by pd-app-charts
```sh
./kube-diff -s <src-cluster> -d <dst-cluster> -w <worker_list> --enableworkers
```

11. wait for 30 mins and monitor all th datadog 

12. switch traffic to 50%, check if there are errors 

13. enable cron 

14. wait for 30 mins and monitor all th datadog 

15. switch traffic to 100%, check if there are errors 

16. change the live tag for datadog and vector and go to argo force sync the changes
