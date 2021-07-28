package main

import (
	"context"
	helmclient "github.com/mittwald/go-helm-client"
	"helm.sh/helm/v3/pkg/storage/driver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	"github.com/kubernetes-sigs/service-catalog/pkg/apis/servicecatalog/v1beta1"
)

const (
	HelmBrokerReleaseName           = "helm-broker"
	ServiceCatalogAddonsReleaseName = "service-catalog-addons"
	ServiceCatalogReleaseName       = "service-catalog"
)

type Cleaner struct {
	k8sCli            client.Client
	kubeConfigContent []byte
}

func NewCleaner(kubeConfigContent []byte) (*Cleaner, error) {
	kubeconfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigContent)
	if err != nil {
		return nil, err
	}

	rc, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	k8sCli, err := client.New(rc, client.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return nil, err
	}
	err = v1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	return &Cleaner{
		k8sCli:            k8sCli,
		kubeConfigContent: kubeConfigContent,
	}, nil
}

func (c *Cleaner) RemoveRelease(releaseName string) error {
	helmCli, err := helmclient.NewClientFromKubeConf(&helmclient.KubeConfClientOptions{
		Options: &helmclient.Options{
			Namespace: "kyma-system",
		},
		KubeConfig: c.kubeConfigContent,
	})

	log.Println("Looking for Service Catalog release...")
	release, err := helmCli.GetRelease(releaseName)
	if err == driver.ErrReleaseNotFound {
		log.Println("service-catalog release not found, nothing to do")
		return nil
	}
	if err != nil {
		return err
	}

	log.Printf("Found %s release in the namespace %s: status %s", release.Name, release.Namespace, release.Info.Status.String())
	log.Println(" Uninstalling...")
	err = helmCli.UninstallRelease(&helmclient.ChartSpec{
		ReleaseName: releaseName,
		Timeout:     time.Minute,
		Wait:        true,
	})
	if err != nil {
		return err
	}

	log.Println("DONE")
	return nil
}

func (c *Cleaner) RemoveResources() error {
	gvkList := []schema.GroupVersionKind{
		{
			Kind:    "ServiceBindingUsage",
			Group:   "servicecatalog.kyma-project.io",
			Version: "v1alpha1",
		},
		{
			Kind:    "ServiceBinding",
			Group:   "servicecatalog.k8s.io",
			Version: "v1beta1",
		},
		{
			Kind:    "ServiceInstance",
			Group:   "servicecatalog.k8s.io",
			Version: "v1beta1",
		},
		{
			Kind:    "ServiceBroker",
			Group:   "servicecatalog.kyma-project.io",
			Version: "v1alpha1",
		},
		{
			Kind:    "ClusterServiceBroker",
			Group:   "servicecatalog.kyma-project.io",
			Version: "v1alpha1",
		},
	}

	namespaces := &v1.NamespaceList{}
	err := c.k8sCli.List(context.Background(), namespaces)
	if err != nil {
		return err
	}

	for _, gvk := range gvkList {
		for _, namespace := range namespaces.Items {
			log.Printf("%ss in %s\n", gvk.Kind, namespace.Name)
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(gvk)
			err := c.k8sCli.DeleteAllOf(context.Background(), u, client.InNamespace(namespace.Name))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Cleaner) removeFinalizers(gvk schema.GroupVersionKind, ns string) error {
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(gvk)
	err := c.k8sCli.List(context.Background(), ul, client.InNamespace(ns))
	if err != nil {
		return err
	}

	for _, obj := range ul.Items {
		obj.SetFinalizers([]string{})
		err := c.k8sCli.Update(context.Background(), &obj)
		if err != nil {
			return err
		}
		log.Printf("%s %s/%s: finalizers removed", gvk.Kind, ns, obj.GetName())
	}

	return nil
}

func (c *Cleaner) PrepareForRemoval() error {
	// listing

	namespaces := &v1.NamespaceList{}
	err := c.k8sCli.List(context.Background(), namespaces)
	if err != nil {
		return err
	}

	gvkList := []schema.GroupVersionKind{
		{
			Group:   "servicecatalog.k8s.io",
			Kind:    "ServiceBindingList",
			Version: "v1beta1",
		},
		{
			Group:   "servicecatalog.k8s.io",
			Kind:    "ServiceInstanceList",
			Version: "v1beta1",
		},
		{
			Group:   "servicecatalog.k8s.io",
			Kind:    "ServiceBrokerList",
			Version: "v1beta1",
		},
		{
			Group:   "servicecatalog.k8s.io",
			Kind:    "ClusterServiceBrokerList",
			Version: "v1beta1",
		},
	}

	for _, gvk := range gvkList {
		for _, ns := range namespaces.Items {
			err := c.removeFinalizers(gvk, ns.Name)
			if err != nil {
				return err
			}
		}
	}

	for _, ns := range namespaces.Items {
		ul := &unstructured.UnstructuredList{}
		ul.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "ServiceBindingUsage",
			Group:   "servicecatalog.kyma-project.io",
			Version: "v1alpha1",
		})
		err := c.k8sCli.List(context.Background(), ul, client.InNamespace(ns.Name))
		if err != nil {
			return err
		}

		for _, sbu := range ul.Items {
			log.Printf("Removing owner reference from SBU %s/%s", sbu.GetNamespace(), sbu.GetName())
			sbu.SetOwnerReferences([]metav1.OwnerReference{})
			err := c.k8sCli.Update(context.Background(), &sbu)
			if err != nil {
				return err
			}
		}
	}

	log.Println("ServiceBindings secrets owner references")
	var bindings = &v1beta1.ServiceBindingList{}
	err = c.k8sCli.List(context.Background(), bindings, client.InNamespace(""))
	if err != nil {
		return err
	}
	for _, item := range bindings.Items {
		log.Printf("%s/%s", item.Namespace, item.Name)
		item.Finalizers = []string{}
		err := c.k8sCli.Update(context.Background(), &item)
		if err != nil {
			return err
		}

		// find linked secrets
		var secret = &v1.Secret{}
		err = c.k8sCli.Get(context.Background(), client.ObjectKey{
			Namespace: item.Namespace,
			Name:      item.Spec.SecretName,
		}, secret)
		if err != nil {
			return err
		}

		secret.OwnerReferences = []metav1.OwnerReference{}
		err = c.k8sCli.Update(context.Background(), secret)
		if err != nil {
			return err
		}
	}

	return nil
}
