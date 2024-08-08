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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	kubev1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/barani129/osphealthcheck/api/v1alpha1"
	"github.com/barani129/osphealthcheck/internal/Osphealthcheck/util"
)

var (
	errGetAuthSecret    = errors.New("failed to get Secret containing External alert system credentials")
	errGetAuthConfigMap = errors.New("failed to get ConfigMap containing the data to be sent to the external alert system")
)

// OsphealthcheckReconciler reconciles a Osphealthcheck object
type OsphealthcheckReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	Kind                     string
	ClusterResourceNamespace string
	recorder                 record.EventRecorder
}

func (r *OsphealthcheckReconciler) newIssuer() (client.Object, error) {
	OsphealthcheckKind := monitoringv1alpha1.GroupVersion.WithKind(r.Kind)
	ro, err := r.Scheme.New(OsphealthcheckKind)
	if err != nil {
		return nil, err
	}
	return ro.(client.Object), nil
}

//+kubebuilder:rbac:groups=monitoring.spark.co.nz,resources=osphealthchecks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.spark.co.nz,resources=osphealthchecks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=monitoring.spark.co.nz,resources=osphealthchecks/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/proxy,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/portforward,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/attach,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Osphealthcheck object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *OsphealthcheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = log.FromContext(ctx)

	checker, err := r.newIssuer()
	if err != nil {
		log.Log.Error(err, "unrecognized osphealtchecker type")
		return ctrl.Result{}, err
	}
	if err = r.Get(ctx, req.NamespacedName, checker); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, fmt.Errorf("unexpected get error: %v", err)
		}
		log.Log.Info("osphealthchecker is not found")
		return ctrl.Result{}, nil
	}
	spec, status, err := util.GetSpecAndStatus(checker)
	if err != nil {
		log.Log.Error(err, "unexpected error while trying to get osphealthcheck spec and status")
		return ctrl.Result{}, err
	}
	if spec.Suspend != nil && *spec.Suspend {
		log.Log.Info("osphealthcheck is suspended, exiting.")
		return ctrl.Result{}, nil
	}
	secretName := types.NamespacedName{
		Name: spec.ExternalSecret,
	}

	configmapName := types.NamespacedName{
		Name: spec.ExternalData,
	}

	switch checker.(type) {
	case *monitoringv1alpha1.Osphealthcheck:
		secretName.Namespace = r.ClusterResourceNamespace
		configmapName.Namespace = r.ClusterResourceNamespace
	default:
		log.Log.Error(fmt.Errorf("unexpected issuer type: %s", checker), "not retrying")
		return ctrl.Result{}, nil
	}

	var secret corev1.Secret
	var configmap corev1.ConfigMap
	var username []byte
	var password []byte
	var data map[string]string

	if spec.NotifyExtenal != nil && *spec.NotifyExtenal {
		if err := r.Get(ctx, secretName, &secret); err != nil {
			return ctrl.Result{}, fmt.Errorf("%w, secret name: %s, reason: %v", errGetAuthSecret, secretName, err)
		}
		username = secret.Data["username"]
		password = secret.Data["password"]
		if err := r.Get(ctx, configmapName, &configmap); err != nil {
			return ctrl.Result{}, fmt.Errorf("%w, configmap name: %s, reason: %v", errGetAuthConfigMap, configmapName, err)
		}
		data = configmap.Data
	}

	// report gives feedback by updating the Ready condition of the Port scan
	report := func(conditionStatus monitoringv1alpha1.ConditionStatus, message string, err error) {
		eventType := corev1.EventTypeNormal
		if err != nil {
			log.Log.Error(err, message)
			eventType = corev1.EventTypeWarning
			message = fmt.Sprintf("%s: %v", message, err)
		} else {
			log.Log.Info(message)
		}
		r.recorder.Event(checker, eventType, monitoringv1alpha1.EventReasonIssuerReconciler, message)
		util.SetReadyCondition(status, conditionStatus, monitoringv1alpha1.EventReasonIssuerReconciler, message)
	}

	defer func() {
		if err != nil {
			report(monitoringv1alpha1.ConditionFalse, "One or more healthchecks are failing", err)
		}
		if updateErr := r.Status().Update(ctx, checker); updateErr != nil {
			err = utilerrors.NewAggregate([]error{err, updateErr})
			result = ctrl.Result{}
		}
	}()

	if ready := util.GetReadyCondition(status); ready == nil {
		report(monitoringv1alpha1.ConditionUnknown, "First Seen", nil)
		return ctrl.Result{}, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get in cluster configuration due to error %s", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get in cluster configuration due to error %s", err)
	}
	_, err = clientset.CoreV1().Namespaces().Get(context.Background(), "openstack", v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("openstack namespace is not found, not running on openstack ctlplane on openshift cluster")
	}
	clientpod, err := clientset.CoreV1().Pods("openstack").Get(context.Background(), "openstackclient", v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("openstackclient pod is not found, exiting")
	}
	if clientpod.Status.Phase != "Running" {
		return ctrl.Result{}, fmt.Errorf("openstackclient pod is not running, unable to execute openstack healthcheck at this moment")
	}

	externalVIPs := make(map[string]string)
	cm, err := clientset.CoreV1().ConfigMaps("openstack").Get(context.Background(), "tripleo-deploy-config-default", v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("unable to fetch configmap tripleo-deploy-config-default")
	}
	con := cm.Data["rendered-tripleo-config.yaml"]
	conSl := strings.Split(con, "\n")
	var externalIP []string
	for _, val := range conSl {
		if strings.Contains(val, "ExternalNetworkVip:") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs["externalVIP"] = fmt.Sprintf("ping -c 3 %s", val2[1])
			externalIP = append(externalIP, val2[1])
		} else if strings.Contains(val, "InternalApiNetworkVip:") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs["internalVIP"] = fmt.Sprintf("ping -c 3 %s", val2[1])

		} else if strings.Contains(val, "ControlPlaneIP") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs["ctlplaneVIP"] = fmt.Sprintf("ping -c 3 %s", val2[1])
		}
	}
	vmList := kubev1.VirtualMachineInstanceList{}
	err = clientset.RESTClient().Get().AbsPath("/apis/kubevirt.io/v1/namespaces/openstack/virtualmachineinstances").Do(context.Background()).Into(&vmList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch virtualmachine instance list")
	}
	var activeVM []string
	var nonActiveVM []string
	for _, vm := range vmList.Items {
		if vm.Status.Phase == "Running" {
			activeVM = append(activeVM, vm.Name)
		} else {
			nonActiveVM = append(nonActiveVM, vm.Name)
		}
	}
	var defaultHealthCheckInterval time.Duration
	if spec.CheckInterval != nil {
		defaultHealthCheckInterval = time.Minute * time.Duration(*spec.CheckInterval)
	} else {
		defaultHealthCheckInterval = time.Minute * 30
	}

	if status.LastRunTime == nil {
		var errSlice []string
		log.Log.Info(fmt.Sprintf("starting openstack healthchecks in cluster %s", config.Host))
		log.Log.Info("Modifying ssh config file permission to avoid openstack command execution failure after openstackclient pod restarts")
		_, err := util.ExecuteCommand("chmod 644 /home/cloud-admin/.ssh/config", clientset, config)
		if err != nil {
			if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
				util.SendEmailAlert("openstackclient", fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"), spec, "modifyfileperm")

			}
			if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
				util.NotifyExternalSystem(data, "firing", "modifyfileperm", "openstackclient", spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", "modifyfileperm", "openstackclient"), status)
			}
			status.FailedChecks = append(status.FailedChecks, "failed to modify cloud-admin ssh file permission")
			return ctrl.Result{}, fmt.Errorf("unable to modify file permission of /home/cloud-admin/.ssh/config in openstackclient pod, exiting as subsequent healtchecks might fail")
		}
		log.Log.Info("Checking virtual machine instances in Openstack namespace")
		if len(nonActiveVM) > 0 {
			for _, vm := range nonActiveVM {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"), spec, vm)
				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					util.NotifyExternalSystem(data, "firing", "found a non running vm", vm, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", vm, "non-active"), status)
				}
			}
			errSlice = append(errSlice, "non running controller vms")
		}
		log.Log.Info("Check pcs status")
		pcsErr, err := util.CheckPcsStatus(activeVM[0], clientset, config)
		if err != nil {
			log.Log.Error(err, "failed to execute pcs status command ")
		}
		if len(pcsErr) > 0 {
			if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
				for _, err := range pcsErr {
					util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", err, "pcs"), spec, err)
				}
			}
			if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
				for _, err := range pcsErr {
					util.NotifyExternalSystem(data, "firing", err, activeVM[0], spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", err, "pcs"), status)
				}
			}
			errSlice = append(errSlice, "pcs errors")
		}
		log.Log.Info("Check pcs stonith")
		stonith, err := util.CheckPcsStonith(activeVM[0], clientset, config)
		if err != nil {
			log.Log.Error(err, "failed to execute pcs stonith check command")
		}
		if stonith {
			if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
				util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"), spec, "pcs stonith is disalbed")
			}
			if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
				util.NotifyExternalSystem(data, "firing", "pcs stonith is disabled", activeVM[0], spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", "checkstonith", "pcs"), status)
			}
			status.FailedChecks = append(status.FailedChecks, "stonith is disabled, please ignore if it is intended")
			errSlice = append(errSlice, "stonith is disabled")
		}
		log.Log.Info("Check Openstack Compute Service")
		hostsErr, err := util.CheckComputeService(clientset, config)
		if err != nil {
			log.Log.Error(err, "failed to execute openstack compute service command ")
		}
		if len(hostsErr) > 0 {
			for _, host := range hostsErr {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(host, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"), spec, "compute-service")
				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					util.NotifyExternalSystem(data, "firing", "openstack compute service is down", host, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", host, "compute-service"), status)
				}
				status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host))
			}
			errSlice = append(errSlice, "hosts with nova down")
		}
		log.Log.Info("Check Openstack network agents")
		netErr, err := util.CheckNetworkAgents(clientset, config)
		if err != nil {
			log.Log.Error(err, "failed to execute openstack network agent list command")
		}
		if len(netErr) > 0 {
			for _, host := range netErr {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(host, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"), spec, "network-agent")
				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					util.NotifyExternalSystem(data, "firing", "openstack network service is down", host, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", host, "networkagent"), status)
				}
				status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host))
			}
			errSlice = append(errSlice, "hosts with network down")
		}
		if len(errSlice) < 1 {
			status.Healthy = true
			now := v1.Now()
			status.LastSuccessfulRunTime = &now
		} else {
			status.Healthy = false
		}
	} else {
		pastTime := time.Now().Add(-1 * defaultHealthCheckInterval)
		timeDiff := status.LastRunTime.Time.Before(pastTime)
		if timeDiff {
			var errSlice []string
			log.Log.Info(fmt.Sprintf("starting openstack healthchecks in cluster %s", config.Host))
			log.Log.Info("Modifying ssh config file permission to avoid openstack command execution failure after openstackclient pod restarts")
			_, err := util.ExecuteCommand("chmod 644 /home/cloud-admin/.ssh/config", clientset, config)
			if err != nil {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert("openstackclient", fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"), spec, "modifyfileperm")

				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					util.SubNotifyExternalSystem(data, "firing", "modifyfileperm", "openstackclient", spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", "modifyfileperm", "openstackclient"), status)
				}
				status.FailedChecks = append(status.FailedChecks, "failed to modify cloud-admin ssh file permission")
				return ctrl.Result{}, fmt.Errorf("unable to modify file permission of /home/cloud-admin/.ssh/config in openstackclient pod, exiting as subsequent healtchecks might fail")
			}
			log.Log.Info("Checking virtual machine instances in Openstack namespace")
			if len(nonActiveVM) > 0 {
				for _, vm := range nonActiveVM {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"), spec, vm)
					}
					if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
						util.SubNotifyExternalSystem(data, "firing", "found a non running vm", vm, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", vm, "non-active"), status)
					}
				}
				errSlice = append(errSlice, "non running controller vms")
			}
			log.Log.Info("Check pcs status")
			pcsErr, err := util.CheckPcsStatus(activeVM[0], clientset, config)
			if err != nil {
				log.Log.Error(err, "failed to execute pcs status command ")
			}
			if len(pcsErr) > 0 {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					for _, err := range pcsErr {
						util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", err, "pcs"), spec, err)
					}
				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					for _, err := range pcsErr {
						util.SubNotifyExternalSystem(data, "firing", err, activeVM[0], spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", err, "pcs"), status)
					}
				}
				errSlice = append(errSlice, "pcs errors")
			}
			log.Log.Info("Check pcs stonith")
			stonith, err := util.CheckPcsStonith(activeVM[0], clientset, config)
			if err != nil {
				log.Log.Error(err, "failed to execute pcs stonith check command")
			}
			if stonith {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(activeVM[0], fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"), spec, "pcs stonith is disalbed")
				}
				if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
					util.SubNotifyExternalSystem(data, "firing", "pcs stonith is disabled", activeVM[0], spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", "checkstonith", "pcs"), status)
				}
				status.FailedChecks = append(status.FailedChecks, "stonith is disabled, please ignore if it is intended")
				errSlice = append(errSlice, "stonith is disabled")
			}
			log.Log.Info("Check Openstack Compute Service")
			hostsErr, err := util.CheckComputeService(clientset, config)
			if err != nil {
				log.Log.Error(err, "failed to execute openstack compute service command ")
			}
			if len(hostsErr) > 0 {
				for _, host := range hostsErr {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(host, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"), spec, "compute-service")
					}
					if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
						util.SubNotifyExternalSystem(data, "firing", "openstack compute service is down", host, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", host, "compute-service"), status)
					}
					status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host))
				}
				errSlice = append(errSlice, "hosts with nova down")
			}
			log.Log.Info("Check Openstack network agents")
			netErr, err := util.CheckNetworkAgents(clientset, config)
			if err != nil {
				log.Log.Error(err, "failed to execute openstack network agent list command")
			}
			if len(netErr) > 0 {
				for _, host := range netErr {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(host, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"), spec, "network-agent")
					}
					if spec.NotifyExtenal != nil && !*spec.NotifyExtenal {
						util.SubNotifyExternalSystem(data, "firing", "openstack network service is down", host, spec.ExternalURL, string(username), string(password), fmt.Sprintf("/home/golanguser/%s-%s-ext.txt", host, "networkagent"), status)
					}
					status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host))
				}
				errSlice = append(errSlice, "hosts with network down")
			}
			if len(errSlice) < 1 {
				status.Healthy = true
				now := v1.Now()
				status.LastSuccessfulRunTime = &now
			} else {
				status.Healthy = false
			}
		}
	}
	now := v1.Now()
	status.LastRunTime = &now
	report(monitoringv1alpha1.ConditionTrue, "All healthchecks are completed, please check the resource status for failed checks.", nil)
	return ctrl.Result{RequeueAfter: defaultHealthCheckInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OsphealthcheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor(monitoringv1alpha1.EventSource)
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Osphealthcheck{}).
		Complete(r)
}
