package main

import (
	"io/ioutil"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"log"
	"os"
	"time"
)

/*
The application expects environment varialbe "KUBECONFIG" to be set, then uninstalls Service Catalog and removes all SC resources.
*/
func main() {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config"
	}

	log.Printf("Using kubeconfig file: %s", kubeconfigPath)

	// read the kubeconfig
	kcContent, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		panic(err)
	}

	cleaner, err := NewCleaner(kcContent)
	if err != nil {
		panic(err)
	}

	log.Println("Removing Service Catalog release")
	cleaner.RemoveRelease(ServiceCatalogReleaseName)

	log.Println("Removing service-catalog-addons release")
	cleaner.RemoveRelease(ServiceCatalogAddonsReleaseName)

	log.Println("Removing Helm Broker release")
	cleaner.RemoveRelease(HelmBrokerReleaseName)
	time.Sleep(2 * time.Second)

	log.Println()
	log.Println("Removing finalizers")
	err = cleaner.PrepareForRemoval()
	if err != nil {
		panic(err)
	}

	time.Sleep(2 * time.Second)

	log.Println()
	log.Println("Deleting resources")
	err = cleaner.RemoveResources()
	if err != nil {
		panic(err)
	}

	log.Println("Deleting CRDs")
	err = cleaner.RemnoveCRDs()
	if err != nil {
		panic(err)
	}



}
