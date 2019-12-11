/*
Copyright 2019 The Tekton Authors

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

package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/client-go/dynamic"

	"go.uber.org/zap"

	triggersv1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryclient "k8s.io/client-go/discovery"
)

// FindAPIResource returns the APIResource definition using the discovery client c.
func FindAPIResource(apiVersion, kind string, c discoveryclient.ServerResourcesInterface) (*metav1.APIResource, error) {
	resourceList, err := c.ServerResourcesForGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("error getting kubernetes server resources for apiVersion %s: %s", apiVersion, err)
	}
	for _, apiResource := range resourceList.APIResources {
		if apiResource.Kind != kind {
			continue
		}
		r := &apiResource
		// Resolve GroupVersion from parent list to have consistent resource identifiers.
		if r.Version == "" || r.Group == "" {
			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				return nil, fmt.Errorf("error parsing parsing GroupVersion: %v", err)
			}
			r.Group = gv.Group
			r.Version = gv.Version
		}
		return r, nil
	}
	return nil, fmt.Errorf("error could not find resource with apiVersion %s and kind %s", apiVersion, kind)
}

// Create uses the kubeClient to create the resource defined in the
// TriggerResourceTemplate and returns any errors with this process
func Create(logger *zap.SugaredLogger, rt json.RawMessage, triggerName, eventID, elName, elNamespace string, c discoveryclient.ServerResourcesInterface, dc dynamic.Interface) error {
	// Assume the TriggerResourceTemplate is valid (it has an apiVersion and Kind)
	data := new(unstructured.Unstructured)
	if err := data.UnmarshalJSON(rt); err != nil {
		return err
	}

	data = AddLabels(data, map[string]string{
		triggersv1.EventListenerLabelKey: elName,
		triggersv1.EventIDLabelKey:       eventID,
		triggersv1.TriggerLabelKey:       triggerName,
	})

	namespace := data.GetNamespace()
	// Default the resource creation to the EventListenerNamespace if not found in the resource template
	if namespace == "" {
		namespace = elNamespace
	}

	// Resolve resource kind to the underlying API Resource type.
	apiResource, err := FindAPIResource(data.GetAPIVersion(), data.GetKind(), c)
	if err != nil {
		return err
	}

	name := data.GetName()
	if name == "" {
		name = data.GetGenerateName()
	}
	logger.Infof("Generating resource: kind: %+v, name: %s", apiResource, name)

	gvr := schema.GroupVersionResource{
		Group:    apiResource.Group,
		Version:  apiResource.Version,
		Resource: apiResource.Name,
	}

	_, err = dc.Resource(gvr).Namespace(namespace).Create(data, metav1.CreateOptions{})
	return err
}

// AddLabels adds autogenerated Tekton labels to created resources.
func AddLabels(us *unstructured.Unstructured, labelsToAdd map[string]string) *unstructured.Unstructured {
	labels := us.GetLabels()
	for k, v := range labelsToAdd {
		l := fmt.Sprintf("%s/%s", triggersv1.GroupName, strings.TrimLeft(k, "/"))
		labels[l] = v
	}

	us.SetLabels(labels)
	return us
}
