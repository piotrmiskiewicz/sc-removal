package main

import (
	"context"
	helmclient "github.com/mittwald/go-helm-client"
	"helm.sh/helm/v3/pkg/storage/driver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
	helmClient        helmclient.Client
}

func NewCleaner(kubeConfigContent []byte) (*Cleaner, error) {
	var restConfig *rest.Config
	if len(kubeConfigContent) > 0 {
		var err error
		kubeconfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigContent)
		if err != nil {
			return nil, err
		}
		restConfig, err = kubeconfig.ClientConfig()
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	k8sCli, err := client.New(restConfig, client.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return nil, err
	}
	err = v1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	helmClient, err := helmclient.NewClientFromRestConf(&helmclient.RestConfClientOptions{
		Options: &helmclient.Options{
			Namespace:        "kyma-system",
			RepositoryConfig: "",
			RepositoryCache:  "",
			Debug:            false,
			Linting:          false,
			DebugLog:         nil,
		},
		RestConfig: restConfig,
	})
	if err != nil {
		return nil, err
	}

	return &Cleaner{
		k8sCli:            k8sCli,
		kubeConfigContent: kubeConfigContent,
		helmClient:        helmClient,
	}, nil
}

func (c *Cleaner) RemoveRelease(releaseName string) error {
	log.Printf("Looking for %s release...", releaseName)
	release, err := c.helmClient.GetRelease(releaseName)
	if err == driver.ErrReleaseNotFound {
		log.Printf("%s release not found, nothing to do", releaseName)
		return nil
	}
	if err != nil {
		return err
	}

	log.Printf("Found %s release in the namespace %s: status %s", release.Name, release.Namespace, release.Info.Status.String())
	log.Println(" Uninstalling...")
	err = c.helmClient.UninstallRelease(&helmclient.ChartSpec{
		ReleaseName:  releaseName,
		DisableHooks: true,
		Wait:         true,
		Timeout:      time.Minute,
		Force:        true,
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
			Kind:    "UsageKind",
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
			Group:   "servicecatalog.k8s.io",
			Version: "v1beta1",
		},
		{
			Kind:    "AddonsConfiguration",
			Group:   "addons.kyma-project.io",
			Version: "v1alpha1",
		},
		{
			Kind:    "ClusterAddonsConfiguration",
			Group:   "addons.kyma-project.io",
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

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ClusterServiceBroker",
		Group:   "servicecatalog.k8s.io",
		Version: "v1beta1",
	})
	err = c.k8sCli.DeleteAllOf(context.Background(), u, client.InNamespace(""))
	if err != nil {
		return err
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

func (c *Cleaner) PrepareSBUForRemoval() error {
	namespaces := &v1.NamespaceList{}
	err := c.k8sCli.List(context.Background(), namespaces)
	if err != nil {
		return err
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
		{
			Kind:    "UsageKind",
			Group:   "servicecatalog.kyma-project.io",
			Version: "v1alpha1",
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

func (c *Cleaner) RemnoveCRDs() error {
	crdsList := &apiextensions.CustomResourceDefinitionList{}

	err := c.k8sCli.List(context.Background(), crdsList)
	if err != nil {
		return err
	}

	for _, crd := range crdsList.Items {
		if crd.Spec.Group == "servicecatalog.k8s.io" || crd.Spec.Group == "servicecatalog.kyma-project.io" {
			log.Printf("Removing CRD %s", crd.Name)
			err := c.k8sCli.Delete(context.Background(), &crd)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
