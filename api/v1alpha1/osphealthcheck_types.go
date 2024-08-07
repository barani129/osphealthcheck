/*
Copyright 2024 baranitharan.chittharanjan@spark.co.nz.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	EventSource                 = "Osphealthcheck"
	EventReasonIssuerReconciler = "OsphealthcheckReconciler"
)

// OsphealthcheckSpec defines the desired state of Osphealthcheck
type OsphealthcheckSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Set suspend to true to disable monitoring the custom resource
	Suspend *bool `json:"suspend"`

	// Suspends email alerts if set to true, target email (.spec.email) will not be notified
	SuspendEmailAlert *bool `json:"suspendEmailAlert,omitempty"`

	// Target user's email for container status notification
	Email string `json:"email,omitempty"`

	// SMTP Relay host for sending the email
	RelayHost string `json:"relayHost,omitempty"`

	// To notify the external alerting system, boolean (true, false). Set true to notify the external system.
	NotifyExtenal *bool `json:"notifyExternal,omitempty"`

	// URL of the external alert system
	ExternalURL string `json:"externalURL,omitempty"`

	// Data to be sent to the external system in the form of config map
	ExternalData string `json:"externalData,omitempty"`

	// Secret which has the username and password to post the alert notification to the external system
	ExternalSecret string `json:"externalSecret,omitempty"`

	// the frequency of checks to be done, if not set, defaults to 2 minutes
	CheckInterval *int64 `json:"checkInterval,omitempty"`
}

// OsphealthcheckStatus defines the observed state of Osphealthcheck
type OsphealthcheckStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// list of status conditions to indicate the status of managed cluster
	// known conditions are 'Ready'.
	// +optional
	Conditions []OsphealthcheckCondition `json:"conditions,omitempty"`

	// last successful timestamp of retrieved cluster status
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// Indicates if external alerting system is notified
	// +optional
	ExternalNotified bool `json:"externalNotified,omitempty"`

	// Indicates the timestamp when external alerting system is notified
	// +optional
	ExternalNotifiedTime *metav1.Time `json:"externalNotifiedTime,omitempty"`

	// affected targets
	// +optional
	FailedChecks []string `json:"failedChecks,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// Osphealthcheck is the Schema for the osphealthchecks API
type Osphealthcheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OsphealthcheckSpec   `json:"spec,omitempty"`
	Status OsphealthcheckStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OsphealthcheckList contains a list of Osphealthcheck
type OsphealthcheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Osphealthcheck `json:"items"`
}

type OsphealthcheckCondition struct {
	// Type of the condition, known values are 'Ready'.
	Type OsphealthcheckConditionType `json:"type"`

	// Status of the condition, one of ('True', 'False', 'Unknown')
	Status ConditionStatus `json:"status"`

	// LastTransitionTime is the timestamp of the last update to the status
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is the machine readable explanation for object's condition
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is the human readable explanation for object's condition
	Message string `json:"message"`
}

// ManagedConditionType represents a managed cluster condition value.
type OsphealthcheckConditionType string

const (
	// ContainerScanConditionReady represents the fact that a given managed cluster condition
	// is in reachable from the ACM/source cluster.
	// If the `status` of this condition is `False`, managed cluster is unreachable
	OsphealthcheckConditionReady OsphealthcheckConditionType = "Ready"
)

// ConditionStatus represents a condition's status.
// +kubebuilder:validation:Enum=True;False;Unknown
type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in
// the condition; "ConditionFalse" means a resource is not in the condition;
// "ConditionUnknown" means kubernetes can't decide if a resource is in the
// condition or not. In the future, we could add other intermediate
// conditions, e.g. ConditionDegraded.
const (
	// ConditionTrue represents the fact that a given condition is true
	ConditionTrue ConditionStatus = "True"

	// ConditionFalse represents the fact that a given condition is false
	ConditionFalse ConditionStatus = "False"

	// ConditionUnknown represents the fact that a given condition is unknown
	ConditionUnknown ConditionStatus = "Unknown"
)

func init() {
	SchemeBuilder.Register(&Osphealthcheck{}, &OsphealthcheckList{})
}
