

kind create cluster

kubectl apply -f hack/resources/crds

kubectl create ns kyma-system

helm install service-catalog hack/charts/service-catalog --namespace kyma-system --set asyncBindingOperationsEnabled=true

helm install pod-preset hack/charts/pod-preset --namespace kyma-system

# helm broker
helm install helm-broker hack/charts/helm-broker --namespace kyma-system

kubectl apply -f hack/resources/sample-addons.yaml

# wait for clusterserviceclasses
kubectl get clusterserviceclass

# create an instance and a binding
kubectl apply -f hack/resources/instance.yaml
kubectl apply -f hack/resources/binding.yaml

# check the binding status
kubectl get servicebinding

# the secret should be created
kubectl get secret testing

# install SBU controller
helm install service-catalog-addons hack/charts/service-catalog-addons --namespace kyma-system

kubectl apply -f hack/resources/sample-app.yaml

# check logs from sample app (there is no CONFIG_MAP_NAME)
kubectl logs -l app=sample

kubectl apply -f hack/resources/service-binding-usage.yaml

#check the status of SBU
kubectl get servicebindingusage -o yaml

# if ready, check logs from sample app (CONFIG_MAP_NAME must be present in the logs)
kubectl logs -l app=sample

# Run the tool
go run main.go cleaner.go

# the secret must still exists
kubectl get secret testing

# sample app still have CONFIG_MAP_NAME injected
kubectl logs -l app=sample