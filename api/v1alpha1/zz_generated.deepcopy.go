//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Osphealthcheck) DeepCopyInto(out *Osphealthcheck) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Osphealthcheck.
func (in *Osphealthcheck) DeepCopy() *Osphealthcheck {
	if in == nil {
		return nil
	}
	out := new(Osphealthcheck)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Osphealthcheck) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OsphealthcheckCondition) DeepCopyInto(out *OsphealthcheckCondition) {
	*out = *in
	if in.LastTransitionTime != nil {
		in, out := &in.LastTransitionTime, &out.LastTransitionTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OsphealthcheckCondition.
func (in *OsphealthcheckCondition) DeepCopy() *OsphealthcheckCondition {
	if in == nil {
		return nil
	}
	out := new(OsphealthcheckCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OsphealthcheckList) DeepCopyInto(out *OsphealthcheckList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Osphealthcheck, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OsphealthcheckList.
func (in *OsphealthcheckList) DeepCopy() *OsphealthcheckList {
	if in == nil {
		return nil
	}
	out := new(OsphealthcheckList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OsphealthcheckList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OsphealthcheckSpec) DeepCopyInto(out *OsphealthcheckSpec) {
	*out = *in
	if in.Suspend != nil {
		in, out := &in.Suspend, &out.Suspend
		*out = new(bool)
		**out = **in
	}
	if in.SuspendEmailAlert != nil {
		in, out := &in.SuspendEmailAlert, &out.SuspendEmailAlert
		*out = new(bool)
		**out = **in
	}
	if in.NotifyExtenal != nil {
		in, out := &in.NotifyExtenal, &out.NotifyExtenal
		*out = new(bool)
		**out = **in
	}
	if in.CheckInterval != nil {
		in, out := &in.CheckInterval, &out.CheckInterval
		*out = new(int64)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OsphealthcheckSpec.
func (in *OsphealthcheckSpec) DeepCopy() *OsphealthcheckSpec {
	if in == nil {
		return nil
	}
	out := new(OsphealthcheckSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OsphealthcheckStatus) DeepCopyInto(out *OsphealthcheckStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]OsphealthcheckCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastRunTime != nil {
		in, out := &in.LastRunTime, &out.LastRunTime
		*out = (*in).DeepCopy()
	}
	if in.LastSuccessfulRunTime != nil {
		in, out := &in.LastSuccessfulRunTime, &out.LastSuccessfulRunTime
		*out = (*in).DeepCopy()
	}
	if in.ExternalNotifiedTime != nil {
		in, out := &in.ExternalNotifiedTime, &out.ExternalNotifiedTime
		*out = (*in).DeepCopy()
	}
	if in.FailedChecks != nil {
		in, out := &in.FailedChecks, &out.FailedChecks
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OsphealthcheckStatus.
func (in *OsphealthcheckStatus) DeepCopy() *OsphealthcheckStatus {
	if in == nil {
		return nil
	}
	out := new(OsphealthcheckStatus)
	in.DeepCopyInto(out)
	return out
}
