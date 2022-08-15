/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package create

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	secretForSaLong = templates.LongDesc(i18n.T(`
		Create a new secret for use in Service Accounts as a token.`))

	secretForSaExample = templates.Examples(i18n.T(`
		  # If you don't already have a .dockercfg file, you can create a dockercfg secret directly by using:
		  kubectl create secret token-sa my-secret --serviceaccount=serviceaccount`))
)

// CreateSecretTokenSaOptions holds the options for 'create secret docker-registry' sub command
type CreateSecretTokenSaOptions struct {
	// PrintFlags holds options necessary for obtaining a printer
	PrintFlags *genericclioptions.PrintFlags
	PrintObj   func(obj runtime.Object) error

	// Name of secret (required)
	Name string
	// Service Account for token (required)
	ServiceAccount string

	FieldManager     string
	CreateAnnotation bool
	Namespace        string
	Annotations      []string
	EnforceNamespace bool

	Client              corev1client.CoreV1Interface
	DryRunStrategy      cmdutil.DryRunStrategy
	DryRunVerifier      *resource.QueryParamVerifier
	ValidationDirective string

	genericclioptions.IOStreams
}

// NewSecretDockerRegistryOptions creates a new *CreateSecretTokenSaOptions with default value
func NewSecretSaOptions(ioStreams genericclioptions.IOStreams) *CreateSecretTokenSaOptions {
	return &CreateSecretTokenSaOptions{
		ServiceAccount:     "test-sa",
		PrintFlags: genericclioptions.NewPrintFlags("created").WithTypeSetter(scheme.Scheme),
		IOStreams:  ioStreams,
	}
}

// NewSecretSaOptions is a macro command for creating secrets to work with Docker registries
func NewCmdCreateSa(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewSecretSaOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "token-sa NAME --serviceaccount=serviceaccount",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create a secret token for service account"),
		Long:                  secretForSaLong,
		Example:               secretForSaExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run())
		},
	}

	o.PrintFlags.AddFlags(cmd)

	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddDryRunFlag(cmd)

	cmd.Flags().StringVar(&o.ServiceAccount, "serviceaccount", o.ServiceAccount, i18n.T("ServiceAccount that will create token"))
	cmdutil.AddFieldManagerFlagVar(cmd, &o.FieldManager, "kubectl-create")

	return cmd
}

// Complete loads data from the command line environment
func (o *CreateSecretTokenSaOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	var err error
	o.Name, err = NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}

	restConfig, err := f.ToRESTConfig()
	if err != nil {
		return err
	}

	o.Client, err = corev1client.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	o.CreateAnnotation = cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag)

	o.DryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}

	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return err
	}

	discoveryClient, err := f.ToDiscoveryClient()
	if err != nil {
		return err
	}

	o.DryRunVerifier = resource.NewQueryParamVerifier(dynamicClient, discoveryClient, resource.QueryParamDryRun)

	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.DryRunStrategy)
	printer, err := o.PrintFlags.ToPrinter()
	if err != nil {
		return err
	}

	o.PrintObj = func(obj runtime.Object) error {
		return printer.PrintObj(obj, o.Out)
	}

	o.ValidationDirective, err = cmdutil.GetValidationDirective(cmd)
	if err != nil {
		return err
	}

	return nil
}

// Validate checks if CreateSecretTokenSaOptions has sufficient value to run
func (o *CreateSecretTokenSaOptions) Validate() error {
	if len(o.Name) == 0 {
		return fmt.Errorf("name must be specified")
	}
	if (len(o.ServiceAccount) == 0) {
		return fmt.Errorf("either --serviceaccount is required")
	}
	return nil
}

// Run calls createSecretDockerRegistry which will create secretDockerRegistry based on CreateSecretTokenSaOptions
// and makes an API call to the server
func (o *CreateSecretTokenSaOptions) Run() error {
	secretSa, err := o.createSecretSa()
	if err != nil {
		return err
	}
	err = util.CreateOrUpdateAnnotation(o.CreateAnnotation, secretSa, scheme.DefaultJSONEncoder())
	if err != nil {
		return err
	}
	if o.DryRunStrategy != cmdutil.DryRunClient {
		createOptions := metav1.CreateOptions{}
		if o.FieldManager != "" {
			createOptions.FieldManager = o.FieldManager
		}
		createOptions.FieldValidation = o.ValidationDirective
		if o.DryRunStrategy == cmdutil.DryRunServer {
			err := o.DryRunVerifier.HasSupport(secretSa.GroupVersionKind())
			if err != nil {
				return err
			}
			createOptions.DryRun = []string{metav1.DryRunAll}
		}
		secretSa, err = o.Client.Secrets(o.Namespace).Create(context.TODO(), secretSa, createOptions)
		if err != nil {
			return fmt.Errorf("failed to create secret %v", err)
		}
	}

	return o.PrintObj(secretSa)
}

// createSecretDockerRegistry fills in key value pair from the information given in
// CreateSecretDockerRegistryOptions into *corev1.Secret
func (o *CreateSecretTokenSaOptions) createSecretSa() (*corev1.Secret, error) {
	namespace := ""
	if o.EnforceNamespace {
		namespace = o.Namespace
	}
	annotations := o.buildAnnotations()
	secretSa := newSecretObjToken(o.Name, namespace, corev1.SecretTypeServiceAccountToken, annotations)

	return secretSa, nil
}

func (o *CreateSecretTokenSaOptions) buildAnnotations() map[string]string {
	var annotations = make(map[string]string)
    annotations["kubernetes.io/service-account.name"] = o.ServiceAccount
	return annotations
}