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
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// var (
// 	errGetAuthSecret    = errors.New("failed to get Secret containing External alert system credentials")
// 	errGetAuthConfigMap = errors.New("failed to get ConfigMap containing the data to be sent to the external alert system")
// )

// OsphealthcheckReconciler reconciles a Osphealthcheck object
type OsphealthcheckReconciler struct {
	client.Client
	RESTClient               rest.Interface
	RESTConfig               *rest.Config
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
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubevirt.io,namespace=openstack,resources=virtualmachineinstances,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;create;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
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

	// secretName := types.NamespacedName{
	// 	Name: spec.ExternalSecret,
	// }

	// configmapName := types.NamespacedName{
	// 	Name: spec.ExternalData,
	// }

	// switch checker.(type) {
	// case *monitoringv1alpha1.Osphealthcheck:
	// 	secretName.Namespace = r.ClusterResourceNamespace
	// 	configmapName.Namespace = r.ClusterResourceNamespace
	// default:
	// 	log.Log.Error(fmt.Errorf("unexpected issuer type: %s", checker), "not retrying")
	// 	return ctrl.Result{}, nil
	// }

	// var secret corev1.Secret
	// var configmap corev1.ConfigMap
	// var username []byte
	// var password []byte
	// var data map[string]string

	// if spec.NotifyExtenal != nil && *spec.NotifyExtenal {
	// 	if err := r.Get(ctx, secretName, &secret); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("%w, secret name: %s, reason: %v", errGetAuthSecret, secretName, err)
	// 	}
	// 	username = secret.Data["username"]
	// 	password = secret.Data["password"]
	// 	if err := r.Get(ctx, configmapName, &configmap); err != nil {
	// 		return ctrl.Result{}, fmt.Errorf("%w, configmap name: %s, reason: %v", errGetAuthConfigMap, configmapName, err)
	// 	}
	// 	data = configmap.Data
	// }

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
	cjob, err := clientset.BatchV1().Jobs("openstack").List(context.Background(), v1.ListOptions{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to retrieve jobs, exiting")
	}
	for _, job := range cjob.Items {
		if job.Status.Active > 0 {
			log.Log.Info("There is an active/pending job running, exiting.")
			return ctrl.Result{}, nil
		}
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

	var externalVIPs []string
	cm, err := clientset.CoreV1().ConfigMaps("openstack").Get(context.Background(), "tripleo-deploy-config-default", v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("unable to fetch configmap tripleo-deploy-config-default")
	}
	con := cm.Data["rendered-tripleo-config.yaml"]
	conSl := strings.Split(con, "\n")

	for _, val := range conSl {
		if strings.Contains(val, "ExternalNetworkVip:") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs = append(externalVIPs, val2[1])
		} else if strings.Contains(val, "InternalApiNetworkVip:") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs = append(externalVIPs, val2[1])
		} else if strings.Contains(val, "ControlPlaneIP") {
			val2 := strings.SplitN(val, ": ", 2)
			externalVIPs = append(externalVIPs, val2[1])
		}
	}
	var sftpIP []string
	sftpSecret, err := clientset.CoreV1().Secrets("openstack").Get(context.Background(), "osp-rear-backup-sftp-secret", v1.GetOptions{})
	if err != nil {
		log.Log.Error(err, "unable")
	}
	sftData := sftpSecret.Data["sftp-url"]
	_, sftpa, found := strings.Cut(string(sftData), "@")
	if found {
		sftb, _, found2 := strings.Cut(sftpa, "/")
		if !found2 {
			log.Log.Error(err, "couldn't find valid IP address in the secret")
		} else {
			sftpIP = append(sftpIP, sftb)
		}
	}
	vmList := kubev1.VirtualMachineInstanceList{}
	err = clientset.RESTClient().Get().AbsPath("/apis/kubevirt.io/v1/namespaces/openstack/virtualmachineinstances").Do(context.Background()).Into(&vmList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch virtualmachine instance list")
	}
	var defaultHealthCheckInterval time.Duration
	if spec.CheckInterval != nil {
		defaultHealthCheckInterval = time.Minute * time.Duration(*spec.CheckInterval)
	} else {
		defaultHealthCheckInterval = time.Minute * 60
	}

	env := os.Getenv("cluster")
	if env == "" {
		env = "test-cluster"
	}
	var wg sync.WaitGroup
	if status.LastRunTime == nil {
		log.Log.Info(fmt.Sprintf("starting openstack healthchecks in cluster %s", config.Host))
		log.Log.Info("Checking virtual machine instances in Openstack namespace")
		activeVM, nonActiveVM, err := util.CheckFailedVms(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
		}
		if len(nonActiveVM) > 0 {
			for _, vm := range nonActiveVM {
				if !slices.Contains(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm)) {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"), spec, fmt.Sprintf("found a failed or stopped vm %s", vm))
					}
					status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm))
				}
			}
		}
		log.Log.Info("Checking failed migrations in Openstack namespace")
		mig, err := util.CheckFailedMigrations(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
		}
		if len(mig) > 0 {
			if !slices.Contains(status.FailedChecks, "found a failed or on-going migration") {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "failed-ongoing-mig"), spec, fmt.Sprintf("found a failed or an on-going migration of vm %v", mig))
				}
				status.FailedChecks = append(status.FailedChecks, "found a failed or on-going migration")
			}
		}
		log.Log.Info("Checking virtual machine instances placement in Openstack namespace")
		affectedVms, node, err := util.CheckVMIPlacement(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
		}
		if len(affectedVms) > 0 {
			if !slices.Contains(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details") {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "vmi-placement"), spec, fmt.Sprintf("found multiple ctlplane VMs in the same node %s", node))
				}
				status.FailedChecks = append(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details")
			}
		}
		log.Log.Info("Modifying ssh config file permission to avoid openstack command execution failure after openstackclient pod restarts")
		modifyReq := returnCommand(r, "chmod 644 /home/cloud-admin/.ssh/config")
		err = util.ModifyExecuteCommand(modifyReq, r.RESTConfig, "sshfile")
		if err != nil {
			if !slices.Contains(status.FailedChecks, "failed to modify cloud-admin ssh file permission") {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"), spec, "failed to modify file permission (/home/cloud-admin/.ssh/config) in openstackclient pod")
				}
				status.FailedChecks = append(status.FailedChecks, "failed to modify cloud-admin ssh file permission")
			}
			return ctrl.Result{}, fmt.Errorf("unable to modify file permission of /home/cloud-admin/.ssh/config in openstackclient pod, exiting as subsequent healtchecks might fail")
		}
		log.Log.Info("Check pcs status")
		pcsReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo pcs status", generateRandom(activeVM)))
		pcsErr, err := util.CheckPcsStatus(pcsReq, clientset, r.RESTConfig, "pcsfile")
		if err != nil {
			log.Log.Error(err, "failed to execute pcs status command ")
		}
		if len(pcsErr) > 0 {
			if !slices.Contains(status.FailedChecks, "pcs errors") {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "pcs"), spec, "found pcs errors in controller, please check")
				}
				status.FailedChecks = append(status.FailedChecks, "pcs errors")
			}
		}
		log.Log.Info("Check pcs stonith status")
		stonithReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo pcs property show stonith-enabled", generateRandom(activeVM)))
		stonith, err := util.CheckPcsStonith(stonithReq, clientset, r.RESTConfig, "stonith")
		if err != nil {
			log.Log.Error(err, "failed to execute pcs stonith check command")
		}
		if stonith {
			if !slices.Contains(status.FailedChecks, "stonith is disabled, please ignore if it is intended") {
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"), spec, "pcs stonith is disabled, please ignore if it is intended.")
				}
				status.FailedChecks = append(status.FailedChecks, "stonith is disabled, please ignore if it is intended")
			}
		}
		// check external IPs connectivity
		log.Log.Info("Check external IPs connectivity from openstackclient pod")

		if len(externalVIPs) > 0 {
			wg.Add(len(externalVIPs))
			for _, external := range externalVIPs {
				go func() {
					defer wg.Done()
					extReq := returnCommand(r, fmt.Sprintf("ping -c 3 %s", external))
					err := util.ModifyExecuteCommand(extReq, r.RESTConfig, util.HandleCNString(external))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable from %s.ctlplane", external, generateRandom(activeVM))) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", external, "external-ip"), spec, fmt.Sprintf("external IP %s is unreachable from openstackclient pod", external))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable from %s.ctlplane", external, generateRandom(activeVM)))
						}
					}
				}()
			}
			wg.Wait()
		}
		// check backup sftp server connectivity
		log.Log.Info("Check backup sftp server connectivity from all control plane VMs")
		if len(sftpIP) > 0 {
			wg.Add(len(activeVM))
			for _, vm := range activeVM {
				go func() {
					defer wg.Done()
					vmReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo nc -zv -w 3 %s 22", vm, sftpIP[0]))
					err := util.ModifyExecuteCommand(vmReq, r.RESTConfig, util.HandleCNString(vm))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM))) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, sftpIP[0]), spec, fmt.Sprintf("backup server IP %s is unreachable from control plane VM %s", sftpIP[0], vm))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM)))
						}
					}
				}()
			}
			wg.Wait()
		}
		// check multiple galera containers
		log.Log.Info("Check for multiple containers in each control plane VM")
		wg.Add(len(activeVM))
		for _, vm := range activeVM {
			go func() {
				defer wg.Done()
				vmReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo podman ps -a --format {{.ID}} --filter name=galera", vm))
				err := util.CheckGaleraContainers(vmReq, r.RESTConfig, util.HandleCNString(vm))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "galera"), spec, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
					}
				}
			}()
		}
		wg.Wait()
		running, err := isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// check workload status
		log.Log.Info("Check for workload VMs with non active state")
		vm_status := []string{"shutoff", "build", "error", "migrating", "paused", "reboot", "rescue", "resize", "unknown", "suspended", "shelved", "verify_resize"}
		wg.Add(len(vm_status))
		for _, vmstatus := range vm_status {
			go func() {
				defer wg.Done()
				workloadReq := returnCommand(r, fmt.Sprintf("openstack server list --all-projects --long -c Name -c Host --status %s -f value", vmstatus))
				failedVm, err := util.CheckWorkloadVm(workloadReq, r.RESTConfig, util.HandleCNString(vmstatus))
				if err != nil {
					log.Log.Error(err, "unable to retrieve failed vm")
				}
				if len(failedVm) > 0 {
					for _, vm := range failedVm {
						vmData := strings.SplitN(vm, ":", 2)
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("found VM %s with status %s in host %s", vmData[0], vmstatus, vmData[1])) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vmData[0], vmData[1]), spec, fmt.Sprintf("VM %s has status %s on host %s", vmData[0], vmstatus, vmData[1]))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found VM %s with status %s in host %s", vmData[0], vmstatus, vmData[1]))
						}
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// retrieve compute list and check connectivity on ctlplane, tenant, internal, storage
		log.Log.Info("Check network connectivity to each compute host from a control plane VM")
		hostReq := returnCommand(r, "openstack compute service list -c Host -f value")
		hosts, err := util.GetHostList(hostReq, r.RESTConfig, "gethosts")
		if err != nil {
			log.Log.Error(err, "unable to retrieve openstack compute service list")
		}
		conn := []string{"ctlplane", "tenant", "internalapi", "storage"}
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				for _, co := range conn {
					coReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ping -c 3 %s.%s", generateRandom(activeVM), host, co))
					err := util.ModifyExecuteCommand(coReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, co), spec, fmt.Sprintf("host %s is unreachable on %s network", host, co))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co))
						}
					}
				}
			}()
		}
		wg.Wait()
		log.Log.Info("Check Openstack Compute Service")
		svcReq := returnCommand(r, "openstack compute service list -c Host -c State -f value")
		hostsErr, err := util.CheckComputeService(svcReq, clientset, r.RESTConfig, "checksrv")
		if err != nil {
			log.Log.Error(err, "failed to execute openstack compute service command ")
		}
		if len(hostsErr) > 0 {
			for _, host := range hostsErr {
				if !slices.Contains(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host)) {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"), spec, fmt.Sprintf("nova service is down in compute %s", host))
					}
					status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host))
				}
			}
		}
		log.Log.Info("Check Openstack network agents")
		netReq := returnCommand(r, "openstack network agent list -c Host -c State -f value")
		netErr, err := util.CheckNetworkAgents(netReq, clientset, r.RESTConfig, "networkagent")
		if err != nil {
			log.Log.Error(err, "failed to execute openstack network agent list command")
		}
		if len(netErr) > 0 {
			for _, host := range netErr {
				if !slices.Contains(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host)) {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"), spec, fmt.Sprintf("network agent is down in %s", host))
					}
					status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host))
				}
			}
		}
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// check nova containers
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				novaReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo podman ps --format {{.ID}} --filter name=nova", host))
				err := util.GetNovaContainers(novaReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "nova"), spec, fmt.Sprintf("Not all nova containers are up and running on host %s", host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// check dpdk bond status
		log.Log.Info("Check DPDK bond status on each host")
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				bond3Req := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-appctl bond/show dpdkbond3", host))
				err := util.CheckOvsBond(bond3Req, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond3"), spec, fmt.Sprintf("dpdkbond3 %s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				bond4Req := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-appctl bond/show dpdkbond4", host))
				err := util.CheckOvsBond(bond4Req, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond4"), spec, fmt.Sprintf("dpdkbond4 %s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// ovs service
		log.Log.Info("Check OVS service on each host")
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				srvReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo systemctl status ovs-vswitchd.service", host))
				err := util.CheckService(srvReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-srv"), spec, fmt.Sprintf("%s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// ovs logs for bug/warning/error
		log.Log.Info("Check OVS logs for error and warning on each host")
		var ovsaffectedNodes []string
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo cat /var/log/openvswitch/ovs-vswitchd.log", host))
				err := util.CheckOvsLogs(intReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					ovsaffectedNodes = append(ovsaffectedNodes, host)
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host)) {
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		if len(ovsaffectedNodes) > 0 {
			wg.Add(len(ovsaffectedNodes))
			for _, host := range ovsaffectedNodes {
				go func() {
					defer wg.Done()
					if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log")); os.IsNotExist(err) {
						_, err = os.Create(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log"))
						if err != nil {
							log.Log.Error(err, fmt.Sprintf("unable to create ovs error log file for host %s", host))
						}
					}
				}()
			}
			wg.Wait()
			if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
				slices.Sort(ovsaffectedNodes)
				var ovss string
				for _, host := range ovsaffectedNodes {
					if strings.Contains(host, "-") {
						_, af, _ := strings.Cut(host, "-")
						ovss += af
					}
				}
				util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", ovss, "ovslog"), spec, fmt.Sprintf("observing errors/warnings in ovs log file of hosts %v", ovsaffectedNodes))
			}
		}
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// ovs-vsctl show o/p
		log.Log.Info("Check ovs-vsctl output")
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-vsctl show", host))
				err := util.CheckOvsInterfaces(intReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-int"), spec, fmt.Sprintf("%s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// time sync
		log.Log.Info("Check time sync on each host")
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo timedatectl", host))
				err := util.CheckTime(intReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "chronyd"), spec, fmt.Sprintf("%s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// nova logs
		log.Log.Info("Check nova logs for errors/warning on each host")
		var novaaffectedNodes []string
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo cat /var/log/containers/nova/nova-compute.log", host))
				err := util.CheckLogs(intReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					novaaffectedNodes = append(novaaffectedNodes, host)
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host)) {
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
		if len(novaaffectedNodes) > 0 {
			wg.Add(len(novaaffectedNodes))
			for _, host := range novaaffectedNodes {
				go func() {
					defer wg.Done()
					if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log")); os.IsNotExist(err) {
						_, err = os.Create(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log"))
						if err != nil {
							log.Log.Error(err, fmt.Sprintf("unable to create nova error log file for host %s", host))
						}
					}
				}()
			}
			wg.Wait()
			if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
				slices.Sort(novaaffectedNodes)
				var novas string
				for _, host := range novaaffectedNodes {
					if strings.Contains(host, "-") {
						_, af, _ := strings.Cut(host, "-")
						novas += af
					}
				}
				util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", novas, "novalog"), spec, fmt.Sprintf("observing errors/warning in nova-compute log file of hosts %v", novaaffectedNodes))
			}
		}
		running, err = isRunning(clientset)
		if err != nil {
			log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
		}
		if running {
			return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
		}
		// splunk
		log.Log.Info("Check Splunk service on each host")
		wg.Add(len(hosts))
		for _, host := range hosts {
			go func() {
				defer wg.Done()
				srvReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo systemctl status splunk.service", host))
				err := util.CheckService(srvReq, r.RESTConfig, util.HandleCNString(host))
				if err != nil {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "splunk"), spec, fmt.Sprintf("%s in %s", err, host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
					}
				}
			}()
		}
		wg.Wait()
	} else {
		pastTime := time.Now().Add(-1 * defaultHealthCheckInterval)
		timeDiff := status.LastRunTime.Time.Before(pastTime)
		if timeDiff {
			log.Log.Info(fmt.Sprintf("starting openstack healthchecks in cluster %s as reconciling period is elapsed", config.Host))
			log.Log.Info("Checking virtual machine instances in Openstack namespace")
			activeVM, nonActiveVM, err := util.CheckFailedVms(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
			}
			if len(nonActiveVM) > 0 {
				for _, vm := range nonActiveVM {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"), spec, fmt.Sprintf("found a failed or stopped vm %s", vm))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm))
					}
				}
			} else {
				for _, vm := range activeVM {
					if slices.Contains(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"), spec, fmt.Sprintf("found a failed or stopped vm %s", vm))
						}
						idx := slices.Index(status.FailedChecks, fmt.Sprintf("found a failed or stopped vm %s", vm))
						deleteElementSlice(status.FailedChecks, idx)
						os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "non-active"))
					}
				}
			}
			log.Log.Info("Checking failed migrations in Openstack namespace")
			mig, err := util.CheckFailedMigrations(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
			}
			if len(mig) > 0 {
				if !slices.Contains(status.FailedChecks, "found a failed or on-going migration") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "failed-ongoing-mig"), spec, fmt.Sprintf("found a failed or an on-going migration in vm %v", mig))
					}
					status.FailedChecks = append(status.FailedChecks, "found a failed or on-going migration")
				}
			} else {
				if slices.Contains(status.FailedChecks, "found a failed or on-going migration") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "failed-ongoing-mig"), spec, fmt.Sprintf("VM %v is posting successful migration state", mig))
					}
					idx := slices.Index(status.FailedChecks, "found a failed or on-going migration")
					deleteElementSlice(status.FailedChecks, idx)
					os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "failed-ongoing-mig"))
				}
			}
			log.Log.Info("Checking virtual machine instances placement in Openstack namespace")
			affectedVms, node, err := util.CheckVMIPlacement(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve vm list in openstack namespace")
			}
			if len(affectedVms) > 0 {
				if !slices.Contains(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "vmi-placement"), spec, fmt.Sprintf("found multiple ctlplane VMs in the same node %s", node))
					}
					status.FailedChecks = append(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details")
				}
			} else {
				if slices.Contains(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "vmi-placement"), spec, "ctlplane VMs are now placed in separate nodes")
					}
					idx := slices.Index(status.FailedChecks, "found multiple ctlplane VMs in the same node, please execute oc get vm -n openstack -o wide for details")
					deleteElementSlice(status.FailedChecks, idx)
					os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "vmi-placement"))
				}
			}
			log.Log.Info("Modifying ssh config file permission to avoid openstack command execution failure after openstackclient pod restarts")
			modifyReq := returnCommand(r, "chmod 644 /home/cloud-admin/.ssh/config")
			err = util.ModifyExecuteCommand(modifyReq, r.RESTConfig, "sshfile")
			if err != nil {
				if !slices.Contains(status.FailedChecks, "failed to modify cloud-admin ssh file permission") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"), spec, "failed to modify file permission (/home/cloud-admin/.ssh/config) in openstackclient pod")
					}
					status.FailedChecks = append(status.FailedChecks, "failed to modify cloud-admin ssh file permission")
				}
				return ctrl.Result{}, fmt.Errorf("unable to modify file permission of /home/cloud-admin/.ssh/config in openstackclient pod, exiting as subsequent healtchecks might fail")
			} else {
				if slices.Contains(status.FailedChecks, "failed to modify cloud-admin ssh file permission") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"), spec, "modified file permission (/home/cloud-admin/.ssh/config) in openstackclient pod")
					}
					idx := slices.Index(status.FailedChecks, "failed to modify cloud-admin ssh file permission")
					deleteElementSlice(status.FailedChecks, idx)
					os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", "modifyfileperm", "openstackclient"))
				}
			}
			log.Log.Info("Check pcs status")
			pcsReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo pcs status", generateRandom(activeVM)))
			pcsErr, err := util.CheckPcsStatus(pcsReq, clientset, r.RESTConfig, "pcsfile")
			if err != nil {
				log.Log.Error(err, "failed to execute pcs status command ")
			}
			if len(pcsErr) > 0 {
				if !slices.Contains(status.FailedChecks, "pcs errors") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "pcs"), spec, "found pcs errors in controller, please check")
					}
					status.FailedChecks = append(status.FailedChecks, "pcs errors")
				}
			} else {
				if slices.Contains(status.FailedChecks, "pcs errors") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "pcs"), spec, "pcs errors are cleared")
					}
					idx := slices.Index(status.FailedChecks, "pcs errors")
					deleteElementSlice(status.FailedChecks, idx)
					os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", "error", "pcs"))
				}
			}
			log.Log.Info("Check pcs stonith status")
			stonithReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo pcs property show stonith-enabled", generateRandom(activeVM)))
			stonith, err := util.CheckPcsStonith(stonithReq, clientset, r.RESTConfig, "stonith")
			if err != nil {
				log.Log.Error(err, "failed to execute pcs stonith check command")
			}
			if stonith {
				if !slices.Contains(status.FailedChecks, "stonith is disabled, please ignore if it is intended") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"), spec, "pcs stonith is disabled, , please ignore if it is intended.")
					}
					status.FailedChecks = append(status.FailedChecks, "stonith is disabled, please ignore if it is intended")
				}
			} else {
				if slices.Contains(status.FailedChecks, "stonith is disabled, please ignore if it is intended") {
					if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
						util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"), spec, "pcs stonith is now enabled")
					}
					idx := slices.Index(status.FailedChecks, "stonith is disabled, please ignore if it is intended")
					deleteElementSlice(status.FailedChecks, idx)
					os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", "checkstonith", "pcs"))
				}
			}

			// check external IPs connectivity
			log.Log.Info("Check external IPs connectivity from openstackclient pod")
			if len(externalVIPs) > 0 {
				wg.Add(len(externalVIPs))
				for _, external := range externalVIPs {
					go func() {
						defer wg.Done()
						extReq := returnCommand(r, fmt.Sprintf("ping -c 3 %s", external))
						err := util.ModifyExecuteCommand(extReq, r.RESTConfig, util.HandleCNString(external))
						if err != nil {
							if !slices.Contains(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable", external)) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", external, "external-ip"), spec, fmt.Sprintf("external IP %s is unreachable", external))
								}
								status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable from %s.ctlplane", external, generateRandom(activeVM)))
							}
						} else {
							if slices.Contains(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable", external)) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", external, "external-ip"), spec, fmt.Sprintf("external IP %s is now reachable", external))
								}
								idx := slices.Index(status.FailedChecks, fmt.Sprintf("external IP %s is unreachable", external))
								deleteElementSlice(status.FailedChecks, idx)
								os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", external, "external-ip"))
							}
						}
					}()
				}
				wg.Wait()
			}
			// check backup sftp server connectivity
			log.Log.Info("Check backup sftp server connectivity from all control plane VMs")
			if len(sftpIP) > 0 {
				wg.Add(len(activeVM))
				for _, vm := range activeVM {
					go func() {
						defer wg.Done()
						vmReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo nc -zv -w 3 %s 22", vm, sftpIP[0]))
						err := util.ModifyExecuteCommand(vmReq, r.RESTConfig, util.HandleCNString(vm))
						if err != nil {
							if !slices.Contains(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM))) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, sftpIP[0]), spec, fmt.Sprintf("backup server IP %s is unreachable from control plane VM %s", sftpIP[0], vm))
								}
								status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM)))
							}
						} else {
							if slices.Contains(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM))) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, sftpIP[0]), spec, fmt.Sprintf("backup server IP %s is now reachable from control plane VM %s", sftpIP[0], vm))
								}
								idx := slices.Index(status.FailedChecks, fmt.Sprintf("backup server %s is unreachable from control plane VM %s", sftpIP[0], generateRandom(activeVM)))
								deleteElementSlice(status.FailedChecks, idx)
								os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, sftpIP[0]))
							}
						}
					}()
				}
				wg.Wait()
			}
			// check multiple galera containers
			log.Log.Info("Check for multiple containers in each control plane VM")
			wg.Add(len(activeVM))
			for _, vm := range activeVM {
				go func() {
					defer wg.Done()
					vmReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo podman ps -a --format {{.ID}} --filter name=galera", vm))
					err := util.CheckGaleraContainers(vmReq, r.RESTConfig, util.HandleCNString(vm))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "galera"), spec, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "galera"), spec, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("found multiple running galera containers in VM %s", vm))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", vm, "galera"))
						}
					}
				}()
			}
			wg.Wait()
			running, err := isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// check workload status
			log.Log.Info("Check for workload VMs with non active state")
			var allFailedVms []string
			vm_status := []string{"shutoff", "build", "error", "migrating", "paused", "reboot", "rescue", "resize", "unknown", "suspended", "shelved", "verify_resize"}
			wg.Add(len(vm_status))
			for _, vmstatus := range vm_status {
				go func() {
					defer wg.Done()
					workloadReq := returnCommand(r, fmt.Sprintf("openstack server list --all-projects --long -c Name -c Host --status %s -f value", vmstatus))
					failedVm, err := util.CheckWorkloadVm(workloadReq, r.RESTConfig, util.HandleCNString(vmstatus))
					if err != nil {
						log.Log.Error(err, "unable to retrieve failed vm")
					}
					if len(failedVm) > 0 {
						for _, vm := range failedVm {
							allFailedVms = append(allFailedVms, vm)
							vmData := strings.SplitN(vm, ":", 2)
							if !slices.Contains(status.FailedChecks, fmt.Sprintf("found VM %s with non-running status on host %s", vmData[0], vmData[1])) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vmData[0], vmData[1]), spec, fmt.Sprintf("VM %s has non-running status in host %s", vmData[0], vmData[1]))
								}
								status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("found VM %s with non-running status in host %s", vmData[0], vmData[1]))
							}
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			if len(allFailedVms) > 0 {
				wg.Add(len(allFailedVms))
				for _, vm := range allFailedVms {
					go func() {
						defer wg.Done()
						vmData := strings.SplitN(vm, ":", 2)
						workloadReq := returnCommand(r, fmt.Sprintf("openstack server list --all-projects --long -c Status --name %s -f value", vmData[0]))
						activev, err := util.ClearWorkloadVm(workloadReq, r.RESTConfig, util.HandleCNString(vm))
						if err != nil {
							log.Log.Error(err, "unable to retrieve active vm")
						}
						if activev {
							if slices.Contains(status.FailedChecks, fmt.Sprintf("found VM %s with non-running status on host %s", vmData[0], vmData[1])) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", vmData[0], vmData[1]), spec, fmt.Sprintf("VM %s is now running with active status on host %s", vmData[0], vmData[1]))
								}
								idx := slices.Index(status.FailedChecks, fmt.Sprintf("found VM %s with non-running status on host %s", vmData[0], vmData[1]))
								deleteElementSlice(status.FailedChecks, idx)
								os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", vmData[0], vmData[1]))
							}
						}
					}()
				}
				wg.Wait()
			}
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// retrieve compute list and check connectivity on ctlplane, tenant, internal, storage
			log.Log.Info("Check network connectivity to each compute host from a control plane VM")
			hostReq := returnCommand(r, "openstack compute service list -c Host -f value")
			hosts, err := util.GetHostList(hostReq, r.RESTConfig, "gethosts")
			if err != nil {
				log.Log.Error(err, "unable to retrieve openstack compute service list")
			}
			conn := []string{"ctlplane", "tenant", "internalapi", "storage"}
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					for _, co := range conn {
						coReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ping -c 3 %s.%s", generateRandom(activeVM), host, co))
						err := util.ModifyExecuteCommand(coReq, r.RESTConfig, util.HandleCNString(host))
						if err != nil {
							if !slices.Contains(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co)) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, co), spec, fmt.Sprintf("host %s is unreachable on  %s network", host, co))
								}
								status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co))
							}
						} else {
							if slices.Contains(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co)) {
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, co), spec, fmt.Sprintf("host %s is now reachable on  %s network", host, co))
								}
								idx := slices.Index(status.FailedChecks, fmt.Sprintf("host %s is unreachable on  %s network", host, co))
								deleteElementSlice(status.FailedChecks, idx)
								os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, co))
							}
						}
					}
				}()
			}
			wg.Wait()
			log.Log.Info("Check Openstack Compute Service")
			svcReq := returnCommand(r, "openstack compute service list -c Host -c State -f value")
			hostsErr, err := util.CheckComputeService(svcReq, clientset, r.RESTConfig, "checksrv")
			if err != nil {
				log.Log.Error(err, "failed to execute openstack compute service command ")
			}
			if len(hostsErr) > 0 {
				for _, host := range hostsErr {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"), spec, fmt.Sprintf("nova service is down in compute %s", host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host))
					}
				}
			} else {
				wg.Add(len(hosts))
				for _, host := range hosts {
					go func() {
						defer wg.Done()
						if slices.Contains(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"), spec, fmt.Sprintf("nova service is now up in compute %s", host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("openstack compute service is down in node %s", host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "computesvc"))
						}
					}()
				}
				wg.Wait()
			}
			log.Log.Info("Check Openstack network agents")
			netReq := returnCommand(r, "openstack network agent list -c Host -c State -f value")
			netErr, err := util.CheckNetworkAgents(netReq, clientset, r.RESTConfig, "networkagent")
			if err != nil {
				log.Log.Error(err, "failed to execute openstack network agent list command")
			}
			if len(netErr) > 0 {
				for _, host := range netErr {
					if !slices.Contains(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host)) {
						if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
							util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"), spec, fmt.Sprintf("network agent is down in %s", host))
						}
						status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host))
					}
				}
			} else {
				wg.Add(len(hosts))
				for _, host := range hosts {
					go func() {
						defer wg.Done()
						if slices.Contains(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"), spec, fmt.Sprintf("network agent is now up in %s", host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("openstack network agent is down in node %s", host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "networkagent"))
						}

					}()
				}
				wg.Wait()
			}
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// check nova containers
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					novaReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo podman ps --format {{.ID}} --filter name=nova", host))
					err := util.GetNovaContainers(novaReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "nova"), spec, fmt.Sprintf("Not all nova containers are up and running on host %s", host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "nova"), spec, fmt.Sprintf("all nova containers are up and running on host %s", host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("Not all nova containers are up and running on host %s", host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "nova"))
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// check dpdk bond status
			log.Log.Info("Check DPDK bond status on each host")
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					bond3Req := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-appctl bond/show dpdkbond3", host))
					err := util.CheckOvsBond(bond3Req, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond3"), spec, fmt.Sprintf("dpdkbond3 %s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond3"), spec, fmt.Sprintf("dpdkbond3 issue is cleared in %s", host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("dpdkbond3 %s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond3"))
						}

					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					bond4Req := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-appctl bond/show dpdkbond4", host))
					err := util.CheckOvsBond(bond4Req, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond4"), spec, fmt.Sprintf("dpdkbond4 %s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond4"), spec, fmt.Sprintf("dpdkbond4 is cleared in %s", host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("dpdkbond4 %s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-bond4"))
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// ovs service
			log.Log.Info("Check OVS service on each host")
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					srvReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo systemctl status ovs-vswitchd.service", host))
					err := util.CheckService(srvReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-srv"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-srv"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-srv"))
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// ovs logs for bug/warning/error
			log.Log.Info("Check OVS logs for error and warning on each host")
			var ovsaffectedNodes []string
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo cat /var/log/openvswitch/ovs-vswitchd.log", host))
					err := util.CheckOvsLogs(intReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						ovsaffectedNodes = append(ovsaffectedNodes, host)
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host)) {
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host)) {
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s of ovs in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
						}
					}
				}()
			}
			wg.Wait()
			var ovss string
			if len(ovsaffectedNodes) > 0 {
				wg.Add(len(hosts))
				for _, host := range hosts {
					go func() {
						defer wg.Done()
						if !slices.Contains(ovsaffectedNodes, host) {
							if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log")); os.IsNotExist(err) {
								//
							} else {
								err := os.WriteFile(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log"), []byte("sent"), 0666)
								if err != nil {
									log.Log.Error(err, fmt.Sprintf("unable to write email sent to ovs log file of host %s", host))
								}
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log"), spec, fmt.Sprintf("No longer observing errors/warnings in ovs log file of host %s", host))
								}
								os.Remove(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "ovs-log"))
							}
						}
					}()
				}
				wg.Wait()
				wg.Add(len(ovsaffectedNodes))
				for _, ohost := range ovsaffectedNodes {
					go func() {
						defer wg.Done()
						if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", ohost, "ovs-log")); os.IsNotExist(err) {
							_, err = os.Create(fmt.Sprintf("/home/golanguser/.%s-%s.txt", ohost, "ovs-log"))
							if err != nil {
								log.Log.Error(err, fmt.Sprintf("unable to create ovs error log file for host %s", ohost))
							}
						}
					}()
				}
				wg.Wait()
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					slices.Sort(ovsaffectedNodes)
					for _, host := range ovsaffectedNodes {
						if strings.Contains(host, "-") {
							_, af, _ := strings.Cut(host, "-")
							ovss += af
						}
					}
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", ovss, "ovslog"), spec, fmt.Sprintf("observing errors/warnings in ovs log file of hosts %v", ovsaffectedNodes))
				}
			} else {
				dir, err := os.Open("/home/golanguser/")
				if err != nil {
					log.Log.Error(err, "unable to open directory /home/golanguser")
				}
				files, err := os.ReadDir(dir.Name())
				if err != nil {
					log.Log.Error(err, "unable to read directory /home/golanguser")
				}
				var ovsfileName string
				for _, f := range files {
					if strings.Contains(f.Name(), "ovslog") {
						ovsfileName = f.Name()
					}
				}
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					util.SendEmailRecoveredAlert(env, ovsfileName, spec, fmt.Sprintf("observing errors/warnings in ovs log file of hosts %v", ovsaffectedNodes))
				}
				os.Remove(ovsfileName)
			}
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// ovs-vsctl show o/p
			log.Log.Info("Check ovs-vsctl output")
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo ovs-vsctl show", host))
					err := util.CheckOvsInterfaces(intReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-int"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-int"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "ovs-int"))
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// time sync
			log.Log.Info("Check time sync on each host")
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo timedatectl", host))
					err := util.CheckTime(intReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "chronyd"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "chronyd"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "chronyd"))
						}
					}
				}()
			}
			wg.Wait()
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// nova logs
			log.Log.Info("Check nova logs for errors/warning on each host")
			var novaaffectedNodes []string
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					intReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo cat /var/log/containers/nova/nova-compute.log", host))
					err := util.CheckLogs(intReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						novaaffectedNodes = append(novaaffectedNodes, host)
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host)) {
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host)) {
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s of nova in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
						}
					}
				}()
			}
			wg.Wait()
			var novas string
			if len(novaaffectedNodes) > 0 {
				wg.Add(len(hosts))
				for _, host := range hosts {
					go func() {
						defer wg.Done()
						if !slices.Contains(ovsaffectedNodes, host) {
							if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log")); os.IsNotExist(err) {
								//
							} else {
								err := os.WriteFile(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log"), []byte("sent"), 0666)
								if err != nil {
									log.Log.Error(err, fmt.Sprintf("unable to write email sent to ovs log file of host %s", host))
								}
								if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
									util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log"), spec, fmt.Sprintf("No longer observing errors/warnings in nova log file of host %s", host))
								}
								os.Remove(fmt.Sprintf("/home/golanguser/.%s-%s.txt", host, "nova-log"))
							}
						}
					}()
				}
				wg.Wait()
				wg.Add(len(novaaffectedNodes))
				for _, nhost := range novaaffectedNodes {
					go func() {
						defer wg.Done()
						if _, err := os.Stat(fmt.Sprintf("/home/golanguser/.%s-%s.txt", nhost, "nova-log")); os.IsNotExist(err) {
							_, err = os.Create(fmt.Sprintf("/home/golanguser/.%s-%s.txt", nhost, "nova-log"))
							if err != nil {
								log.Log.Error(err, fmt.Sprintf("unable to create nova error log file for host %s", nhost))
							}
						}
					}()
				}
				wg.Wait()
				if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
					slices.Sort(novaaffectedNodes)
					for _, host := range novaaffectedNodes {
						if strings.Contains(host, "-") {
							_, af, _ := strings.Cut(host, "-")
							novas += af
						}
					}
					util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/.%s-%s.txt", novas, "novalog"), spec, fmt.Sprintf("observing errors/warnings in nova log file of hosts %v", novaaffectedNodes))
				}
			} else {
				dir, err := os.Open("/home/golanguser/")
				if err != nil {
					log.Log.Error(err, "unable to open directory /home/golanguser")
				}
				files, err := os.ReadDir(dir.Name())
				if err != nil {
					log.Log.Error(err, "unable to read directory /home/golanguser")
				}
				var novafileName string
				for _, f := range files {
					if strings.Contains(f.Name(), "novalog") {
						novafileName = f.Name()
					}
				}
				util.SendEmailRecoveredAlert(env, novafileName, spec, "No longer observing errors/warnings in nova log file of hosts")
				os.Remove(novafileName)
			}
			running, err = isRunning(clientset)
			if err != nil {
				log.Log.Error(err, "unable to retrieve jobs in openstack namespace")
			}
			if running {
				return ctrl.Result{}, fmt.Errorf("there is an on-going/pending job in openstack, exiting")
			}
			// splunk
			log.Log.Info("Check Splunk service on each host")
			wg.Add(len(hosts))
			for _, host := range hosts {
				go func() {
					defer wg.Done()
					srvReq := returnCommand(r, fmt.Sprintf("ssh -q %s.ctlplane sudo systemctl status splunk.service", host))
					err := util.CheckService(srvReq, r.RESTConfig, util.HandleCNString(host))
					if err != nil {
						if !slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "splunk"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							status.FailedChecks = append(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
						}
					} else {
						if slices.Contains(status.FailedChecks, fmt.Sprintf("%s in %s", err, host)) {
							if spec.SuspendEmailAlert != nil && !*spec.SuspendEmailAlert {
								util.SendEmailRecoveredAlert(env, fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "splunk"), spec, fmt.Sprintf("%s in %s", err, host))
							}
							idx := slices.Index(status.FailedChecks, fmt.Sprintf("%s in %s", err, host))
							deleteElementSlice(status.FailedChecks, idx)
							os.Remove(fmt.Sprintf("/home/golanguser/%s-%s.txt", host, "splunk"))
						}
					}
				}()
			}
			wg.Wait()
		}
	}
	if status.FailedChecks != nil && len(status.FailedChecks) < 1 {
		status.Healthy = true
		now := v1.Now()
		status.LastSuccessfulRunTime = &now
		report(monitoringv1alpha1.ConditionTrue, "All healthchecks are completed successfully.", nil)
	} else {
		status.Healthy = false
		report(monitoringv1alpha1.ConditionFalse, "Some checks are failing, please check status.FailedChecks for list of failures.", nil)
	}
	now := v1.Now()
	status.LastRunTime = &now
	return ctrl.Result{RequeueAfter: defaultHealthCheckInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OsphealthcheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor(monitoringv1alpha1.EventSource)
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Osphealthcheck{}).
		Complete(r)
}

func deleteElementSlice(slice []string, index int) []string {
	return append(slice[:index], slice[index+1:]...)
}

func returnCommand(r *OsphealthcheckReconciler, commandToRun string) *rest.Request {
	execReq := r.RESTClient.
		Post().
		Namespace("openstack").
		Resource("pods").
		Name("openstackclient").
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "openstackclient",
			Command:   strings.Fields(commandToRun),
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
		}, runtime.NewParameterCodec(r.Scheme))
	return execReq
}

func generateRandom(sli []string) string {
	if len(sli) < 1 {
		return ""
	}
	idx := rand.Intn(len(sli))
	return sli[idx]
}

func isRunning(clientset *kubernetes.Clientset) (bool, error) {
	job, err := clientset.BatchV1().Jobs("openstack").List(context.Background(), v1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, j := range job.Items {
		if j.Status.Active > 0 {
			return true, err
		}
	}
	return false, nil
}
