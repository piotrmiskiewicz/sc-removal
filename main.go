package main

import (
	"io/ioutil"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"log"
	"os"
	"path"
	"time"
)

const CommandSBUPrepare = "sbu-prepare"
const CommandFinalClean = "final-clean"
/*
The application expects environment varialbe "KUBECONFIG" to be set, then uninstalls Service Catalog and removes all SC resources.
*/
func main() {
	command := CommandFinalClean
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		kubeconfigPath = path.Join(home, ".kube/config")
	}

	log.Printf("Using kubeconfig file: %s", kubeconfigPath)

	// read the kubeconfig
	kcContent, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// empty kubeconfig content means - use "in cluster config"
			log.Println("Kubeconfig does not exists, using in-cluster config")
			kcContent = []byte{}
		} else {
			panic(err)
		}
	}

	cleaner, err := NewCleaner(kcContent)
	if err != nil {
		panic(err)
	}

	if command == CommandSBUPrepare {
		log.Println("Removing service-catalog-addons release")
		err = cleaner.RemoveRelease(ServiceCatalogAddonsReleaseName)
		if err != nil {
			panic(err)
		}

		log.Println("Removing finalizers and ownerreferences from SBU")
		err = cleaner.PrepareSBUForRemoval()
		if err != nil {
			panic(err)
		}

		return
	}

	if command == CommandFinalClean {
		log.Println("Removing Service Catalog release")
		err = cleaner.RemoveRelease(ServiceCatalogReleaseName)
		if err != nil {
			panic(err)
		}

		log.Println("Removing Helm Broker release")
		err = cleaner.RemoveRelease(HelmBrokerReleaseName)
		if err != nil {
			panic(err)
		}

		time.Sleep(10 * time.Second)
		log.Println("Removing finalizers")
		err = cleaner.PrepareForRemoval()
		if err != nil {
			panic(err)
		}

		time.Sleep(4 * time.Second)

		log.Println()
		log.Println("Deleting resources")
		if command == CommandFinalClean {
			err = cleaner.RemoveResources()
			if err != nil {
				panic(err)
			}
		}

		log.Println("Deleting CRDs")
		err = cleaner.RemnoveCRDs()
		if err != nil {
			panic(err)
		}

		return
	}

}
