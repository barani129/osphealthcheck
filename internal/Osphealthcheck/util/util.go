package util

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/barani129/osphealthcheck/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	kubev1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSpecAndStatus(osphealtcheck client.Object) (*v1alpha1.OsphealthcheckSpec, *v1alpha1.OsphealthcheckStatus, error) {
	switch t := osphealtcheck.(type) {
	case *v1alpha1.Osphealthcheck:
		return &t.Spec, &t.Status, nil
	default:
		return nil, nil, fmt.Errorf("not an osphealthcheck type: %t", t)
	}
}

func GetReadyCondition(status *v1alpha1.OsphealthcheckStatus) *v1alpha1.OsphealthcheckCondition {
	for _, c := range status.Conditions {
		if c.Type == v1alpha1.OsphealthcheckConditionReady {
			return &c
		}
	}
	return nil
}

func IsReady(status *v1alpha1.OsphealthcheckStatus) bool {
	if c := GetReadyCondition(status); c != nil {
		return c.Status == v1alpha1.ConditionTrue
	}
	return false
}

func SetReadyCondition(status *v1alpha1.OsphealthcheckStatus, conditionStatus v1alpha1.ConditionStatus, reason, message string) {
	ready := GetReadyCondition(status)
	if ready == nil {
		ready = &v1alpha1.OsphealthcheckCondition{
			Type: v1alpha1.OsphealthcheckConditionReady,
		}
		status.Conditions = append(status.Conditions, *ready)
	}
	if ready.Status != conditionStatus {
		ready.Status = conditionStatus
		now := metav1.Now()
		ready.LastTransitionTime = &now
	}
	ready.Reason = reason
	ready.Message = message

	for i, c := range status.Conditions {
		if c.Type == v1alpha1.OsphealthcheckConditionReady {
			status.Conditions[i] = *ready
			return
		}
	}
}

func HandleCNString(cn string) string {
	var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	return nonAlphanumericRegex.ReplaceAllString(cn, "")
}

func ExecuteCommand(req *rest.Request, config *rest.Config, host string) ([]byte, error) {
	outFile, err := os.OpenFile(fmt.Sprintf("/home/golanguser/commandoutput-%s.txt", host), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	ex, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	err = ex.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: outFile,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		return nil, err
	}
	time.Sleep(2 * time.Second)
	data, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, err
	}
	os.Remove(outFile.Name())
	return data, nil
}

func ModifyExecuteCommand(req *rest.Request, config *rest.Config, host string) error {
	outFile, err := os.OpenFile(fmt.Sprintf("/home/golanguser/commandoutput-%s.txt", host), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	ex, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}
	err = ex.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: outFile,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		return err
	}
	os.Remove(outFile.Name())
	return nil
}

func SendEmailAlert(nodeName string, filename string, spec *v1alpha1.OsphealthcheckSpec, commandToRun string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" "" "Alert: %s" | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
		cmd3 := exec.Command("/bin/bash", "-c", message)
		err := cmd3.Run()
		if err != nil {
			fmt.Printf("Failed to send the alert: %s", err)
		}
		writeFile(filename, "sent")
	} else {
		data, _ := ReadFile(filename)
		if data != "sent" {
			message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" "" "Alert: %s" | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
			cmd3 := exec.Command("/bin/bash", "-c", message)
			err := cmd3.Run()
			if err != nil {
				fmt.Printf("Failed to send the alert: %s", err)
			}
			os.Truncate(filename, 0)
			writeFile(filename, "sent")
		}
	}
}

func randomString(length int) string {
	b := make([]byte, length+2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[2 : length+2]
}

func SendEmailRecoveredAlert(nodeName string, filename string, spec *v1alpha1.OsphealthcheckSpec, commandToRun string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		//
	} else {
		data, err := ReadFile(filename)
		if err != nil {
			fmt.Printf("Failed to send the alert: %s", err)
		}
		if data == "sent" {
			message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" ""  "Resolved: %s" | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
			cmd3 := exec.Command("/bin/bash", "-c", message)
			err := cmd3.Run()
			if err != nil {
				fmt.Printf("Failed to send the alert: %s", err)
			}
		}
	}
}

func SubNotifyExternalSystem(data map[string]string, status string, commandToRun string, nodeName string, url string, username string, password string, filename string, clstatus *v1alpha1.OsphealthcheckStatus) error {
	var fingerprint string
	var err error
	if status == "resolved" {
		fingerprint, err = ReadFile(filename)
		if err != nil || fingerprint == "" {
			return fmt.Errorf("unable to notify the system for the %s status due to missing fingerprint in the file %s", status, filename)
		}
	} else {
		fingerprint, _ = ReadFile(filename)
		if fingerprint != "" {
			return nil
		}
		fingerprint = randomString(10)
	}
	data["fingerprint"] = fingerprint
	data["status"] = status
	data["startsAt"] = time.Now().String()
	data["alertName"] = fmt.Sprintf("%s node %s, please check", commandToRun, nodeName)
	data["message"] = fmt.Sprintf("%s in node %s, please check", commandToRun, nodeName)
	m, b := data, new(bytes.Buffer)
	json.NewEncoder(b).Encode(m)
	var client *http.Client
	if strings.Contains(url, "https://") {
		tr := http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{
			Timeout:   5 * time.Second,
			Transport: &tr,
		}
	} else {
		client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Basic "+basicAuth(username, password))
	req.Header.Set("User-Agent", "Openshift")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp == nil {
		return err
	}
	writeFile(filename, fingerprint)
	return nil
}

func NotifyExternalSystem(data map[string]string, status string, commandToRun string, nodeName string, url string, username string, password string, filename string, clstatus *v1alpha1.OsphealthcheckStatus) error {

	fig, _ := ReadFile(filename)
	if fig != "" {
		log.Printf("External system has already been notified for target %s . Exiting", url)
		return nil
	}
	fingerprint := randomString(10)
	data["fingerprint"] = fingerprint
	data["status"] = status
	data["startsAt"] = time.Now().String()
	data["alertName"] = fmt.Sprintf("%s node %s, please check", commandToRun, nodeName)
	data["message"] = fmt.Sprintf("%s in node %s, please check", commandToRun, nodeName)
	m, b := data, new(bytes.Buffer)
	json.NewEncoder(b).Encode(m)
	var client *http.Client
	if strings.Contains(url, "https://") {
		tr := http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{
			Timeout:   5 * time.Second,
			Transport: &tr,
		}
	} else {
		client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Basic "+basicAuth(username, password))
	req.Header.Set("User-Agent", "Openshift")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp == nil {
		return err
	}
	writeFile(filename, fingerprint)
	return nil
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func writeFile(filename string, data string) error {
	err := os.WriteFile(filename, []byte(data), 0666)
	if err != nil {
		return err
	}
	return nil
}

func ReadFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func CheckPcsStatus(rest *rest.Request, clientset *kubernetes.Clientset, config *rest.Config, host string) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(data), "Stopped") {
		errSlice = append(errSlice, "found-stopped-service-in-pcs-status")
	}
	if strings.Contains(string(data), "Failed Fencing Actions") {
		errSlice = append(errSlice, "found-failed-fencing-actions-in-pcs-status")
	}
	if strings.Contains(string(data), "failed") {
		errSlice = append(errSlice, "found-other-failed-events-in-pcs-status")
	}
	return errSlice, nil
}

func CheckPcsStonith(rest *rest.Request, clientset *kubernetes.Clientset, config *rest.Config, host string) (bool, error) {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return false, err
	}
	if strings.Contains(string(data), "false") {
		return true, nil
	}
	return false, nil
}

func CheckGaleraContainers(rest *rest.Request, config *rest.Config, host string) error {
	var galera []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if len(string(data)) <= 1 {
		return fmt.Errorf("no galera containers found")
	}
	sliceData := strings.Split(string(data), "\n")
	for _, cont := range sliceData {
		cont = strings.TrimSpace(cont)
		if len(cont) > 0 {
			galera = append(galera, cont)
		}
	}
	if len(galera) > 1 {
		return fmt.Errorf("multiple galera containers found")
	}
	return nil
}

func GetHostList(rest *rest.Request, config *rest.Config, host string) ([]string, error) {
	var hosts []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil, err
	}
	if len(string(data)) <= 1 {
		return nil, fmt.Errorf("no hosts found")
	}
	sliceData := strings.Split(string(data), "\n")
	for _, host := range sliceData {
		if strings.Contains(host, "dpdkcompute") {
			bef, _, _ := strings.Cut(host, ".")
			hosts = append(hosts, bef)
		}
	}
	return hosts, nil
}

func GetNovaContainers(rest *rest.Request, config *rest.Config, host string) error {
	var nova []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if len(string(data)) <= 1 {
		return fmt.Errorf("no nova containers found")
	}
	sliceData := strings.Split(string(data), "\n")
	for _, cont := range sliceData {
		cont = strings.TrimSpace(cont)
		if len(cont) > 0 {
			nova = append(nova, cont)
		}
	}
	if len(nova) < 4 {
		return fmt.Errorf("found only %d nova containers", len(nova))
	}
	return nil
}

func CheckOvsBond(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "may_enable: false") {
		return fmt.Errorf("one of the dpdk interface is down")
	}
	return nil
}

func CheckService(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "Active: active (running)") {
		return fmt.Errorf("ovs service is down")
	}
	return nil
}

func CheckOvsInterfaces(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "error") {
		return fmt.Errorf("observing errors in ovs-vsctl show output")
	}
	return nil
}

func CheckOvsLogs(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "ERROR") {
		return fmt.Errorf("observing errors in log file")
	}
	return nil
}

func CheckLogs(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "ERROR") || strings.Contains(string(data), "WARN") {
		return fmt.Errorf("observing errors/warnings in log file")
	}
	return nil
}

func CheckStaleResources(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "There are allocations remaining against the source host") {
		return fmt.Errorf("found stale resource allocations")
	}
	return nil
}

func CheckVMInterface(rest *rest.Request, config *rest.Config, host string) []string {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil
	}
	var intData []string
	var intData2 []string
	sliceData := strings.Split(string(data), "\n")
	for _, inte := range sliceData {
		if strings.Contains(inte, "name                : vhu") {
			intData = append(intData, inte)
		}
	}
	if len(intData) > 0 {
		for _, inter := range intData {
			inte := strings.Split(inter, "name                : ")
			intData2 = append(intData2, inte[1])
		}
	}
	return intData2
}

func GetVMInterface(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "down") {
		return fmt.Errorf("VM interface is down")
	}
	return nil
}

func CheckTime(rest *rest.Request, config *rest.Config, host string) error {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "NTP service: active") || !strings.Contains(string(data), "System clock synchronized: yes") {
		return fmt.Errorf("NTP service is out of sync")
	}
	return nil
}

func CheckComputeService(rest *rest.Request, clientset *kubernetes.Clientset, config *rest.Config, host string) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil, err
	}
	sliceData := strings.Split(string(data), "\n")
	for _, dat := range sliceData {
		if strings.Contains(dat, "down") {
			datSlice := strings.SplitN(dat, " ", 2)
			errSlice = append(errSlice, datSlice[0])
		}
	}
	return errSlice, nil
}

func CheckNetworkAgents(rest *rest.Request, clientset *kubernetes.Clientset, config *rest.Config, host string) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil, err
	}
	sliceData := strings.Split(string(data), "\n")
	for _, dat := range sliceData {
		if strings.Contains(dat, "False") {
			datSlice := strings.SplitN(dat, " ", 2)
			errSlice = append(errSlice, datSlice[0])
		}
	}
	return errSlice, nil
}

func CheckFailedMigrations(clientset *kubernetes.Clientset) ([]string, error) {
	var migrations []string
	vmList := kubev1.VirtualMachineInstanceList{}
	err := clientset.RESTClient().Get().AbsPath("/apis/kubevirt.io/v1/namespaces/openstack/virtualmachineinstances").Do(context.Background()).Into(&vmList)
	if err != nil {
		return nil, err
	}
	for _, vm := range vmList.Items {
		if vm.Name != "" && strings.Contains(vm.Name, "controller-") {
			if vm.Status.MigrationState != nil {
				if !vm.Status.MigrationState.Completed {
					migrations = append(migrations, vm.Name)
				}
			}
		}
	}
	if len(migrations) > 0 {
		return migrations, nil
	}
	return nil, nil
}

func CheckFailedVms(clientset *kubernetes.Clientset) ([]string, []string, error) {
	var activeVM []string
	var nonActiveVM []string
	vmList := kubev1.VirtualMachineInstanceList{}
	err := clientset.RESTClient().Get().AbsPath("/apis/kubevirt.io/v1/namespaces/openstack/virtualmachineinstances").Do(context.Background()).Into(&vmList)
	if err != nil {
		return nil, nil, err
	}
	for _, vm := range vmList.Items {
		if strings.Contains(vm.Name, "controller-") {
			if vm.Status.Phase == "Running" {
				activeVM = append(activeVM, vm.Name)
			} else {
				nonActiveVM = append(nonActiveVM, vm.Name)
			}
		}
	}
	return activeVM, nonActiveVM, nil
}

func CheckWorkloadVm(rest *rest.Request, config *rest.Config, host string) ([]string, error) {
	var failedVm []string
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return nil, err
	}
	if len(string(data)) <= 1 {
		return nil, nil
	}
	sliceData := strings.Split(string(data), "\n")
	for _, dat := range sliceData {
		dat = strings.TrimSpace(dat)
		if len(dat) > 0 {
			dat2 := strings.Split(dat, " ")
			failedVm = append(failedVm, dat2[0]+":"+dat2[1])
		}
	}
	return failedVm, nil
}

func ClearWorkloadVm(rest *rest.Request, config *rest.Config, host string) (bool, error) {
	data, err := ExecuteCommand(rest, config, host)
	if err != nil {
		return false, err
	}
	if len(string(data)) <= 1 {
		return false, nil
	}
	sliceData := strings.Split(string(data), "\n")
	for _, cont := range sliceData {
		cont = strings.TrimSpace(cont)
		if len(cont) > 0 {
			if cont == "ACTIVE" {
				return true, nil
			}
		}
	}
	return false, nil
}

func CheckVMIPlacement(clientset *kubernetes.Clientset) ([]string, string, error) {
	var affectVms []string
	vmiNodes := make(map[string][]string)
	nodeList, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, "", err
	}
	vmList := kubev1.VirtualMachineInstanceList{}
	err = clientset.RESTClient().Get().AbsPath("/apis/kubevirt.io/v1/namespaces/openstack/virtualmachineinstances").Do(context.Background()).Into(&vmList)
	if err != nil {
		return nil, "", err
	}
	for _, vmi := range vmList.Items {
		if strings.Contains(vmi.Name, "controller-") {
			vmiNodes[vmi.Status.NodeName] = append(vmiNodes[vmi.Status.NodeName], vmi.Name)
		}
	}
	for _, node := range nodeList.Items {
		vminodelist := vmiNodes[node.Name]
		if len(vminodelist) > 1 {
			for _, vmi := range vminodelist {
				affectVms = append(affectVms, node.Name+vmi)
			}
			return affectVms, node.Name, nil
		}
	}
	return nil, "", nil
}
