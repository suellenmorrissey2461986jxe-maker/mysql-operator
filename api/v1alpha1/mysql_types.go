/*
Copyright 2026.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MySQLSpec defines the desired state of MySQL.
// 用户希望MySQL最终变成什么样，填写在spec中。
type MySQLSpec struct {
	// DatabaseName is the database created for the application.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	DatabaseName string `json:"databaseName"`

	// Image is the MySQL container image.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="mysql:8.4"
	Image string `json:"image,omitempty"`

	// Replicas is the desired number of MySQL Pods.
	// The current single-instance architecture supports exactly one replica.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`

	// StorageSize is the requested persistent volume size.
	// +optional
	// +kubebuilder:default="1Gi"
	StorageSize resource.Quantity `json:"storageSize,omitempty"`
}

// MySQLStatus defines the observed state of MySQL.
// Controller实际观察到的运行状态，写入status中。
type MySQLStatus struct {
	// Phase is the current lifecycle phase.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Creating;Running;Degraded
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of ready MySQL Pods.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Message provides a human-readable status description.
	// +optional
	Message string `json:"message,omitempty"`

	// ObservedGeneration records which spec generation was processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent detailed resource conditions.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.databaseName`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MySQL is the Schema for the mysqls API.
type MySQL struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MySQL.
	// +required
	Spec MySQLSpec `json:"spec"`

	// status defines the observed state of MySQL.
	// +optional
	Status MySQLStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MySQLList contains a list of MySQL.
type MySQLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MySQL `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &MySQL{}, &MySQLList{})
		return nil
	})
}
