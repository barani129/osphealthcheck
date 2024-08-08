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
	"strings"
	"time"

	"github.com/barani129/osphealthcheck/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
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

func ExecuteCommand(commandToRun string, clientset *kubernetes.Clientset, config *rest.Config) ([]byte, error) {
	outFile, err := os.OpenFile("/home/golanguser/commandoutput.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	outErrFile, err := os.OpenFile("/home/golanguser/commanderr.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name("openstackclient").Namespace("openstack").SubResource("exec").VersionedParams(
		&corev1.PodExecOptions{
			Container: "openstackclient",
			Command:   strings.Fields(commandToRun),
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	// ex, err := remotecommand.NewWebSocketExecutor(config, "GET", req.URL().String())
	ex, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	err = ex.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: outFile,
		Stderr: outErrFile,
		Tty:    false,
	})
	if err != nil {
		return nil, err
	}
	errData, err := os.ReadFile(outErrFile.Name())
	if errData != nil {
		return nil, err
	}
	data, err := os.ReadFile(outFile.Name())
	if errData != nil {
		return nil, err
	}
	os.Remove(outErrFile.Name())
	os.Remove(outFile.Name())
	return data, nil
}

func SendEmailAlert(nodeName string, filename string, spec *v1alpha1.OsphealthcheckSpec, commandToRun string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" ""  "Failed to execute command %s" | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
		cmd3 := exec.Command("/bin/bash", "-c", message)
		err := cmd3.Run()
		if err != nil {
			fmt.Printf("Failed to send the alert: %s", err)
		}
		writeFile(filename, "sent")
	} else {
		data, _ := ReadFile(filename)
		fmt.Println(data)
		if data != "sent" {
			message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" ""  "Failed to execute command %s" | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
			cmd3 := exec.Command("/bin/bash", "-c", message)
			err := cmd3.Run()
			if err != nil {
				fmt.Printf("Failed to send the alert: %s", err)
			}
		}
	}
}

func randomString(length int) string {
	b := make([]byte, length+2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[2 : length+2]
}

func SendEmailRecoveredAlert(nodeName string, filename string, spec *v1alpha1.OsphealthcheckSpec, commandToRun string) {
	data, err := ReadFile(filename)
	if err != nil {
		fmt.Printf("Failed to send the alert: %s", err)
	}
	if data == "sent" {
		message := fmt.Sprintf(`/usr/bin/printf '%s\n' "Subject: Alert from %s" ""  "command %s which previously failed to execute is now working." | /usr/sbin/sendmail -f %s -S %s %s`, "%s", nodeName, commandToRun, spec.Email, spec.RelayHost, spec.Email)
		cmd3 := exec.Command("/bin/bash", "-c", message)
		err := cmd3.Run()
		if err != nil {
			fmt.Printf("Failed to send the alert: %s", err)
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

func CheckPcsStatus(activeVM string, clientset *kubernetes.Clientset, config *rest.Config) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand(fmt.Sprintf("ssh -q %s.ctlplane sudo pcs status", activeVM), clientset, config)
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

func CheckPcsStonith(activeVM string, clientset *kubernetes.Clientset, config *rest.Config) (bool, error) {
	data, err := ExecuteCommand(fmt.Sprintf("ssh -q %s.ctlplane sudo pcs property show pcs-enabled", activeVM), clientset, config)
	if err != nil {
		return false, err
	}
	if strings.Contains(string(data), "false") {
		return true, nil
	}
	return false, nil
}

func CheckComputeService(clientset *kubernetes.Clientset, config *rest.Config) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand("openstack compute service list -c Host -c State -f value", clientset, config)
	if err != nil {
		return nil, err
	}
	sliceData := strings.Split(string(data), "/n")
	for _, dat := range sliceData {
		if strings.Contains(dat, "down") {
			datSlice := strings.SplitN(dat, " ", 2)
			errSlice = append(errSlice, datSlice[0])
		}
	}
	return errSlice, nil
}

func CheckNetworkAgents(clientset *kubernetes.Clientset, config *rest.Config) ([]string, error) {
	var errSlice []string
	data, err := ExecuteCommand("openstack network agent list -c Host -c State -f value", clientset, config)
	if err != nil {
		return nil, err
	}
	sliceData := strings.Split(string(data), "/n")
	for _, dat := range sliceData {
		if strings.Contains(dat, "False") {
			datSlice := strings.SplitN(dat, " ", 2)
			errSlice = append(errSlice, datSlice[0])
		}
	}
	return errSlice, nil
}
